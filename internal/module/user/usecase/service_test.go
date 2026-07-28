package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"document-hub-api/internal/common/apperr"
	"document-hub-api/internal/common/errcode"
	"document-hub-api/internal/common/port"
	"document-hub-api/internal/module/user/domain"
	"document-hub-api/internal/module/user/mocks"
	"document-hub-api/internal/module/user/usecase"
)

// harness gom service + các mock để test tiện lợi.
type harness struct {
	svc   usecase.Service
	repo  *mocks.MockUserRepository
	tx    *mocks.MockTxManager
	cache *mocks.MockCache
	pub   *mocks.MockPublisher
	hash  *mocks.MockHasher
}

func newHarness() *harness {
	repo := &mocks.MockUserRepository{}
	tx := &mocks.MockTxManager{}
	cache := &mocks.MockCache{}
	pub := &mocks.MockPublisher{}
	hash := &mocks.MockHasher{}

	// Cache là best-effort: cho phép mọi lời gọi, không bắt buộc.
	cache.On("Get", mock.Anything, mock.Anything).Return("", errCacheMiss()).Maybe()
	cache.On("Set", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	cache.On("Del", mock.Anything, mock.Anything).Return(nil).Maybe()
	tx.On("Do", mock.Anything).Return().Maybe()

	svc := usecase.NewService(usecase.Deps{
		Repo: repo, Tx: tx, Cache: cache, Publisher: pub, Hasher: hash,
		Clock: mocks.FixedClock{T: time.Unix(0, 0).UTC()},
	})
	return &harness{svc, repo, tx, cache, pub, hash}
}

func errCacheMiss() error {
	// port.ErrCacheMiss được usecase kiểm tra bằng errors.Is -> rơi xuống DB.
	return port.ErrCacheMiss
}

func TestCreate_HappyPath(t *testing.T) {
	h := newHarness()
	ctx := context.Background()

	h.repo.On("ExistsByEmail", ctx, "a@b.com", (*uuid.UUID)(nil)).Return(false, nil)
	h.hash.On("Hash", "P@ss123").Return("hashed", nil)
	h.repo.On("Create", mock.Anything, mock.AnythingOfType("*domain.User")).Return(nil)
	h.pub.On("Publish", mock.Anything, mock.Anything).Return(nil)

	u, err := h.svc.Create(ctx, usecase.CreateInput{
		Email: "a@b.com", FullName: "A", Password: "P@ss123", Roles: []string{"user"},
	})
	require.NoError(t, err)
	require.Equal(t, "a@b.com", u.Email)
	require.Equal(t, "hashed", u.PasswordHash)
	require.Equal(t, domain.StatusActive, u.Status)
	h.pub.AssertCalled(t, "Publish", mock.Anything, mock.Anything)
}

func TestCreate_DuplicateEmail_IsBusinessError(t *testing.T) {
	h := newHarness()
	ctx := context.Background()

	h.repo.On("ExistsByEmail", ctx, "dup@b.com", (*uuid.UUID)(nil)).Return(true, nil)

	_, err := h.svc.Create(ctx, usecase.CreateInput{Email: "dup@b.com", Password: "x"})
	require.Error(t, err)

	be, ok := apperr.AsBusiness(err)
	require.True(t, ok, "trùng email phải là lỗi NGHIỆP VỤ")
	require.Equal(t, errcode.DuplicateEmail, be.Code)
	h.hash.AssertNotCalled(t, "Hash", mock.Anything)
}

func TestGetByID_NotFound_IsTechnical404(t *testing.T) {
	h := newHarness()
	ctx := context.Background()
	id := uuid.New()

	h.repo.On("FindByID", mock.Anything, id).Return(nil, domain.ErrNotFound)

	_, err := h.svc.GetByID(ctx, id)
	require.Error(t, err)

	te, ok := apperr.AsTechnical(err)
	require.True(t, ok, "không tìm thấy phải là lỗi KỸ THUẬT (404)")
	require.Equal(t, errcode.UserNotFound, te.Code)
}

func TestUpdate_VersionConflict_IsBusinessError(t *testing.T) {
	h := newHarness()
	ctx := context.Background()
	id := uuid.New()
	existing := domain.NewUser("a@b.com", "A", "h", nil)
	existing.ID = id
	existing.Version = 3

	// FindByID: lần đầu (trước update) và lần sau (resolveWriteConflict) đều thấy bản ghi.
	h.repo.On("FindByID", mock.Anything, id).Return(existing, nil)
	// Update trả ErrNoRowsAffected -> nghi ngờ version cũ.
	h.repo.On("Update", mock.Anything, mock.Anything).Return(domain.ErrNoRowsAffected)

	_, err := h.svc.Update(ctx, usecase.UpdateInput{ID: id, FullName: "Tên mới", Version: 1})
	require.Error(t, err)

	be, ok := apperr.AsBusiness(err)
	require.True(t, ok, "xung đột version phải là lỗi NGHIỆP VỤ")
	require.Equal(t, errcode.ConflictVersion, be.Code)
}

func TestSetStatus_Locked_ThenUpdateSucceeds(t *testing.T) {
	h := newHarness()
	ctx := context.Background()
	id := uuid.New()
	existing := domain.NewUser("a@b.com", "A", "h", nil)
	existing.ID = id
	existing.Version = 1

	h.repo.On("FindByID", mock.Anything, id).Return(existing, nil)
	h.repo.On("Update", mock.Anything, mock.MatchedBy(func(u *domain.User) bool {
		return u.Status == domain.StatusLocked
	})).Return(nil)

	u, err := h.svc.SetStatus(ctx, usecase.SetStatusInput{
		ID: id, Status: domain.StatusLocked, Version: 1,
	})
	require.NoError(t, err)
	require.Equal(t, domain.StatusLocked, u.Status)
}

func TestSetStatus_InvalidStatus_IsBusinessError(t *testing.T) {
	h := newHarness()
	ctx := context.Background()
	id := uuid.New()
	existing := domain.NewUser("a@b.com", "A", "h", nil)
	existing.ID = id

	h.repo.On("FindByID", mock.Anything, id).Return(existing, nil)

	_, err := h.svc.SetStatus(ctx, usecase.SetStatusInput{ID: id, Status: "banana", Version: 1})
	require.Error(t, err)
	require.True(t, apperr.IsBusiness(err))
}

func TestDelete_NotFound_IsTechnical404(t *testing.T) {
	h := newHarness()
	ctx := context.Background()
	id := uuid.New()

	h.repo.On("SoftDelete", mock.Anything, id, 1).Return(domain.ErrNoRowsAffected)
	h.repo.On("FindByID", mock.Anything, id).Return(nil, domain.ErrNotFound)

	err := h.svc.Delete(ctx, id, 1)
	require.Error(t, err)
	te, ok := apperr.AsTechnical(err)
	require.True(t, ok)
	require.Equal(t, errcode.UserNotFound, te.Code)
}
