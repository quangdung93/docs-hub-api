// Package usecase chứa service tầng nghiệp vụ của module user: điều phối
// repository, transaction, cache, publish sự kiện. Trả BusinessError cho lỗi
// nghiệp vụ (không panic). KHÔNG import gin/gorm (depguard bảo vệ).
package usecase

import (
	"context"

	"github.com/google/uuid"

	"document-hub-api/internal/common/pagination"
	"document-hub-api/internal/common/port"
	"document-hub-api/internal/module/user/domain"
)

// PasswordHasher là hợp đồng băm mật khẩu mà usecase cần (implement bằng pkg/hashing).
type PasswordHasher interface {
	Hash(plain string) (string, error)
	Compare(hash, plain string) error
}

// Service là API nghiệp vụ của module user mà tầng delivery gọi.
type Service interface {
	Create(ctx context.Context, in CreateInput) (*domain.User, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	List(ctx context.Context, in ListInput) ([]domain.User, pagination.Meta, error)
	Update(ctx context.Context, in UpdateInput) (*domain.User, error)
	SetStatus(ctx context.Context, in SetStatusInput) (*domain.User, error)
	Delete(ctx context.Context, id uuid.UUID, version int) error
	EmailAvailable(ctx context.Context, email string) (bool, error)
}

// Deps gom phụ thuộc của service (struct thay vì nhiều tham số rời).
type Deps struct {
	Repo      domain.UserRepository
	Tx        port.TxManager
	Cache     port.Cache
	Publisher port.Publisher
	Hasher    PasswordHasher
	Clock     port.Clock
}

// service là implementation của Service.
type service struct {
	repo   domain.UserRepository
	tx     port.TxManager
	cache  port.Cache
	pub    port.Publisher
	hasher PasswordHasher
	clock  port.Clock
}

// NewService dựng service từ Deps.
func NewService(d Deps) Service {
	return &service{
		repo:   d.Repo,
		tx:     d.Tx,
		cache:  d.Cache,
		pub:    d.Publisher,
		hasher: d.Hasher,
		clock:  d.Clock,
	}
}

// --- Input types (không phải DTO HTTP; DTO nằm ở tầng delivery) ---

// CreateInput là tham số tạo user.
type CreateInput struct {
	Email    string
	FullName string
	Password string
	Roles    []string
}

// UpdateInput là tham số cập nhật hồ sơ (optimistic lock qua Version).
type UpdateInput struct {
	ID       uuid.UUID
	FullName string
	Version  int
}

// SetStatusInput là tham số đổi trạng thái.
type SetStatusInput struct {
	ID      uuid.UUID
	Status  domain.Status
	Version int
}

// ListInput là tham số liệt kê + phân trang.
type ListInput struct {
	Filter domain.Filter
	Page   pagination.Query
}
