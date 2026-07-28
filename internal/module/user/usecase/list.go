package usecase

import (
	"context"

	"github.com/quangdung393/docs-hub-api/internal/common/apperr"
	"github.com/quangdung393/docs-hub-api/internal/common/pagination"
	"github.com/quangdung393/docs-hub-api/internal/module/user/domain"
)

// List liệt kê user theo filter + phân trang. Trả kèm pagination.Meta để tầng
// delivery đưa vào envelope.
func (s *service) List(ctx context.Context, in ListInput) ([]domain.User, pagination.Meta, error) {
	page := in.Page.Normalize()

	users, total, err := s.repo.List(ctx, in.Filter, page)
	if err != nil {
		return nil, pagination.Meta{}, apperr.Database("Lỗi truy vấn danh sách người dùng").WithCause(err)
	}

	meta := pagination.NewMeta(page.Page, page.Limit, total)
	return users, meta, nil
}
