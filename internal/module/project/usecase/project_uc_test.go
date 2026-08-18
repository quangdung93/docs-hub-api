package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/errcode"
	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	"github.com/quangdung93/docs-hub-api/internal/common/port"
	portmocks "github.com/quangdung93/docs-hub-api/internal/common/port/mocks"
	"github.com/quangdung93/docs-hub-api/internal/module/project/domain"
	domainmocks "github.com/quangdung93/docs-hub-api/internal/module/project/domain/mocks"
	"github.com/quangdung93/docs-hub-api/internal/module/project/usecase"
)

const (
	avatarMaxBytes     = int64(1024)
	avatarPresignedTTL = 15 * time.Minute
)

// harness gom service + các mock để test tiện lợi.
type harness struct {
	svc         usecase.Service
	projectRepo *domainmocks.MockProjectRepository
	memberRepo  *domainmocks.MockProjectMemberRepository
	tx          *portmocks.MockTxManager
	clock       *portmocks.MockClock
	objectStore *portmocks.MockObjectStore
}

func newHarness(t *testing.T) *harness {
	projectRepo := domainmocks.NewMockProjectRepository(t)
	memberRepo := domainmocks.NewMockProjectMemberRepository(t)
	tx := portmocks.NewMockTxManager(t)
	clock := portmocks.NewMockClock(t)
	objectStore := portmocks.NewMockObjectStore(t)

	svc := usecase.NewService(usecase.Deps{
		ProjectRepo:        projectRepo,
		MemberRepo:         memberRepo,
		Tx:                 tx,
		Clock:              clock,
		ObjectStore:        objectStore,
		AvatarMaxBytes:     avatarMaxBytes,
		AvatarPresignedTTL: avatarPresignedTTL,
	})
	return &harness{
		svc: svc, projectRepo: projectRepo, memberRepo: memberRepo, tx: tx, clock: clock, objectStore: objectStore,
	}
}

// runTx cấu hình MockTxManager để THỰC SỰ chạy callback (mô phỏng transaction
// thành công) — giống hệt hành vi của postgres.TxManager thật.
func runTx(tx *portmocks.MockTxManager) {
	tx.EXPECT().Do(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		})
}

func TestCreate_HappyPath(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	ownerID := uuid.New()
	fixedNow := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	runTx(h.tx)
	h.clock.EXPECT().Now().Return(fixedNow)
	h.projectRepo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.Project")).Return(nil)
	h.memberRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(m *domain.ProjectMember) bool {
		return m.UserID == ownerID && m.Role == domain.RoleOwner && m.Status == domain.MemberStatusActive &&
			m.JoinedAt != nil && m.JoinedAt.Equal(fixedNow)
	})).Return(nil)

	project, err := h.svc.Create(ctx, usecase.CreateProjectInput{
		OwnerID: ownerID, Name: "RAG Demo", Description: "mô tả",
		Settings: domain.ProjectSettings{Model: "gpt-4o-mini", TopK: 5, ChunkSize: 500},
	})
	require.NoError(t, err)
	require.Equal(t, "RAG Demo", project.Name)
	require.Equal(t, domain.DefaultProjectStatus, project.Status)
	require.Equal(t, ownerID, project.OwnerID)
}

func TestCreate_RepoError_IsTechnicalDatabase(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	runTx(h.tx)
	h.clock.EXPECT().Now().Return(time.Now())
	h.projectRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("boom"))

	_, err := h.svc.Create(ctx, usecase.CreateProjectInput{OwnerID: uuid.New(), Name: "X"})
	require.Error(t, err)
	te, ok := apperr.AsTechnical(err)
	require.True(t, ok, "lỗi hạ tầng phải là lỗi KỸ THUẬT")
	require.Equal(t, errcode.DB500, te.Code)
}

