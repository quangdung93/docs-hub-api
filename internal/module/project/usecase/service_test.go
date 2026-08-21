package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/common/errcode"
	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	"github.com/quangdung93/docs-hub-api/internal/common/port"
	"github.com/quangdung93/docs-hub-api/internal/module/project/domain"
)

type fakeRepo struct {
	exists           bool
	createErr        error
	created          *domain.Project
	role             string
	createVersionErr error
	createdVersion   *domain.ProjectVersion
	versions         []domain.ProjectVersion
}

func (f *fakeRepo) CodeExists(context.Context, string) (bool, error) { return f.exists, nil }
func (f *fakeRepo) Create(_ context.Context, params domain.CreateParams) error {
	f.created = params.Project
	return f.createErr
}
func (f *fakeRepo) MemberRole(context.Context, uuid.UUID, uuid.UUID) (string, error) {
	return f.role, nil
}
func (f *fakeRepo) CreateVersion(_ context.Context, params domain.CreateVersionParams) error {
	f.createdVersion = params.Version
	if params.Version.SequenceNo == 0 {
		params.Version.SequenceNo = 1
	}
	return f.createVersionErr
}
func (f *fakeRepo) ListVersions(
	context.Context, uuid.UUID, pagination.Query,
) ([]domain.ProjectVersion, int64, error) {
	return f.versions, int64(len(f.versions)), nil
}

type fakeTx struct{}

func (fakeTx) Do(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type fixedClock struct{ now time.Time }

func (f fixedClock) Now() time.Time { return f.now }

type fakeRAG struct {
	createCalls int
	deleteCalls int
	createdName string
	createErr   error
}

func (f *fakeRAG) Health(context.Context) error { return nil }
func (f *fakeRAG) CreateDataset(_ context.Context, name, _ string) (port.RAGDataset, error) {
	f.createCalls++
	f.createdName = name
	if f.createErr != nil {
		return port.RAGDataset{}, f.createErr
	}
	return port.RAGDataset{ID: "rag-dataset-1", Name: name}, nil
}
func (*fakeRAG) FindDatasetByName(context.Context, string) (*port.RAGDataset, error) {
	return nil, nil
}
func (*fakeRAG) UpdateDataset(context.Context, string, string, string) error { return nil }
func (f *fakeRAG) DeleteDatasets(context.Context, []string) error {
	f.deleteCalls++
	return nil
}
func (*fakeRAG) UploadDocument(context.Context, string, port.RAGDocumentFile) (port.RAGDocument, error) {
	return port.RAGDocument{}, nil
}
func (*fakeRAG) GetDocument(context.Context, string, string) (port.RAGDocument, error) {
	return port.RAGDocument{}, nil
}
func (*fakeRAG) FindDocumentByName(context.Context, string, string) (*port.RAGDocument, error) {
	return nil, nil
}
func (*fakeRAG) StartParsing(context.Context, string, []string) error { return nil }
func (*fakeRAG) StopParsing(context.Context, string, []string) error  { return nil }
func (*fakeRAG) DeleteDocuments(context.Context, string, []string) error {
	return nil
}
func (*fakeRAG) Retrieve(context.Context, port.RAGRetrievalRequest) (port.RAGRetrievalResult, error) {
	return port.RAGRetrievalResult{}, nil
}

func actorContext() context.Context {
	actor := contextx.Actor{UserID: "10000000-0000-4000-8000-000000000001", Email: "owner@example.com"}
	return contextx.WithActor(context.Background(), actor)
}

func TestCreate_TaoProjectVaDatasetRAGFlow(t *testing.T) {
	repo, rag := &fakeRepo{}, &fakeRAG{}
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.FixedZone("ICT", 7*60*60))
	service := New(repo, fakeTx{}, rag, fixedClock{now: now}, "docs hub_local")

	project, err := service.Create(actorContext(), CreateInput{Code: "AFFILIATE", Name: "Affiliate Marketing"})

	require.NoError(t, err)
	require.Same(t, project, repo.created)
	require.Equal(t, "rag-dataset-1", project.RAGFlowDatasetID)
	require.Equal(t, "ready", project.RAGFlowSyncStatus)
	require.True(t, strings.HasPrefix(rag.createdName, "docs_hub_local_"))
	require.NotContains(t, rag.createdName, "-")
	require.Equal(t, now.UTC(), project.CreatedAt)
}

