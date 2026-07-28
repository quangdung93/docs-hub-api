// Code generated MANUALLY theo phong cách mockery. Chạy `make mocks` để sinh lại.
// (Viết tay để bộ test biên dịch được ngay khi chưa cài mockery.)
package mocks

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"github.com/quangdung393/docs-hub-api/internal/common/pagination"
	"github.com/quangdung393/docs-hub-api/internal/module/user/domain"
)

// MockUserRepository là mock của domain.UserRepository.
type MockUserRepository struct {
	mock.Mock
}

func (m *MockUserRepository) Create(ctx context.Context, u *domain.User) error {
	return m.Called(ctx, u).Error(0)
}

func (m *MockUserRepository) Update(ctx context.Context, u *domain.User) error {
	return m.Called(ctx, u).Error(0)
}

func (m *MockUserRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}

func (m *MockUserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}

func (m *MockUserRepository) ExistsByEmail(ctx context.Context, email string, excludeID *uuid.UUID) (bool, error) {
	args := m.Called(ctx, email, excludeID)
	return args.Bool(0), args.Error(1)
}

func (m *MockUserRepository) List(
	ctx context.Context, f domain.Filter, page pagination.Query,
) ([]domain.User, int64, error) {
	args := m.Called(ctx, f, page)
	var users []domain.User
	if args.Get(0) != nil {
		users = args.Get(0).([]domain.User)
	}
	return users, args.Get(1).(int64), args.Error(2)
}

func (m *MockUserRepository) SoftDelete(ctx context.Context, id uuid.UUID, version int) error {
	return m.Called(ctx, id, version).Error(0)
}
