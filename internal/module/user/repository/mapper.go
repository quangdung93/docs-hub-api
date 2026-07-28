package repository

import (
	"encoding/json"

	"github.com/google/uuid"

	"document-hub-api/internal/module/user/domain"
)

// toDomain chuyển bản ghi DB sang entity nghiệp vụ (trả VALUE để tránh copylocks
// khi build danh sách; caller cần con trỏ tự lấy địa chỉ).
func toDomain(m *userModel) (domain.User, error) {
	id, err := uuid.Parse(m.ID)
	if err != nil {
		return domain.User{}, err
	}
	return domain.User{
		ID:           id,
		Email:        m.Email,
		FullName:     m.FullName,
		PasswordHash: m.PasswordHash,
		Status:       domain.Status(m.Status),
		Roles:        decodeRoles(m.RolesJSON),
		Version:      m.Version,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}, nil
}

// fromDomain chuyển entity nghiệp vụ sang bản ghi DB.
func fromDomain(u *domain.User) *userModel {
	return &userModel{
		ID:           u.ID.String(),
		Email:        u.Email,
		FullName:     u.FullName,
		PasswordHash: u.PasswordHash,
		Status:       string(u.Status),
		RolesJSON:    encodeRoles(u.Roles),
		Version:      u.Version,
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
	}
}

// toDomainList map hàng loạt, bỏ qua bản ghi lỗi parse (không nên xảy ra).
func toDomainList(models []userModel) ([]domain.User, error) {
	out := make([]domain.User, 0, len(models))
	for i := range models {
		u, err := toDomain(&models[i])
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, nil
}

func encodeRoles(roles []string) string {
	if len(roles) == 0 {
		return "[]"
	}
	b, err := json.Marshal(roles)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func decodeRoles(raw string) []string {
	if raw == "" {
		return nil
	}
	var roles []string
	if err := json.Unmarshal([]byte(raw), &roles); err != nil {
		return nil
	}
	return roles
}
