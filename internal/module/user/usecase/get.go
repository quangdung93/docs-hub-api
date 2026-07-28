package usecase

import (
	"context"

	"github.com/google/uuid"

	"document-hub-api/internal/common/apperr"
	"document-hub-api/internal/module/user/domain"
)

// GetByID lấy user theo id, có cache-aside:
//   - Cache hit -> trả ngay.
//   - Miss -> đọc DB; không có -> USR_404 (kỹ thuật, 404); có -> nạp cache.
func (s *service) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	if cached, ok := s.getCachedUser(ctx, id); ok {
		return cached, nil
	}

	user, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if isRepoErr(err, domain.ErrNotFound) {
			return nil, domain.ErrUserNotFound()
		}
		return nil, apperr.Database("Lỗi truy vấn người dùng").WithCause(err)
	}

	s.cacheUser(ctx, user)
	return user, nil
}

// EmailAvailable kiểm tra email còn trống không (cho endpoint check-email).
func (s *service) EmailAvailable(ctx context.Context, email string) (bool, error) {
	exists, err := s.repo.ExistsByEmail(ctx, email, nil)
	if err != nil {
		return false, apperr.Database("Lỗi kiểm tra email").WithCause(err)
	}
	return !exists, nil
}
