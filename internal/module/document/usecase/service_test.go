package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	"github.com/quangdung93/docs-hub-api/internal/common/port"
	"github.com/quangdung93/docs-hub-api/internal/module/document/domain"
)

type fakeRepo struct {
	role     string
	scope    bool
	created  *domain.CreateRevisionParams
	revision *domain.Revision
}

func (f *fakeRepo) MemberRole(context.Context, uuid.UUID, uuid.UUID) (string, error) {
	return f.role, nil
}
func (f *fakeRepo) ScopeExists(context.Context, uuid.UUID, domain.Scope) (bool, error) {
	return f.scope, nil
}
func (f *fakeRepo) CreateRevision(_ context.Context, in domain.CreateRevisionParams) (*domain.Document, *domain.Revision, error) {
	f.created = &in
	return &domain.Document{ID: in.DocumentID}, &domain.Revision{ID: in.RevisionID, ObjectKey: in.ObjectKey}, nil
}
func (*fakeRepo) CreateUpload(context.Context, *domain.Upload) error { return nil }
func (*fakeRepo) FindUpload(context.Context, uuid.UUID, uuid.UUID) (*domain.Upload, error) {
	return nil, domain.ErrNotFound
}
func (*fakeRepo) CompleteUpload(context.Context, *domain.Upload) (*domain.Document, *domain.Revision, error) {
	return nil, nil, nil
}
func (*fakeRepo) List(context.Context, uuid.UUID, domain.Filter, pagination.Query) ([]domain.Document, int64, error) {
	return nil, 0, nil
}
func (*fakeRepo) FindDocument(context.Context, uuid.UUID, uuid.UUID) (*domain.Document, []domain.Revision, error) {
	return nil, nil, nil
}
func (f *fakeRepo) FindRevision(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*domain.Revision, error) {
	return f.revision, nil
}
func (*fakeRepo) Update(context.Context, uuid.UUID, uuid.UUID, string, string, int) (*domain.Document, error) {
	return nil, nil
}
func (*fakeRepo) Retry(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) error { return nil }
func (*fakeRepo) SoftDelete(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error       { return nil }

type fakeStore struct {
	data       []byte
	deleted    string
	size       *int64
	presignErr error
}

func (f *fakeStore) Put(context.Context, string, []byte, string) (port.StoredObject, error) {
	panic("không dùng")
}
func (f *fakeStore) PutReader(_ context.Context, key string, r io.Reader, _ int64, ct string) (port.StoredObject, error) {
	f.data, _ = io.ReadAll(r)
	size := int64(len(f.data))
	if f.size != nil {
		size = *f.size
	}
	return port.StoredObject{Key: key, Size: size, ContentType: ct}, nil
}
func (*fakeStore) Get(context.Context, string) ([]byte, error) { return nil, nil }
func (f *fakeStore) GetReader(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(f.data)), nil
}
func (*fakeStore) Stat(context.Context, string) (port.StoredObject, error) {
	return port.StoredObject{}, nil
}
func (f *fakeStore) PresignedPutURL(context.Context, string, time.Duration) (string, error) {
	return "", f.presignErr
}
func (*fakeStore) PresignedGetURL(context.Context, string, time.Duration) (string, error) {
	return "", nil
}
func (f *fakeStore) Delete(_ context.Context, key string) error { f.deleted = key; return nil }

type fakeTx struct{}

func (fakeTx) Do(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type fakeClock struct{}

func (fakeClock) Now() time.Time { return time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC) }

func TestUpload_TaoRevisionVaObjectKeyAnToan(t *testing.T) {
	actor, pid, vid := uuid.New(), uuid.New(), uuid.New()
	repo := &fakeRepo{role: "editor", scope: true}
	store := &fakeStore{}
	svc := New(repo, fakeTx{}, store, fakeClock{})
	data := []byte("noi dung tai lieu")
	sum := sha256.Sum256(data)
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actor.String()})
	input := UploadInput{
		ProjectID: pid, Scope: domain.Scope{VersionID: &vid}, Title: "Tai lieu",
		FileName: "../../yeu cau.md", MediaType: mimeMarkdown, SizeBytes: int64(len(data)),
		SHA256: hex.EncodeToString(sum[:]), Reader: bytes.NewReader(data),
	}
	d, r, err := svc.Upload(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, d)
	require.NotNil(t, r)
	require.NotNil(t, repo.created)
	require.Contains(t, repo.created.ObjectKey, "projects/"+pid.String()+"/documents/")
	require.NotContains(t, repo.created.ObjectKey, "..")
	require.Equal(t, data, store.data)
}

func TestUpload_ViewerBiTuChoi(t *testing.T) {
	actor, pid, vid := uuid.New(), uuid.New(), uuid.New()
	svc := New(&fakeRepo{role: "viewer", scope: true}, fakeTx{}, &fakeStore{}, fakeClock{})
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actor.String()})
	_, _, err := svc.Upload(ctx, UploadInput{ProjectID: pid, Scope: domain.Scope{VersionID: &vid}})
	var technical *apperr.TechnicalError
	require.ErrorAs(t, err, &technical)
	require.Equal(t, 403, technical.HTTPStatus)
}

