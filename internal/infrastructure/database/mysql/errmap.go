package mysql

import (
	"errors"

	driver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

// Mã lỗi MySQL thường gặp.
const (
	codeDuplicateEntry  = 1062
	codeDeadlock        = 1213
	codeLockWaitTimeout = 1205
)

// Sentinel nội bộ tầng repository. Repository CHUYỂN lỗi GORM thô thành các
// sentinel này; usecase quyết định ý nghĩa nghiệp vụ (ví dụ ErrNoRows khi update
// là "không tồn tại" hay "version cũ").
var (
	ErrNotFound     = errors.New("mysql: không tìm thấy bản ghi")
	ErrDuplicateKey = errors.New("mysql: vi phạm ràng buộc duy nhất")
	ErrDeadlock     = errors.New("mysql: deadlock/lock timeout")
)

// Translate chuẩn hóa lỗi GORM/driver thành sentinel của tầng repository.
// Lỗi không nhận diện được giữ nguyên (repository sẽ bọc thành apperr.Database).
func Translate(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}

	var myErr *driver.MySQLError
	if errors.As(err, &myErr) {
		switch myErr.Number {
		case codeDuplicateEntry:
			return ErrDuplicateKey
		case codeDeadlock, codeLockWaitTimeout:
			return ErrDeadlock
		}
	}
	return err
}
