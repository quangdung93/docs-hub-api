package usecase

import (
	"context"

	"document-hub-api/internal/common/apperr"
	"document-hub-api/internal/module/user/domain"
)

// SetStatus đổi trạng thái user (active/inactive/locked) theo optimistic lock.
func (s *service) SetStatus(ctx context.Context, in SetStatusInput) (*domain.User, error) {
	current, err := s.repo.FindByID(ctx, in.ID)
	if err != nil {
		if isRepoErr(err, domain.ErrNotFound) {
			return nil, domain.ErrUserNotFound()
		}
		return nil, apperr.Database("Lỗi truy vấn người dùng").WithCause(err)
	}

	if statusErr := current.SetStatus(in.Status); statusErr != nil {
		return nil, statusErr // ErrInvalidStatus (nghiệp vụ)
	}
	current.Version = in.Version

	if updErr := s.repo.Update(ctx, current); updErr != nil {
		return nil, s.resolveWriteConflict(ctx, in.ID, updErr)
	}

	s.invalidateUser(ctx, in.ID)
	return s.reload(ctx, in.ID)
}
