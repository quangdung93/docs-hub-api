package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// Mã lỗi SQLSTATE của PostgreSQL thường gặp.
const (
	codeUniqueViolation      = "23505"
	codeDeadlockDetected     = "40P01"
	codeSerializationFailure = "40001"
)

// Sentinel nội bộ tầng repository. Repository CHUYỂN lỗi GORM thô thành các
// sentinel này; usecase quyết định ý nghĩa nghiệp vụ (ví dụ ErrNoRows khi update
// là "không tồn tại" hay "version cũ").
var (
	ErrNotFound     = errors.New("postgres: không tìm thấy bản ghi")
	ErrDuplicateKey = errors.New("postgres: vi phạm ràng buộc duy nhất")
	ErrDeadlock     = errors.New("postgres: deadlock/serialization failure")
)

// Translate chuẩn hóa lỗi GORM/pgx thành sentinel của tầng repository.
// Lỗi không nhận diện được giữ nguyên (repository sẽ bọc thành apperr.Database).
func Translate(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case codeUniqueViolation:
			return ErrDuplicateKey
		case codeDeadlockDetected, codeSerializationFailure:
			return ErrDeadlock
		}
	}
	return err
}
