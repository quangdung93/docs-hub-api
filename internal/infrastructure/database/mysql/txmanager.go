package mysql

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/quangdung393/docs-hub-api/internal/common/port"
)

// TxManager implement port.TxManager bằng transaction của GORM.
type TxManager struct {
	db *gorm.DB
}

// NewTxManager tạo TxManager. Trả về port.TxManager (interface) để usecase
// không phụ thuộc kiểu cụ thể.
func NewTxManager(db *gorm.DB) port.TxManager {
	return &TxManager{db: db}
}

// Do chạy fn trong một transaction. Handle transaction được đặt vào context để
// các repository dùng DBFrom tự động tham gia. fn trả lỗi -> rollback.
func (m *TxManager) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(withTx(ctx, tx))
	})
	if err != nil {
		return fmt.Errorf("transaction thất bại: %w", err)
	}
	return nil
}