func TestList_HappyPath(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	userID := uuid.New()
	projects := []domain.Project{*domain.NewProject(userID, "P1", "", domain.ProjectSettings{})}

	h.projectRepo.EXPECT().
		ListForUser(mock.Anything, userID, mock.AnythingOfType("pagination.Query")).
		Return(projects, int64(1), nil)

	got, meta, err := h.svc.List(ctx, userID, pagination.Query{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, int64(1), meta.TotalItems)
}

func TestUpdate_HappyPath(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	id := uuid.New()
	existing := domain.NewProject(uuid.New(), "Old", "old desc", domain.ProjectSettings{})
	existing.ID = id

	h.projectRepo.EXPECT().FindByID(mock.Anything, id).Return(existing, nil)
	h.projectRepo.EXPECT().Update(mock.Anything, mock.MatchedBy(func(p *domain.Project) bool {
		return p.Name == "New"
	})).Return(nil)

	newName := "New"
	got, err := h.svc.Update(ctx, usecase.UpdateProjectInput{ID: id, Name: &newName})
	require.NoError(t, err)
	require.Equal(t, "New", got.Name)
	require.Equal(t, "old desc", got.Description)
}

func TestUpdate_NotFound_IsTechnical404(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	id := uuid.New()

	h.projectRepo.EXPECT().FindByID(mock.Anything, id).Return(nil, domain.ErrNotFound)

	_, err := h.svc.Update(ctx, usecase.UpdateProjectInput{ID: id})
	require.Error(t, err)
	te, ok := apperr.AsTechnical(err)
	require.True(t, ok)
	require.Equal(t, errcode.ProjectNotFound, te.Code)
}

func TestDelete_HappyPath(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	id := uuid.New()
	existing := domain.NewProject(uuid.New(), "Core Banking", "", domain.ProjectSettings{})
	existing.ID = id

	h.projectRepo.EXPECT().FindByID(mock.Anything, id).Return(existing, nil)
	h.projectRepo.EXPECT().Delete(mock.Anything, id).Return(nil)

	err := h.svc.Delete(ctx, usecase.DeleteProjectInput{ID: id, ConfirmName: "Core Banking"})
	require.NoError(t, err)
}

func TestDelete_ConfirmNameMismatch_IsBusinessError(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	id := uuid.New()
	existing := domain.NewProject(uuid.New(), "Core Banking", "", domain.ProjectSettings{})
	existing.ID = id

	h.projectRepo.EXPECT().FindByID(mock.Anything, id).Return(existing, nil)

	err := h.svc.Delete(ctx, usecase.DeleteProjectInput{ID: id, ConfirmName: "Sai Ten"})
	require.Error(t, err)
	be, ok := apperr.AsBusiness(err)
	require.True(t, ok, "tên xác nhận sai phải là lỗi NGHIỆP VỤ, không được xóa")
	require.Equal(t, errcode.ConfirmNameMismatch, be.Code)
	h.projectRepo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
}

func TestDelete_NotFound_IsTechnical404(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	id := uuid.New()

	h.projectRepo.EXPECT().FindByID(mock.Anything, id).Return(nil, domain.ErrNotFound)

	err := h.svc.Delete(ctx, usecase.DeleteProjectInput{ID: id, ConfirmName: "bất kỳ"})
	require.Error(t, err)
	te, ok := apperr.AsTechnical(err)
	require.True(t, ok)
	require.Equal(t, errcode.ProjectNotFound, te.Code)
}

func TestListMembers_HappyPath(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	projectID := uuid.New()
	members := []domain.ProjectMember{*domain.NewInvite(projectID, uuid.New(), domain.RoleViewer)}

	h.memberRepo.EXPECT().ListByProject(mock.Anything, projectID).Return(members, nil)

	got, err := h.svc.ListMembers(ctx, projectID)
	require.NoError(t, err)
	require.Len(t, got, 1)
}

func TestInviteMember_HappyPath(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	projectID, userID := uuid.New(), uuid.New()

	h.memberRepo.EXPECT().FindByProjectAndUser(mock.Anything, projectID, userID).Return(nil, domain.ErrNotFound)
	h.memberRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(m *domain.ProjectMember) bool {
		return m.Status == domain.MemberStatusPending && m.Role == domain.RoleEditor
	})).Return(nil)

	member, err := h.svc.InviteMember(ctx, usecase.InviteMemberInput{
		ProjectID: projectID, UserID: userID, Role: domain.RoleEditor,
	})
	require.NoError(t, err)
	require.Equal(t, domain.MemberStatusPending, member.Status)
}

func TestInviteMember_AlreadyMember_ExistingRow_IsBusinessError(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	projectID, userID := uuid.New(), uuid.New()
	existing := domain.NewInvite(projectID, userID, domain.RoleViewer)

	h.memberRepo.EXPECT().FindByProjectAndUser(mock.Anything, projectID, userID).Return(existing, nil)

	_, err := h.svc.InviteMember(ctx, usecase.InviteMemberInput{
		ProjectID: projectID, UserID: userID, Role: domain.RoleEditor,
	})
	require.Error(t, err)
	be, ok := apperr.AsBusiness(err)
	require.True(t, ok, "mời user đã là thành viên phải là lỗi NGHIỆP VỤ")
	require.Equal(t, errcode.AlreadyMember, be.Code)
}

