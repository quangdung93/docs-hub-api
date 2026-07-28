// Code generated MANUALLY theo phong cách mockery. Chạy `make mocks` để sinh lại.
package mocks

import (
	"context"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/quangdung93/docs-hub-api/internal/common/port"
	"github.com/quangdung93/docs-hub-api/internal/module/user/usecase"
)

// MockTxManager — port.TxManager. Do chạy luôn callback với cùng ctx (mô phỏng
// transaction thành công). Lỗi từ callback được trả nguyên vẹn.
type MockTxManager struct {
	mock.Mock
}

func (m *MockTxManager) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	m.Called(ctx)
	return fn(ctx)
}

// MockCache — port.Cache. Mặc định trả cache-miss để usecase rơi xuống DB.
type MockCache struct {
	mock.Mock
}

func (m *MockCache) Get(ctx context.Context, key string) (string, error) {
	args := m.Called(ctx, key)
	return args.String(0), args.Error(1)
}

func (m *MockCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return m.Called(ctx, key, value, ttl).Error(0)
}

func (m *MockCache) Del(ctx context.Context, keys ...string) error {
	return m.Called(ctx, keys).Error(0)
}

func (m *MockCache) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	args := m.Called(ctx, key, ttl)
	return args.Get(0).(int64), args.Error(1)
}

// MockPublisher — port.Publisher.
type MockPublisher struct {
	mock.Mock
}

func (m *MockPublisher) Publish(ctx context.Context, evt port.Event) error {
	return m.Called(ctx, evt).Error(0)
}

// MockHasher — usecase.PasswordHasher.
type MockHasher struct {
	mock.Mock
}

func (m *MockHasher) Hash(plain string) (string, error) {
	args := m.Called(plain)
	return args.String(0), args.Error(1)
}

func (m *MockHasher) Compare(hash, plain string) error {
	return m.Called(hash, plain).Error(0)
}

// FixedClock — port.Clock trả thời gian cố định cho test xác định.
type FixedClock struct {
	T time.Time
}

func (c FixedClock) Now() time.Time { return c.T }

// đảm bảo các mock thỏa interface tại compile-time.
var (
	_ port.TxManager         = (*MockTxManager)(nil)
	_ port.Cache             = (*MockCache)(nil)
	_ port.Publisher         = (*MockPublisher)(nil)
	_ usecase.PasswordHasher = (*MockHasher)(nil)
	_ port.Clock             = (*FixedClock)(nil)
)
