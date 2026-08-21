package filesystem

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/quangdung93/docs-hub-api/internal/common/port"
)

// Store implement ObjectStore bằng filesystem giới hạn trong một root cố định.
type Store struct{ rootPath string }

var _ port.ObjectStore = (*Store)(nil)

// New tạo thư mục storage nếu chưa tồn tại và trả adapter local.
func New(rootPath string) (*Store, error) {
	absolute, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, fmt.Errorf("chuẩn hóa storage root: %w", err)
	}
	if err = os.MkdirAll(absolute, 0o750); err != nil {
		return nil, fmt.Errorf("tạo storage root %q: %w", absolute, err)
	}
	root, err := os.OpenRoot(absolute)
	if err != nil {
		return nil, fmt.Errorf("mở storage root %q: %w", absolute, err)
	}
	if err = root.Close(); err != nil {
		return nil, fmt.Errorf("đóng storage root %q: %w", absolute, err)
	}
	return &Store{rootPath: absolute}, nil
}

func (s *Store) Put(ctx context.Context, key string, data []byte, contentType string) (port.StoredObject, error) {
	return s.PutReader(ctx, key, bytes.NewReader(data), int64(len(data)), contentType)
}

func (s *Store) PutReader(
	ctx context.Context, key string, reader io.Reader, size int64, contentType string,
) (port.StoredObject, error) {
	name, err := objectName(key)
	if err != nil {
		return port.StoredObject{}, err
	}
	if size < 0 {
		return port.StoredObject{}, fmt.Errorf("kích thước object không hợp lệ")
	}
	root, err := os.OpenRoot(s.rootPath)
	if err != nil {
		return port.StoredObject{}, fmt.Errorf("mở storage root: %w", err)
	}
	defer root.Close()
	directory := filepath.Dir(name)
	if directory != "." {
		if err = root.MkdirAll(directory, 0o750); err != nil {
			return port.StoredObject{}, fmt.Errorf("tạo thư mục object %q: %w", key, err)
		}
	}
	temporary, err := temporaryName(directory)
	if err != nil {
		return port.StoredObject{}, fmt.Errorf("tạo tên object tạm: %w", err)
	}
	file, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return port.StoredObject{}, fmt.Errorf("tạo object tạm %q: %w", key, err)
	}
	removeTemporary := true
	defer func() {
		_ = file.Close()
		if removeTemporary {
			_ = root.Remove(temporary)
		}
	}()
	written, copyErr := io.Copy(file, io.LimitReader(&contextReader{ctx: ctx, reader: reader}, size+1))
	if copyErr != nil {
		return port.StoredObject{}, fmt.Errorf("ghi object %q: %w", key, copyErr)
	}
	if written != size {
		return port.StoredObject{}, fmt.Errorf("kích thước object %q là %d, cần %d", key, written, size)
	}
	if err = file.Sync(); err != nil {
		return port.StoredObject{}, fmt.Errorf("sync object %q: %w", key, err)
	}
	if err = file.Close(); err != nil {
		return port.StoredObject{}, fmt.Errorf("đóng object %q: %w", key, err)
	}
	if err = root.Rename(temporary, name); err != nil {
		return port.StoredObject{}, fmt.Errorf("hoàn tất object %q: %w", key, err)
	}
	removeTemporary = false
	return port.StoredObject{Key: key, Size: written, ContentType: contentType}, nil
}

func (s *Store) Get(ctx context.Context, key string) ([]byte, error) {
	reader, err := s.GetReader(ctx, key)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(&contextReader{ctx: ctx, reader: reader})
	if err != nil {
		return nil, fmt.Errorf("đọc object %q: %w", key, err)
	}
	return data, nil
}

func (s *Store) GetReader(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	name, err := objectName(key)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(s.rootPath)
	if err != nil {
		return nil, fmt.Errorf("mở storage root: %w", err)
	}
	file, err := root.Open(name)
	closeErr := root.Close()
	if err != nil {
		return nil, fmt.Errorf("mở object %q: %w", key, err)
	}
	if closeErr != nil {
		_ = file.Close()
		return nil, fmt.Errorf("đóng storage root: %w", closeErr)
	}
	return file, nil
}

func (s *Store) Stat(ctx context.Context, key string) (port.StoredObject, error) {
	if err := ctx.Err(); err != nil {
		return port.StoredObject{}, err
	}
	name, err := objectName(key)
	if err != nil {
		return port.StoredObject{}, err
	}
	root, err := os.OpenRoot(s.rootPath)
	if err != nil {
		return port.StoredObject{}, fmt.Errorf("mở storage root: %w", err)
	}
	defer root.Close()
	info, err := root.Stat(name)
	if err != nil {
		return port.StoredObject{}, fmt.Errorf("stat object %q: %w", key, err)
	}
	if !info.Mode().IsRegular() {
		return port.StoredObject{}, fmt.Errorf("object %q không phải file thường", key)
	}
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return port.StoredObject{Key: key, Size: info.Size(), ContentType: contentType}, nil
}

func (*Store) PresignedPutURL(context.Context, string, time.Duration) (string, error) {
	return "", port.ErrPresignUnsupported
}

func (*Store) PresignedGetURL(context.Context, string, time.Duration) (string, error) {
	return "", port.ErrPresignUnsupported
}

func (s *Store) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	name, err := objectName(key)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(s.rootPath)
	if err != nil {
		return fmt.Errorf("mở storage root: %w", err)
	}
	defer root.Close()
	if err = root.Remove(name); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("xóa object %q: %w", key, err)
	}
	return nil
}

func objectName(key string) (string, error) {
	name := filepath.Clean(filepath.FromSlash(strings.TrimSpace(key)))
	if name == "." || filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("object key không hợp lệ")
	}
	return name, nil
}

func temporaryName(directory string) (string, error) {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return filepath.Join(directory, ".upload-"+hex.EncodeToString(value)+".tmp"), nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(value []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
		return r.reader.Read(value)
	}
}