func TestInviteMember_DuplicateRace_IsBusinessError(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	projectID, userID := uuid.New(), uuid.New()

	// Race: FindByProjectAndUser không thấy, nhưng Create đụng unique constraint
	// (request khác chen giữa) -> vẫn phải map về ErrAlreadyMember.
	h.memberRepo.EXPECT().FindByProjectAndUser(mock.Anything, projectID, userID).Return(nil, domain.ErrNotFound)
	h.memberRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(domain.ErrDuplicate)

	_, err := h.svc.InviteMember(ctx, usecase.InviteMemberInput{
		ProjectID: projectID, UserID: userID, Role: domain.RoleViewer,
	})
	require.Error(t, err)
	be, ok := apperr.AsBusiness(err)
	require.True(t, ok)
	require.Equal(t, errcode.AlreadyMember, be.Code)
}

func TestChangeMemberRole_HappyPath(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	projectID, userID := uuid.New(), uuid.New()
	existing := domain.NewInvite(projectID, userID, domain.RoleViewer)
	existing.Status = domain.MemberStatusActive
	updated := domain.NewInvite(projectID, userID, domain.RoleEditor)
	updated.Status = domain.MemberStatusActive

	h.memberRepo.EXPECT().FindByProjectAndUser(mock.Anything, projectID, userID).Return(existing, nil).Once()
	h.memberRepo.EXPECT().UpdateRole(mock.Anything, projectID, userID, domain.RoleEditor).Return(nil)
	h.memberRepo.EXPECT().FindByProjectAndUser(mock.Anything, projectID, userID).Return(updated, nil).Once()

	got, err := h.svc.ChangeMemberRole(ctx, usecase.ChangeMemberRoleInput{
		ProjectID: projectID, UserID: userID, Role: domain.RoleEditor,
	})
	require.NoError(t, err)
	require.Equal(t, domain.RoleEditor, got.Role)
}

func TestChangeMemberRole_NotFound_IsTechnical404(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	projectID, userID := uuid.New(), uuid.New()

	h.memberRepo.EXPECT().FindByProjectAndUser(mock.Anything, projectID, userID).Return(nil, domain.ErrNotFound)

	_, err := h.svc.ChangeMemberRole(ctx, usecase.ChangeMemberRoleInput{
		ProjectID: projectID, UserID: userID, Role: domain.RoleEditor,
	})
	require.Error(t, err)
	te, ok := apperr.AsTechnical(err)
	require.True(t, ok)
	require.Equal(t, errcode.MemberNotFound, te.Code)
}