func TestCreate_TrungCodeKhongGoiRAGFlow(t *testing.T) {
	repo, rag := &fakeRepo{exists: true}, &fakeRAG{}
	service := New(repo, fakeTx{}, rag, fixedClock{now: time.Now()}, "docs_hub_local")

	_, err := service.Create(actorContext(), CreateInput{Code: "AFFILIATE", Name: "Affiliate Marketing"})

	var business *apperr.BusinessError
	require.ErrorAs(t, err, &business)
	require.Equal(t, errcode.ProjectCodeExists, business.Code)
	require.Zero(t, rag.createCalls)
}

func TestCreate_LuuLocalLoiThiXoaDatasetBuTru(t *testing.T) {
	repo := &fakeRepo{createErr: errors.New("database unavailable")}
	rag := &fakeRAG{}
	service := New(repo, fakeTx{}, rag, fixedClock{now: time.Now()}, "docs_hub_local")

	_, err := service.Create(actorContext(), CreateInput{Code: "AFFILIATE", Name: "Affiliate Marketing"})

	var technical *apperr.TechnicalError
	require.ErrorAs(t, err, &technical)
	require.Equal(t, 1, rag.createCalls)
	require.Equal(t, 1, rag.deleteCalls)
}

func TestCreateVersion_EditorTaoDraftThanhCong(t *testing.T) {
	repo := &fakeRepo{role: domain.RoleEditor}
	now := time.Date(2026, 8, 20, 11, 0, 0, 0, time.UTC)
	service := New(repo, fakeTx{}, &fakeRAG{}, fixedClock{now: now}, "docs_hub_local")
	projectID := uuid.MustParse("20000000-0000-4000-8000-000000000001")

	version, err := service.CreateVersion(actorContext(), CreateVersionInput{ProjectID: projectID, Label: " v1.0.0 "})

	require.NoError(t, err)
	require.Same(t, version, repo.createdVersion)
	require.Equal(t, "v1.0.0", version.Label)
	require.Equal(t, domain.StatusDraft, version.Status)
	require.EqualValues(t, 1, version.SequenceNo)
	require.Nil(t, version.ReleasedAt)
}

func TestCreateVersion_ViewerBiCam(t *testing.T) {
	repo := &fakeRepo{role: domain.RoleViewer}
	service := New(repo, fakeTx{}, &fakeRAG{}, fixedClock{now: time.Now()}, "docs_hub_local")

	_, err := service.CreateVersion(actorContext(), CreateVersionInput{
		ProjectID: uuid.New(), Label: "v1",
	})

	var technical *apperr.TechnicalError
	require.ErrorAs(t, err, &technical)
	require.Equal(t, 403, technical.HTTPStatus)
	require.Nil(t, repo.createdVersion)
}

func TestCreateVersion_TrungLabelTraBusinessError(t *testing.T) {
	repo := &fakeRepo{role: domain.RoleOwner, createVersionErr: domain.ErrDuplicateVersionLabel}
	service := New(repo, fakeTx{}, &fakeRAG{}, fixedClock{now: time.Now()}, "docs_hub_local")

	_, err := service.CreateVersion(actorContext(), CreateVersionInput{
		ProjectID: uuid.New(), Label: "v1",
	})

	var business *apperr.BusinessError
	require.ErrorAs(t, err, &business)
	require.Equal(t, errcode.VersionLabelExists, business.Code)
}

func TestListVersions_ViewerDuocXemVaCoPagination(t *testing.T) {
	repo := &fakeRepo{role: domain.RoleViewer, versions: []domain.ProjectVersion{{Label: "v2"}, {Label: "v1"}}}
	service := New(repo, fakeTx{}, &fakeRAG{}, fixedClock{now: time.Now()}, "docs_hub_local")

	versions, meta, err := service.ListVersions(actorContext(), uuid.New(), pagination.Query{})

	require.NoError(t, err)
	require.Len(t, versions, 2)
	require.Equal(t, 1, meta.Page)
	require.Equal(t, 20, meta.Limit)
	require.EqualValues(t, 2, meta.TotalItems)
}
