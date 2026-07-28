package postgres

import (
	"context"

	"gorm.io/gorm"
)

type txCtxKey struct{}

// withTx nhét *gorm.DB (đang trong transaction) vào context.
func withTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, txCtxKey{}, tx)
}

// DBFrom trả về handle DB đúng ngữ cảnh:
//   - Nếu ctx đang trong transaction (do TxManager đặt) -> trả tx đó.
//   - Ngược lại -> trả base.WithContext(ctx) (query độc lập, vẫn gắn context).
//
// Repository LUÔN gọi DBFrom(ctx, r.db) thay vì dùng r.db trực tiếp, nhờ đó tự
// động tham gia transaction mà usecase không cần biết.
func DBFrom(ctx context.Context, base *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(txCtxKey{}).(*gorm.DB); ok && tx != nil {
		return tx
	}
	return base.WithContext(ctx)
}