func TestChangeMemberRole_TargetIsOwner_IsBusinessError(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	projectID, ownerID := uuid.New(), uuid.New()
	owner := domain.NewInvite(projectID, ownerID, domain.RoleOwner)
	owner.Status = domain.MemberStatusActive

	h.memberRepo.EXPECT().FindByProjectAndUser(mock.Anything, projectID, ownerID).Return(owner, nil)

	_, err := h.svc.ChangeMemberRole(ctx, usecase.ChangeMemberRoleInput{
		ProjectID: projectID, UserID: ownerID, Role: domain.RoleViewer,
	})
	require.Error(t, err)
	be, ok := apperr.AsBusiness(err)
	require.True(t, ok, "đổi role của owner phải là lỗi NGHIỆP VỤ, không được thực thi")
	require.Equal(t, errcode.CannotModifyOwner, be.Code)
	h.memberRepo.AssertNotCalled(t, "UpdateRole", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestRemoveMember_HappyPath(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	projectID, userID := uuid.New(), uuid.New()
	existing := domain.NewInvite(projectID, userID, domain.RoleViewer)
	existing.Status = domain.MemberStatusActive

	h.memberRepo.EXPECT().FindByProjectAndUser(mock.Anything, projectID, userID).Return(existing, nil)
	h.memberRepo.EXPECT().Delete(mock.Anything, projectID, userID).Return(nil)

	require.NoError(t, h.svc.RemoveMember(ctx, projectID, userID))
}

func TestRemoveMember_NotFound_IsTechnical404(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	projectID, userID := uuid.New(), uuid.New()

	h.memberRepo.EXPECT().FindByProjectAndUser(mock.Anything, projectID, userID).Return(nil, domain.ErrNotFound)

	err := h.svc.RemoveMember(ctx, projectID, userID)
	require.Error(t, err)
	te, ok := apperr.AsTechnical(err)
	require.True(t, ok)
	require.Equal(t, errcode.MemberNotFound, te.Code)
}

func TestRemoveMember_TargetIsOwner_IsBusinessError(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	projectID, ownerID := uuid.New(), uuid.New()
	owner := domain.NewInvite(projectID, ownerID, domain.RoleOwner)
	owner.Status = domain.MemberStatusActive

	h.memberRepo.EXPECT().FindByProjectAndUser(mock.Anything, projectID, ownerID).Return(owner, nil)

	err := h.svc.RemoveMember(ctx, projectID, ownerID)
	require.Error(t, err)
	be, ok := apperr.AsBusiness(err)
	require.True(t, ok, "gỡ owner phải là lỗi NGHIỆP VỤ, không được thực thi")
	require.Equal(t, errcode.CannotModifyOwner, be.Code)
	h.memberRepo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything, mock.Anything)
}

func TestAcceptInvite_HappyPath(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	projectID, userID := uuid.New(), uuid.New()
	pending := domain.NewInvite(projectID, userID, domain.RoleViewer)
	fixedNow := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	h.memberRepo.EXPECT().FindByProjectAndUser(mock.Anything, projectID, userID).Return(pending, nil)
	h.clock.EXPECT().Now().Return(fixedNow)
	h.memberRepo.EXPECT().UpdateStatus(mock.Anything, mock.MatchedBy(func(m *domain.ProjectMember) bool {
		return m.Status == domain.MemberStatusActive && m.JoinedAt != nil && m.JoinedAt.Equal(fixedNow)
	})).Return(nil)

	got, err := h.svc.AcceptInvite(ctx, projectID, userID)
	require.NoError(t, err)
	require.Equal(t, domain.MemberStatusActive, got.Status)
}

func TestAcceptInvite_NotPending_IsBusinessError(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	projectID, userID := uuid.New(), uuid.New()
	active := domain.NewInvite(projectID, userID, domain.RoleViewer)
	active.Status = domain.MemberStatusActive

	h.memberRepo.EXPECT().FindByProjectAndUser(mock.Anything, projectID, userID).Return(active, nil)
	h.clock.EXPECT().Now().Return(time.Now())

	_, err := h.svc.AcceptInvite(ctx, projectID, userID)
	require.Error(t, err)
	be, ok := apperr.AsBusiness(err)
	require.True(t, ok)
	require.Equal(t, errcode.InviteNotPending, be.Code)
}

func TestAcceptInvite_NotFound_IsTechnical404(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	projectID, userID := uuid.New(), uuid.New()

	h.memberRepo.EXPECT().FindByProjectAndUser(mock.Anything, projectID, userID).Return(nil, domain.ErrNotFound)

	_, err := h.svc.AcceptInvite(ctx, projectID, userID)
	require.Error(t, err)
	te, ok := apperr.AsTechnical(err)
	require.True(t, ok)
	require.Equal(t, errcode.MemberNotFound, te.Code)
}

// --- Avatar ---

func TestRequestAvatarUpload_HappyPath(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	existing := domain.NewProject(uuid.New(), "RAG Demo", "", domain.ProjectSettings{})
	fixedNow := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	h.projectRepo.EXPECT().FindByID(mock.Anything, existing.ID).Return(existing, nil)
	h.objectStore.EXPECT().PresignedPutURL(mock.Anything, mock.AnythingOfType("string"), avatarPresignedTTL).
		Return("https://minio.local/put-url", nil)
	h.clock.EXPECT().Now().Return(fixedNow)

	result, err := h.svc.RequestAvatarUpload(ctx, usecase.RequestAvatarUploadInput{
		ProjectID: existing.ID, MimeType: "image/png", SizeBytes: 100,
	})
	require.NoError(t, err)
	require.Equal(t, "https://minio.local/put-url", result.UploadURL)
	require.Equal(t, fixedNow.Add(avatarPresignedTTL), result.ExpiresAt)
}

func TestRequestAvatarUpload_InvalidFormat_IsBusinessError(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	_, err := h.svc.RequestAvatarUpload(ctx, usecase.RequestAvatarUploadInput{
		ProjectID: uuid.New(), MimeType: "application/pdf", SizeBytes: 10,
	})
	require.Error(t, err)
	be, ok := apperr.AsBusiness(err)
	require.True(t, ok, "định dạng ảnh không hỗ trợ phải là lỗi NGHIỆP VỤ")
	require.Equal(t, errcode.ImageInvalid, be.Code)
}

func TestRequestAvatarUpload_TooLarge_IsBusinessError(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	_, err := h.svc.RequestAvatarUpload(ctx, usecase.RequestAvatarUploadInput{
		ProjectID: uuid.New(), MimeType: "image/png", SizeBytes: avatarMaxBytes + 1,
	})
	require.Error(t, err)
	be, ok := apperr.AsBusiness(err)
	require.True(t, ok, "vượt giới hạn dung lượng phải là lỗi NGHIỆP VỤ")
	require.Equal(t, errcode.FileTooLarge, be.Code)
}

func TestRequestAvatarUpload_ProjectNotFound_IsTechnical404(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	id := uuid.New()

	h.projectRepo.EXPECT().FindByID(mock.Anything, id).Return(nil, domain.ErrNotFound)

	_, err := h.svc.RequestAvatarUpload(ctx, usecase.RequestAvatarUploadInput{
		ProjectID: id, MimeType: "image/png", SizeBytes: 10,
	})
	require.Error(t, err)
	te, ok := apperr.AsTechnical(err)
	require.True(t, ok)
	require.Equal(t, errcode.ProjectNotFound, te.Code)
}

func TestCompleteAvatarUpload_HappyPath(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	existing := domain.NewProject(uuid.New(), "RAG Demo", "", domain.ProjectSettings{})

	h.projectRepo.EXPECT().FindByID(mock.Anything, existing.ID).Return(existing, nil)
	h.objectStore.EXPECT().Stat(mock.Anything, mock.AnythingOfType("string")).
		Return(port.StoredObject{}, nil)
	h.projectRepo.EXPECT().Update(mock.Anything, mock.MatchedBy(func(p *domain.Project) bool {
		return p.AvatarKey != ""
	})).Return(nil)

	got, err := h.svc.CompleteAvatarUpload(ctx, existing.ID)
	require.NoError(t, err)
	require.NotEmpty(t, got.AvatarKey)
}

func TestCompleteAvatarUpload_NotUploaded_IsBusinessError(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	existing := domain.NewProject(uuid.New(), "RAG Demo", "", domain.ProjectSettings{})

	h.projectRepo.EXPECT().FindByID(mock.Anything, existing.ID).Return(existing, nil)
	h.objectStore.EXPECT().Stat(mock.Anything, mock.AnythingOfType("string")).
		Return(port.StoredObject{}, port.ErrObjectNotFound)

	_, err := h.svc.CompleteAvatarUpload(ctx, existing.ID)
	require.Error(t, err)
	be, ok := apperr.AsBusiness(err)
	require.True(t, ok, "ảnh chưa lên storage phải là lỗi NGHIỆP VỤ")
	require.Equal(t, errcode.AvatarNotUploaded, be.Code)
	h.projectRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}

func TestCompleteAvatarUpload_NotFound_IsTechnical404(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	id := uuid.New()

	h.projectRepo.EXPECT().FindByID(mock.Anything, id).Return(nil, domain.ErrNotFound)

	_, err := h.svc.CompleteAvatarUpload(ctx, id)
	require.Error(t, err)
	te, ok := apperr.AsTechnical(err)
	require.True(t, ok)
	require.Equal(t, errcode.ProjectNotFound, te.Code)
}

func TestAvatarURL_EmptyKey_ReturnsEmptyNoCall(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	p := domain.NewProject(uuid.New(), "RAG Demo", "", domain.ProjectSettings{})

	url, err := h.svc.AvatarURL(ctx, p)
	require.NoError(t, err)
	require.Empty(t, url)
	h.objectStore.AssertNotCalled(t, "PresignedGetURL", mock.Anything, mock.Anything, mock.Anything)
}

func TestAvatarURL_WithKey_ReturnsPresignedURL(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	p := domain.NewProject(uuid.New(), "RAG Demo", "", domain.ProjectSettings{})
	p.SetAvatar("projects/x/avatar")

	h.objectStore.EXPECT().PresignedGetURL(mock.Anything, "projects/x/avatar", avatarPresignedTTL).
		Return("https://minio.local/get-url", nil)

	url, err := h.svc.AvatarURL(ctx, p)
	require.NoError(t, err)
	require.Equal(t, "https://minio.local/get-url", url)
}
