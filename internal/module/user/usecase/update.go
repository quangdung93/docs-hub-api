package usecase

import (
	"context"

	"github.com/google/uuid"

	"github.com/quangdung393/docs-hub-api/internal/common/apperr"
	"github.com/quangdung393/docs-hub-api/internal/module/user/domain"
)

// Update cập nhật hồ sơ theo optimistic lock. Khi repo báo không có dòng nào bị
// tác động, usecase phân biệt: bản ghi TỒN TẠI -> xung đột version (nghiệp vụ);
// KHÔNG tồn tại -> USR_404 (kỹ thuật). Đây là lý do logic này nằm ở usecase,
// không ở repository.
func (s *service) Update(ctx context.Context, in UpdateInput) (*domain.User, error) {
	current, err := s.repo.FindByID(ctx, in.ID)
	if err != nil {
		if isRepoErr(err, domain.ErrNotFound) {
			return nil, domain.ErrUserNotFound()
		}
		return nil, apperr.Database("Lỗi truy vấn người dùng").WithCause(err)
	}

	if changeErr := current.ChangeProfile(in.FullName); changeErr != nil {
		return nil, changeErr // ví dụ ErrUserLocked (nghiệp vụ)
	}
	current.Version = in.Version // version client gửi để so khớp

	if updErr := s.repo.Update(ctx, current); updErr != nil {
		return nil, s.resolveWriteConflict(ctx, in.ID, updErr)
	}

	s.invalidateUser(ctx, in.ID)
	// Đọc lại để trả version mới nhất.
	return s.reload(ctx, in.ID)
}

// resolveWriteConflict biến ErrNoRowsAffected thành lỗi đúng ngữ nghĩa.
func (s *service) resolveWriteConflict(ctx context.Context, id uuid.UUID, err error) error {
	if !isRepoErr(err, domain.ErrNoRowsAffected) {
		return apperr.Database("Lỗi cập nhật người dùng").WithCause(err)
	}
	// Không tác động dòng nào: do version cũ hay do bản ghi đã biến mất?
	if _, findErr := s.repo.FindByID(ctx, id); findErr != nil {
		if isRepoErr(findErr, domain.ErrNotFound) {
			return domain.ErrUserNotFound()
		}
		return apperr.Database("Lỗi truy vấn người dùng").WithCause(findErr)
	}
	return domain.ErrVersionConflict
}

// reload đọc lại user sau khi ghi (bỏ qua cache).
func (s *service) reload(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	u, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, apperr.Database("Lỗi tải lại người dùng").WithCause(err)
	}
	s.cacheUser(ctx, u)
	return u, nil
}
