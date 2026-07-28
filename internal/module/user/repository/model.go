// Package repository implement domain.UserRepository bằng GORM.
//
// Đây là NƠI DUY NHẤT trong slice user được import gorm. Entity nghiệp vụ
// (domain.User) được ánh xạ sang userModel (có tag gorm) qua mapper.go — nhờ đó
// tầng domain/usecase hoàn toàn không biết ORM.
package repository

import (
	"time"

	"gorm.io/gorm"
)

// userModel là ánh xạ bảng `users`. Cột/khóa/unique index được định nghĩa bằng
// SQL migration (golang-migrate); tag gorm ở đây chỉ mô tả kiểu để query đúng.
type userModel struct {
	ID           string `gorm:"type:char(36);primaryKey"`
	Email        string `gorm:"type:varchar(255);not null"`
	FullName     string `gorm:"type:varchar(255);not null"`
	PasswordHash string `gorm:"type:varchar(255);not null"`
	Status       string `gorm:"type:varchar(20);not null"`
	// RolesJSON lưu danh sách role dạng JSON (ví dụ ["admin","user"]).
	RolesJSON string `gorm:"column:roles;type:varchar(512)"`
	// Version phục vụ optimistic lock.
	Version   int            `gorm:"not null;default:1"`
	CreatedAt time.Time      `gorm:"autoCreateTime"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

// TableName cố định tên bảng.
func (userModel) TableName() string { return "users" }
