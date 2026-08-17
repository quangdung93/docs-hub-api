// Package usecase điều phối upload và quản lý tài liệu, không phụ thuộc HTTP/GORM.
package usecase

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/common/errcode"
	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	"github.com/quangdung93/docs-hub-api/internal/common/port"
	"github.com/quangdung93/docs-hub-api/internal/module/document/domain"
)

const (
	maxUploadSize int64 = 50 << 20
	mimeTextPlain       = "text/plain"
	mimeMarkdown        = "text/markdown"
	mimeCSV             = "text/csv"
	mimePDF             = "application/pdf"
	mimeDOCX            = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	mimeXLSX            = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
)

var safeNamePattern = regexp.MustCompile(`[^a-zA-Z0-9._-]+`) //nolint:gochecknoglobals // regex bất biến

type Service struct {
	repo             domain.Repository
	tx               port.TxManager
	store            port.ObjectStore
	clock            port.Clock
	bypassProjectACL bool
}

// Option cấu hình hành vi tùy môi trường cho document service.
type Option func(*Service)

// WithProjectACLBypass chỉ dành cho local development; production không được bật.
func WithProjectACLBypass(enabled bool) Option {
	return func(service *Service) { service.bypassProjectACL = enabled }
}

func New(repo domain.Repository, tx port.TxManager, store port.ObjectStore, clock port.Clock, options ...Option) *Service {
	service := &Service{repo: repo, tx: tx, store: store, clock: clock}
	for _, option := range options {
		option(service)
	}
	return service
}

