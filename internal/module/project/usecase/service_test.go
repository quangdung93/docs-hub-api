package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	"github.com/quangdung93/docs-hub-api/internal/common/port"
	"github.com/quangdung93/docs-hub-api/internal/module/project/domain"
)

type fakeProjectRepo struct {
	created   *domain.Project
	createErr error
}

func (f *fakeProjectRepo) Create(_ context.Context, p *domain.Project) error {
	f.created = p
	return f.createErr
}
func (*fakeProjectRepo) Stats(context.Context, []uuid.UUID) (map[uuid.UUID]domain.ProjectStats, error) {
	return nil, nil
}
func (*fakeProjectRepo) Update(context.Context, *domain.Project) error { return nil }
func (*fakeProjectRepo) FindByID(context.Context, uuid.UUID) (*domain.Project, error) {
	return nil, domain.ErrNotFound
}
func (*fakeProjectRepo) ListForUser(context.Context, uuid.UUID, pagination.Query) ([]domain.Project, int64, error) {
	return nil, 0, nil
}
func (*fakeProjectRepo) Delete(context.Context, uuid.UUID) error { return nil }

type fakeMemberRepo struct{ member *domain.ProjectMember }

func (f *fakeMemberRepo) Create(_ context.Context, m *domain.ProjectMember) error {
	f.member = m
	return nil
}
func (f *fakeMemberRepo) FindByProjectAndUser(context.Context, uuid.UUID, uuid.UUID) (*domain.ProjectMember, error) {
	if f.member == nil {
		return nil, domain.ErrNotFound
	}
	return f.member, nil
}
func (*fakeMemberRepo) ListByProject(context.Context, uuid.UUID) ([]domain.MemberWithUser, error) {
	return nil, nil
}
func (*fakeMemberRepo) UpdateRole(context.Context, uuid.UUID, uuid.UUID, domain.Role) error {
	return nil
}
func (*fakeMemberRepo) UpdateStatus(context.Context, *domain.ProjectMember) error { return nil }
func (*fakeMemberRepo) Delete(context.Context, uuid.UUID, uuid.UUID) error        { return nil }

type fakeVersionRepo struct {
	created *domain.ProjectVersion
	err     error
}

func (f *fakeVersionRepo) Create(_ context.Context, v *domain.ProjectVersion, _ string) error {
	f.created = v
	if v.SequenceNo == 0 {
		v.SequenceNo = 1
	}
	return f.err
}
func (*fakeVersionRepo) List(context.Context, uuid.UUID, pagination.Query) ([]domain.ProjectVersion, int64, error) {
	return nil, 0, nil
}

type fakeTx struct{}

func (fakeTx) Do(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type fixedClock struct{ now time.Time }

func (f fixedClock) Now() time.Time { return f.now }

type fakeRAG struct {
	createCalls, deleteCalls int
	createdName              string
}

func (*fakeRAG) Health(context.Context) error { return nil }
func (f *fakeRAG) CreateDataset(_ context.Context, name, _ string) (port.RAGDataset, error) {
	f.createCalls++
	f.createdName = name
	return port.RAGDataset{ID: "rag-dataset-1", Name: name}, nil
}
func (*fakeRAG) FindDatasetByName(context.Context, string) (*port.RAGDataset, error) { return nil, nil }
func (*fakeRAG) UpdateDataset(context.Context, string, string, string) error         { return nil }
func (f *fakeRAG) DeleteDatasets(context.Context, []string) error                    { f.deleteCalls++; return nil }
func (*fakeRAG) UploadDocument(context.Context, string, port.RAGDocumentFile) (port.RAGDocument, error) {
	return port.RAGDocument{}, nil
}
func (*fakeRAG) GetDocument(context.Context, string, string) (port.RAGDocument, error) {
	return port.RAGDocument{}, nil
}
func (*fakeRAG) FindDocumentByName(context.Context, string, string) (*port.RAGDocument, error) {
	return nil, nil
}
func (*fakeRAG) StartParsing(context.Context, string, []string) error    { return nil }
func (*fakeRAG) StopParsing(context.Context, string, []string) error     { return nil }
func (*fakeRAG) DeleteDocuments(context.Context, string, []string) error { return nil }
func (*fakeRAG) Retrieve(context.Context, port.RAGRetrievalRequest) (port.RAGRetrievalResult, error) {
	return port.RAGRetrievalResult{}, nil
}

func newIntegratedService(projectRepo *fakeProjectRepo, memberRepo *fakeMemberRepo, versions *fakeVersionRepo, rag port.RAGClient) Service {
	return NewService(Deps{ProjectRepo: projectRepo, MemberRepo: memberRepo, VersionRepo: versions,
		Tx: fakeTx{}, Clock: fixedClock{now: time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)},
		RAG: rag, DatasetPrefix: "docs hub_local"})
}

func TestCreate_TaoProjectVaDatasetRAGFlow(t *testing.T) {
	repo, members, rag := &fakeProjectRepo{}, &fakeMemberRepo{}, &fakeRAG{}
	svc := newIntegratedService(repo, members, &fakeVersionRepo{}, rag)
	ownerID := uuid.New()
	project, err := svc.Create(context.Background(), CreateProjectInput{OwnerID: ownerID, Code: "AFFILIATE", Name: "Affiliate"})
	require.NoError(t, err)
	require.Same(t, project, repo.created)
	require.Equal(t, "rag-dataset-1", project.RAGFlowDatasetID)
	require.Equal(t, "ready", project.RAGFlowSyncStatus)
	require.True(t, strings.HasPrefix(rag.createdName, "docs_hub_local_"))
	require.Equal(t, domain.RoleOwner, members.member.Role)
}

func TestCreate_LuuLocalLoiThiXoaDatasetBuTru(t *testing.T) {
	repo, rag := &fakeProjectRepo{createErr: errors.New("database unavailable")}, &fakeRAG{}
	svc := newIntegratedService(repo, &fakeMemberRepo{}, &fakeVersionRepo{}, rag)
	_, err := svc.Create(context.Background(), CreateProjectInput{OwnerID: uuid.New(), Code: "AFFILIATE", Name: "Affiliate"})
	require.Error(t, err)
	require.Equal(t, 1, rag.deleteCalls)
}

func TestCreateVersion_EditorTaoDraftThanhCong(t *testing.T) {
	actorID := uuid.New()
	memberRepo := &fakeMemberRepo{member: &domain.ProjectMember{UserID: actorID, Role: domain.RoleEditor, Status: domain.MemberStatusActive}}
	versions := &fakeVersionRepo{}
	svc := newIntegratedService(&fakeProjectRepo{}, memberRepo, versions, nil)
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actorID.String()})
	version, err := svc.CreateVersion(ctx, CreateVersionInput{ProjectID: uuid.New(), Label: " v1.0.0 "})
	require.NoError(t, err)
	require.Same(t, version, versions.created)
	require.Equal(t, "v1.0.0", version.Label)
	require.Equal(t, domain.StatusDraft, version.Status)
}
