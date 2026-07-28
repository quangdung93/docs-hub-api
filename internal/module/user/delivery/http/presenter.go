package http

import (
	"time"

	"document-hub-api/internal/module/user/domain"
)

// UserResponse là biểu diễn user trả ra client. CỐ TÌNH không có password_hash.
type UserResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	FullName  string    `json:"full_name"`
	Status    string    `json:"status"`
	Roles     []string  `json:"roles"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// toResponse ánh xạ entity -> DTO trả về.
func toResponse(u *domain.User) UserResponse {
	roles := u.Roles
	if roles == nil {
		roles = []string{}
	}
	return UserResponse{
		ID:        u.ID.String(),
		Email:     u.Email,
		FullName:  u.FullName,
		Status:    string(u.Status),
		Roles:     roles,
		Version:   u.Version,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}

// toResponseList ánh xạ danh sách.
func toResponseList(users []domain.User) []UserResponse {
	out := make([]UserResponse, 0, len(users))
	for i := range users {
		out = append(out, toResponse(&users[i]))
	}
	return out
}