type UploadInput struct {
	ProjectID, DocumentID                           uuid.UUID
	Scope                                           domain.Scope
	Title, Description, FileName, MediaType, SHA256 string
	SizeBytes                                       int64
	Reader                                          io.Reader
}
type PresignInput struct {
	ProjectID, DocumentID                           uuid.UUID
	Scope                                           domain.Scope
	Title, Description, FileName, MediaType, SHA256 string
	SizeBytes                                       int64
}
type PresignResult struct {
	UploadID  uuid.UUID `json:"upload_id"`
	ObjectKey string    `json:"object_key"`
	UploadURL string    `json:"upload_url"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *Service) Upload(ctx context.Context, in UploadInput) (*domain.Document, *domain.Revision, error) {
	newDocument := in.DocumentID == uuid.Nil
	actor, err := s.authorize(ctx, in.ProjectID, true)
	if err != nil {
		return nil, nil, err
	}
	if newDocument && strings.TrimSpace(in.Title) == "" {
		return nil, nil, apperr.BadRequest("Title là bắt buộc khi tạo tài liệu")
	}
	if len([]rune(in.Title)) > 255 {
		return nil, nil, apperr.BadRequest("Title không được vượt quá 255 ký tự")
	}
	if err = s.validate(ctx, in.ProjectID, in.Scope, in.FileName, in.MediaType, in.SizeBytes, in.SHA256); err != nil {
		return nil, nil, err
	}
	if in.Reader == nil {
		return nil, nil, apperr.BadRequest("File rỗng")
	}
	if in.DocumentID == uuid.Nil {
		in.DocumentID = uuid.New()
	}
	rid := uuid.New()
	key := objectKey(in.ProjectID, in.DocumentID, rid, in.FileName)
	hasher := sha256.New()
	stored, err := s.store.PutReader(ctx, key, io.TeeReader(in.Reader, hasher), in.SizeBytes, in.MediaType)
	if err != nil {
		return nil, nil, apperr.Internal("Không thể lưu tài liệu").WithCause(err)
	}
	if stored.Size != in.SizeBytes {
		_ = s.store.Delete(ctx, key)
		return nil, nil, apperr.BadRequest("Kích thước file thực tế không khớp")
	}
	if hex.EncodeToString(hasher.Sum(nil)) != strings.ToLower(in.SHA256) {
		_ = s.store.Delete(ctx, key)
		return nil, nil, apperr.BadRequest("SHA-256 file không khớp")
	}
	var d *domain.Document
	var rev *domain.Revision
	err = s.tx.Do(ctx, func(txctx context.Context) error {
		var e error
		params := domain.CreateRevisionParams{
			DocumentID: in.DocumentID, RevisionID: rid, ProjectID: in.ProjectID,
			ActorID: actor, Scope: in.Scope, Title: in.Title, Description: in.Description,
			FileName: safeName(in.FileName), MediaType: in.MediaType,
			SHA256: strings.ToLower(in.SHA256), ObjectKey: stored.Key, SizeBytes: stored.Size,
		}
		d, rev, e = s.repo.CreateRevision(txctx, params)
		if e != nil {
			return fmt.Errorf("lưu metadata revision: %w", e)
		}
		return nil
	})
	if err != nil {
		_ = s.store.Delete(ctx, key)
		return nil, nil, apperr.Internal("Không thể tạo revision").WithCause(err)
	}
	return d, rev, nil
}
func (s *Service) Presign(ctx context.Context, in PresignInput) (*PresignResult, error) {
	actor, err := s.authorize(ctx, in.ProjectID, true)
	if err != nil {
		return nil, err
	}
	if in.DocumentID == uuid.Nil && strings.TrimSpace(in.Title) == "" {
		return nil, apperr.BadRequest("Title là bắt buộc khi tạo tài liệu")
	}
	if len([]rune(in.Title)) > 255 {
		return nil, apperr.BadRequest("Title không được vượt quá 255 ký tự")
	}
	if err = s.validate(ctx, in.ProjectID, in.Scope, in.FileName, in.MediaType, in.SizeBytes, in.SHA256); err != nil {
		return nil, err
	}
	if in.DocumentID == uuid.Nil {
		in.DocumentID = uuid.New()
	}
	u := &domain.Upload{
		ID: uuid.New(), ProjectID: in.ProjectID, DocumentID: in.DocumentID,
		RevisionID: uuid.New(), CreatedBy: actor, Scope: in.Scope,
		Title: in.Title, Description: in.Description, FileName: safeName(in.FileName),
		MediaType: in.MediaType, SizeBytes: in.SizeBytes, SHA256: strings.ToLower(in.SHA256),
		Status: "pending", ExpiresAt: s.clock.Now().Add(15 * time.Minute),
	}
	u.ObjectKey = objectKey(u.ProjectID, u.DocumentID, u.RevisionID, u.FileName)
	url, err := s.store.PresignedPutURL(ctx, u.ObjectKey, 15*time.Minute)
	if err != nil {
		if errors.Is(err, port.ErrPresignUnsupported) {
			return nil, apperr.BadRequest("Storage local không hỗ trợ presigned upload; hãy dùng multipart upload")
		}
		return nil, apperr.Internal("Không thể tạo URL upload").WithCause(err)
	}
	if err = s.repo.CreateUpload(ctx, u); err != nil {
		return nil, apperr.Internal("Không thể tạo phiên upload").WithCause(err)
	}
	return &PresignResult{UploadID: u.ID, ObjectKey: u.ObjectKey, UploadURL: url, ExpiresAt: u.ExpiresAt}, nil
}
func (s *Service) Complete(ctx context.Context, pid, uid uuid.UUID) (*domain.Document, *domain.Revision, error) {
	actor, err := s.authorize(ctx, pid, true)
	if err != nil {
		return nil, nil, err
	}
	u, err := s.repo.FindUpload(ctx, pid, uid)
	if err != nil {
		return nil, nil, s.mapErr(err)
	}
	if u.CreatedBy != actor || u.Status != "pending" || !s.clock.Now().Before(u.ExpiresAt) {
		return nil, nil, apperr.NewBusiness(errcode.UploadInvalid, "Phiên upload không hợp lệ hoặc đã hết hạn", false)
	}
	if err = s.verifyObject(ctx, u); err != nil {
		return nil, nil, err
	}
	var d *domain.Document
	var rev *domain.Revision
	err = s.tx.Do(ctx, func(txctx context.Context) error {
		var e error
		d, rev, e = s.repo.CompleteUpload(txctx, u)
		if e != nil {
			return fmt.Errorf("lưu upload hoàn tất: %w", e)
		}
		return nil
	})
	if errors.Is(err, domain.ErrConflict) {
		return nil, nil, apperr.NewBusiness(errcode.UploadInvalid, "Phiên upload đã được hoàn tất", false)
	}
	if err != nil {
		return nil, nil, apperr.Internal("Không thể hoàn tất upload").WithCause(err)
	}
	return d, rev, nil
}

func (s *Service) verifyObject(ctx context.Context, u *domain.Upload) error {
	info, err := s.store.Stat(ctx, u.ObjectKey)
	if err != nil {
		return apperr.BadRequest("Object upload chưa tồn tại").WithCause(err)
	}
	if info.Size != u.SizeBytes {
		return apperr.BadRequest("Kích thước object không khớp")
	}
	reader, err := s.store.GetReader(ctx, u.ObjectKey)
	if err != nil {
		return apperr.Internal("Không thể kiểm tra object").WithCause(err)
	}
	defer reader.Close()
	buffered := bufio.NewReader(reader)
	head, _ := buffered.Peek(512)
	actualMediaType := http.DetectContentType(head)
	if actualMediaType == "text/plain; charset=utf-8" {
		actualMediaType = mimeTextPlain
	}
	actualMediaType = MediaTypeForUpload(u.FileName, actualMediaType)
	if actualMediaType != u.MediaType {
		return apperr.BadRequest("MIME thực tế của object không khớp")
	}
	hash := sha256.New()
	if _, err = io.Copy(hash, buffered); err != nil {
		return apperr.Internal("Không thể kiểm tra checksum").WithCause(err)
	}
	if hex.EncodeToString(hash.Sum(nil)) != u.SHA256 {
		return apperr.BadRequest("SHA-256 object không khớp")
	}
	return nil
}

func (s *Service) List(
	ctx context.Context, pid uuid.UUID, f domain.Filter, p pagination.Query,
) ([]domain.Document, pagination.Meta, error) {
	if _, err := s.authorize(ctx, pid, false); err != nil {
		return nil, pagination.Meta{}, err
	}
	items, total, err := s.repo.List(ctx, pid, f, p.Normalize())
	p = p.Normalize()
	return items, pagination.NewMeta(p.Page, p.Limit, total), err
}
func (s *Service) Detail(ctx context.Context, pid, did uuid.UUID) (*domain.Document, []domain.Revision, error) {
	if _, err := s.authorize(ctx, pid, false); err != nil {
		return nil, nil, err
	}
	d, r, err := s.repo.FindDocument(ctx, pid, did)
	return d, r, s.mapErr(err)
}
func (s *Service) Status(ctx context.Context, pid, did, rid uuid.UUID) (*domain.Revision, error) {
	if _, err := s.authorize(ctx, pid, false); err != nil {
		return nil, err
	}
	r, err := s.repo.FindRevision(ctx, pid, did, rid)
	return r, s.mapErr(err)
}
func (s *Service) Update(ctx context.Context, pid, did uuid.UUID, title, desc string, v int) (*domain.Document, error) {
	if _, err := s.authorize(ctx, pid, true); err != nil {
		return nil, err
	}
	d, err := s.repo.Update(ctx, pid, did, title, desc, v)
	if errors.Is(err, domain.ErrConflict) {
		return nil, apperr.NewBusiness(errcode.ConflictVersion, "Tài liệu đã được cập nhật bởi yêu cầu khác", true)
	}
	return d, s.mapErr(err)
}
func (s *Service) Retry(ctx context.Context, pid, did, rid uuid.UUID) error {
	actor, err := s.authorize(ctx, pid, true)
	if err != nil {
		return err
	}
	err = s.tx.Do(ctx, func(txctx context.Context) error { return s.repo.Retry(txctx, pid, did, rid, actor) })
	if errors.Is(err, domain.ErrConflict) {
		return apperr.NewBusiness(errcode.DocumentRetryInvalid, "Chỉ revision thất bại mới được retry", false)
	}
	return s.mapErr(err)
}
func (s *Service) Download(ctx context.Context, pid, did, rid uuid.UUID) (*domain.Revision, io.ReadCloser, error) {
	r, err := s.Status(ctx, pid, did, rid)
	if err != nil {
		return nil, nil, err
	}
	reader, err := s.store.GetReader(ctx, r.ObjectKey)
	if err != nil {
		return nil, nil, apperr.Internal("Không thể mở tài liệu").WithCause(err)
	}
	return r, reader, nil
}
func (s *Service) Delete(ctx context.Context, pid, did uuid.UUID) error {
	actor, err := s.authorize(ctx, pid, true)
	if err != nil {
		return err
	}
	return s.mapErr(s.tx.Do(ctx, func(txctx context.Context) error { return s.repo.SoftDelete(txctx, pid, did, actor) }))
}
func (s *Service) authorize(ctx context.Context, pid uuid.UUID, write bool) (uuid.UUID, error) {
	a, ok := contextx.ActorFrom(ctx)
	if !ok {
		return uuid.Nil, apperr.Unauthorized("Chưa xác thực")
	}
	uid, err := uuid.Parse(a.UserID)
	if err != nil {
		return uuid.Nil, apperr.Unauthorized("Actor không hợp lệ")
	}
	if s.bypassProjectACL {
		return uid, nil
	}
	role, err := s.repo.MemberRole(ctx, pid, uid)
	if err != nil {
		return uuid.Nil, apperr.Internal("Không thể kiểm tra quyền").WithCause(err)
	}
	if role == "" || (write && role == "viewer") {
		return uuid.Nil, apperr.Forbidden("Không có quyền thao tác project")
	}
	return uid, nil
}
func (s *Service) validate(ctx context.Context, pid uuid.UUID, scope domain.Scope, name, mime string, size int64, hash string) error {
	if !scope.Valid() {
		return apperr.BadRequest("Phải chọn đúng một version hoặc change request")
	}
	ok, err := s.repo.ScopeExists(ctx, pid, scope)
	if err != nil {
		return apperr.Internal("Không thể kiểm tra scope").WithCause(err)
	}
	if !ok {
		return apperr.BadRequest("Scope không thuộc project")
	}
	ext := strings.ToLower(filepath.Ext(name))
	validType := (ext == ".txt" && mime == mimeTextPlain) ||
		(ext == ".md" && mime == mimeMarkdown) ||
		(ext == ".csv" && mime == mimeCSV) ||
		(ext == ".pdf" && mime == mimePDF) ||
		(ext == ".docx" && mime == mimeDOCX) ||
		(ext == ".xlsx" && mime == mimeXLSX)
	if !validType {
		return apperr.BadRequest("Chỉ hỗ trợ TXT, Markdown, CSV, PDF text-layer, DOCX và XLSX")
	}
	if size <= 0 {
		return apperr.BadRequest("File rỗng")
	}
	if size > maxUploadSize {
		return apperr.NewBusiness(errcode.FileTooLarge, "File vượt quá 50 MiB", false)
	}
	if len(hash) != 64 {
		return apperr.BadRequest("SHA-256 không hợp lệ")
	}
	if _, err := hex.DecodeString(hash); err != nil {
		return apperr.BadRequest("SHA-256 không hợp lệ")
	}
	return nil
}

// MediaTypeForUpload chuẩn hóa các container ZIP/text mà DetectContentType
// không thể phân biệt nếu thiếu phần mở rộng của file.
func MediaTypeForUpload(fileName, detected string) string {
	ext := strings.ToLower(filepath.Ext(fileName))
	switch {
	case ext == ".md" && detected == mimeTextPlain:
		return mimeMarkdown
	case ext == ".csv" && detected == mimeTextPlain:
		return mimeCSV
	case ext == ".docx" && detected == "application/zip":
		return mimeDOCX
	case ext == ".xlsx" && detected == "application/zip":
		return mimeXLSX
	default:
		return detected
	}
}
func (s *Service) mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrNotFound) {
		return apperr.NotFound(errcode.NotFound, "Không tìm thấy tài liệu")
	}
	return err
}
func safeName(name string) string {
	name = filepath.Base(name)
	name = safeNamePattern.ReplaceAllString(name, "_")
	name = strings.Trim(name, "._")
	if name == "" {
		return "document"
	}
	if len(name) > 180 {
		name = name[:180]
	}
	return name
}
func objectKey(pid, did, rid uuid.UUID, name string) string {
	return fmt.Sprintf("projects/%s/documents/%s/revisions/%s/%s", pid, did, rid, safeName(name))
}
