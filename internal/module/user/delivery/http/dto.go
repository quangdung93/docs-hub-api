// Package http là tầng delivery của module user: bind/validate request, gọi
// service, trả envelope. Handler phải MỎNG — không chứa business logic.
package http

import (
	"document-hub-api/internal/common/pagination"
	"document-hub-api/internal/module/user/domain"
	"document-hub-api/internal/module/user/usecase"
)

// CreateUserRequest là body tạo user. Tag json snake_case theo templates/02;
// tag binding để gin/validator kiểm tra.
type CreateUserRequest struct {
	Email    string   `json:"email"     binding:"required,email"`
	FullName string   `json:"full_name" binding:"required,min=1,max=255"`
	Password string   `json:"password"  binding:"required,min=8,max=72"`
	Roles    []string `json:"roles"     binding:"omitempty,dive,oneof=admin user"`
}

func (r CreateUserRequest) toInput() usecase.CreateInput {
	roles := r.Roles
	if len(roles) == 0 {
		roles = []string{"user"}
	}
	return usecase.CreateInput{
		Email:    r.Email,
		FullName: r.FullName,
		Password: r.Password,
		Roles:    roles,
	}
}

// UpdateUserRequest là body cập nhật hồ sơ. Version bắt buộc cho optimistic lock.
type UpdateUserRequest struct {
	FullName string `json:"full_name" binding:"required,min=1,max=255"`
	Version  int    `json:"version"   binding:"required,min=1"`
}

// UpdateStatusRequest là body đổi trạng thái.
type UpdateStatusRequest struct {
	Status  string `json:"status"  binding:"required,oneof=active inactive locked"`
	Version int    `json:"version" binding:"required,min=1"`
}

// DeleteUserRequest là body xóa (version cho optimistic lock).
type DeleteUserRequest struct {
	Version int `json:"version" binding:"required,min=1"`
}

// ListUserQuery là query string liệt kê. Nhúng pagination.Query (page/limit/sort_by/order).
type ListUserQuery struct {
	pagination.Query
	Keyword string `form:"keyword"`
	Status  string `form:"status" binding:"omitempty,oneof=active inactive locked"`
}

func (q ListUserQuery) toInput() usecase.ListInput {
	f := domain.Filter{}
	if q.Keyword != "" {
		kw := q.Keyword
		f.Keyword = &kw
	}
	if q.Status != "" {
		st := domain.Status(q.Status)
		f.Status = &st
	}
	return usecase.ListInput{Filter: f, Page: q.Query}
}

// CheckEmailQuery là query cho endpoint check-email.
type CheckEmailQuery struct {
	Email string `form:"email" binding:"required,email"`
}