func TestAuthorize_LocalBypassKhongCanProjectMembership(t *testing.T) {
	actor, pid := uuid.New(), uuid.New()
	svc := New(&fakeRepo{role: ""}, fakeTx{}, &fakeStore{}, fakeClock{}, WithProjectACLBypass(true))
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actor.String(), Email: "admin@local"})

	got, err := svc.authorize(ctx, pid, true)
	require.NoError(t, err)
	require.Equal(t, actor, got)
}

func TestUpload_ChecksumSaiXoaObject(t *testing.T) {
	actor, pid, vid := uuid.New(), uuid.New(), uuid.New()
	repo := &fakeRepo{role: "editor", scope: true}
	store := &fakeStore{}
	svc := New(repo, fakeTx{}, store, fakeClock{})
	data := []byte("abc")
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actor.String()})
	input := UploadInput{
		ProjectID: pid, Scope: domain.Scope{VersionID: &vid}, Title: "x",
		FileName: "x.txt", MediaType: mimeTextPlain, SizeBytes: 3,
		SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Reader: bytes.NewReader(data),
	}
	_, _, err := svc.Upload(ctx, input)
	require.Error(t, err)
	require.NotEmpty(t, store.deleted)
	require.Nil(t, repo.created)
}

func TestUpload_KichThuocLuuTruSaiXoaObject(t *testing.T) {
	actor, pid, vid := uuid.New(), uuid.New(), uuid.New()
	repo := &fakeRepo{role: "editor", scope: true}
	wrongSize := int64(2)
	store := &fakeStore{size: &wrongSize}
	svc := New(repo, fakeTx{}, store, fakeClock{})
	data := []byte("abc")
	sum := sha256.Sum256(data)
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actor.String()})
	_, _, err := svc.Upload(ctx, UploadInput{
		ProjectID: pid, Scope: domain.Scope{VersionID: &vid}, Title: "x",
		FileName: "x.txt", MediaType: mimeTextPlain, SizeBytes: 3,
		SHA256: hex.EncodeToString(sum[:]), Reader: bytes.NewReader(data),
	})
	require.Error(t, err)
	require.NotEmpty(t, store.deleted)
	require.Nil(t, repo.created)
}

func TestValidate_TuChoiExtensionKhongKhopMIME(t *testing.T) {
	actor, pid, vid := uuid.New(), uuid.New(), uuid.New()
	svc := New(&fakeRepo{role: "editor", scope: true}, fakeTx{}, &fakeStore{}, fakeClock{})
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actor.String()})
	_, _, err := svc.Upload(ctx, UploadInput{
		ProjectID: pid, Scope: domain.Scope{VersionID: &vid}, Title: "x",
		FileName: "x.txt", MediaType: "application/pdf", SizeBytes: 3,
		SHA256: strings.Repeat("a", 64), Reader: bytes.NewReader([]byte("abc")),
	})
	require.Error(t, err)
	require.Nil(t, svc.repo.(*fakeRepo).created)
}

func TestMediaTypeForUpload_ChuanHoaDinhDangTaiLieu(t *testing.T) {
	tests := []struct{ name, detected, expected string }{
		{"readme.md", mimeTextPlain, mimeMarkdown},
		{"data.csv", mimeTextPlain, mimeCSV},
		{"spec.docx", "application/zip", mimeDOCX},
		{"cost.xlsx", "application/zip", mimeXLSX},
		{"manual.pdf", mimePDF, mimePDF},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, MediaTypeForUpload(tt.name, tt.detected))
		})
	}
}

func TestPresign_FilesystemYeuCauDungMultipart(t *testing.T) {
	actor, pid, vid := uuid.New(), uuid.New(), uuid.New()
	store := &fakeStore{presignErr: port.ErrPresignUnsupported}
	svc := New(&fakeRepo{role: "editor", scope: true}, fakeTx{}, store, fakeClock{})
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actor.String()})
	_, err := svc.Presign(ctx, PresignInput{
		ProjectID: pid, Scope: domain.Scope{VersionID: &vid}, Title: "x",
		FileName: "x.txt", MediaType: mimeTextPlain, SizeBytes: 3,
		SHA256: strings.Repeat("a", 64),
	})
	var technical *apperr.TechnicalError
	require.ErrorAs(t, err, &technical)
	require.Equal(t, 400, technical.HTTPStatus)
}

func TestDownload_DocFileQuaObjectStore(t *testing.T) {
	actor, pid, did, rid := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	revision := &domain.Revision{ID: rid, DocumentID: did, ProjectID: pid, ObjectKey: "object.txt", SizeBytes: 3}
	store := &fakeStore{data: []byte("abc")}
	svc := New(&fakeRepo{role: "viewer", revision: revision}, fakeTx{}, store, fakeClock{})
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actor.String()})

	gotRevision, reader, err := svc.Download(ctx, pid, did, rid)
	require.NoError(t, err)
	defer reader.Close()
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, revision, gotRevision)
	require.Equal(t, "abc", string(data))
}
