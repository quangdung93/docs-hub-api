package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	"github.com/quangdung93/docs-hub-api/internal/infrastructure/database/postgres"
	"github.com/quangdung93/docs-hub-api/internal/module/user/domain"
)

// userRepository implement domain.UserRepository. KHÔNG chứa business logic —
// chỉ CRUD + ánh xạ lỗi driver sang sentinel hợp đồng của domain.
type userRepository struct {
	db *gorm.DB
}

// New tạo repository. Trả về domain.UserRepository (interface).
func New(db *gorm.DB) domain.UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) Create(ctx context.Context, u *domain.User) error {
	model := fromDomain(u)
	if err := postgres.DBFrom(ctx, r.db).Create(model).Error; err != nil {
		return translate(err)
	}
	return nil
}

// Update cập nhật theo optimistic lock (khớp id + version), tự tăng version.
func (r *userRepository) Update(ctx context.Context, u *domain.User) error {
	res := postgres.DBFrom(ctx, r.db).Model(&userModel{}).
		Where("id = ? AND version = ?", u.ID.String(), u.Version).
		Updates(map[string]any{
			"full_name": u.FullName,
			"status":    string(u.Status),
			"roles":     encodeRoles(u.Roles),
			"version":   gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		return translate(res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNoRowsAffected
	}
	return nil
}

func (r *userRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	var m userModel
	if err := postgres.DBFrom(ctx, r.db).First(&m, "id = ?", id.String()).Error; err != nil {
		return nil, translate(err)
	}
	return toDomainOrWrap(&m)
}

func (r *userRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	var m userModel
	if err := postgres.DBFrom(ctx, r.db).First(&m, "email = ?", email).Error; err != nil {
		return nil, translate(err)
	}
	return toDomainOrWrap(&m)
}

func (r *userRepository) ExistsByEmail(ctx context.Context, email string, excludeID *uuid.UUID) (bool, error) {
	q := postgres.DBFrom(ctx, r.db).Model(&userModel{}).Where("email = ?", email)
	if excludeID != nil {
		q = q.Where("id <> ?", excludeID.String())
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return false, translate(err)
	}
	return count > 0, nil
}

func (r *userRepository) List(
	ctx context.Context, f domain.Filter, page pagination.Query,
) ([]domain.User, int64, error) {
	base := postgres.DBFrom(ctx, r.db).Model(&userModel{}).Scopes(scopeFilter(f))

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, translate(err)
	}
	if total == 0 {
		return []domain.User{}, 0, nil
	}

	var models []userModel
	if err := base.Scopes(scopeSort(page), scopePaginate(page)).Find(&models).Error; err != nil {
		return nil, 0, translate(err)
	}

	users, err := toDomainList(models)
	if err != nil {
		return nil, 0, fmt.Errorf("map danh sách user: %w", err)
	}
	return users, total, nil
}

// SoftDelete xóa mềm theo optimistic lock (khớp id + version).
func (r *userRepository) SoftDelete(ctx context.Context, id uuid.UUID, version int) error {
	res := postgres.DBFrom(ctx, r.db).
		Where("id = ? AND version = ?", id.String(), version).
		Delete(&userModel{})
	if res.Error != nil {
		return translate(res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNoRowsAffected
	}
	return nil
}

// translate chuyển lỗi hạ tầng (qua postgres.Translate) sang sentinel của domain.
func translate(err error) error {
	if err == nil {
		return nil
	}
	switch mapped := postgres.Translate(err); {
	case errors.Is(mapped, postgres.ErrNotFound):
		return domain.ErrNotFound
	case errors.Is(mapped, postgres.ErrDuplicateKey):
		return domain.ErrDuplicate
	default:
		return fmt.Errorf("user repository: %w", mapped)
	}
}

// toDomainOrWrap map model->domain, bọc lỗi parse để dễ trace.
func toDomainOrWrap(m *userModel) (*domain.User, error) {
	u, err := toDomain(m)
	if err != nil {
		return nil, fmt.Errorf("map user %s: %w", m.ID, err)
	}
	return &u, nil
}
