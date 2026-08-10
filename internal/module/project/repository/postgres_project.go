// Package repository implement domain.ProjectRepository và
// domain.ProjectMemberRepository bằng GORM.
//
// Đây là NƠI DUY NHẤT trong slice project được import gorm. Entity nghiệp vụ
// (domain.Project / domain.ProjectMember) được ánh xạ sang model có tag gorm —
// nhờ đó tầng domain/usecase hoàn toàn không biết ORM.
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	"github.com/quangdung93/docs-hub-api/internal/infrastructure/database/postgres"
	"github.com/quangdung93/docs-hub-api/internal/module/project/domain"
)

// --- Models (ánh xạ bảng, tag gorm chỉ mô tả kiểu — cột/khóa/index đã định
// nghĩa bằng SQL migration) ---

// projectModel ánh xạ bảng `projects`.
type projectModel struct {
	ID          string                 `gorm:"column:id;type:uuid;primaryKey"`
	OwnerID     string                 `gorm:"column:owner_id;type:uuid"`
	Name        string                 `gorm:"column:name;type:text;not null"`
	Description string                 `gorm:"column:description;type:text"`
	Status      string                 `gorm:"column:status;type:text;not null;default:active"`
	Settings    domain.ProjectSettings `gorm:"column:settings;type:jsonb;not null"`
	AvatarKey   string                 `gorm:"column:avatar_key;type:text"`
	CreatedAt   time.Time              `gorm:"column:created_at;autoCreateTime"`
}

func (projectModel) TableName() string { return "projects" }

// projectMemberModel ánh xạ bảng `project_members`.
type projectMemberModel struct {
	ID        string     `gorm:"column:id;type:uuid;primaryKey"`
	ProjectID string     `gorm:"column:project_id;type:uuid;not null"`
	UserID    string     `gorm:"column:user_id;type:uuid;not null"`
	Role      string     `gorm:"column:role;type:text;not null"`
	Status    string     `gorm:"column:status;type:text;not null;default:pending"`
	InvitedAt time.Time  `gorm:"column:invited_at;autoCreateTime"`
	JoinedAt  *time.Time `gorm:"column:joined_at"`
}

func (projectMemberModel) TableName() string { return "project_members" }

// sortableProjectColumns là WHITELIST cột được phép sort cho ListForUser — hàng
// rào chống SQL injection qua tham số sort_by (chỉ giá trị có trong map mới được
// ghép vào ORDER BY).
var sortableProjectColumns = map[string]string{ //nolint:gochecknoglobals // bảng tra cứu bất biến
	"created_at": "created_at",
	"name":       "name",
	"status":     "status",
}

// orderClause dựng ORDER BY an toàn (cột từ whitelist, order chỉ asc/desc).
func orderClause(p pagination.Query) string {
	col, ok := sortableProjectColumns[p.SortBy]
	if !ok {
		col = "created_at" // mặc định an toàn
	}
	order := "DESC"
	if p.Order == "asc" {
		order = "ASC"
	}
	return col + " " + order
}

// --- Mapper ---

func projectToDomain(m *projectModel) (domain.Project, error) {
	id, err := uuid.Parse(m.ID)
	if err != nil {
		return domain.Project{}, fmt.Errorf("parse project id: %w", err)
	}
	ownerID, err := uuid.Parse(m.OwnerID)
	if err != nil {
		return domain.Project{}, fmt.Errorf("parse owner id: %w", err)
	}
	return domain.Project{
		ID:          id,
		OwnerID:     ownerID,
		Name:        m.Name,
		Description: m.Description,
		Status:      m.Status,
		Settings:    m.Settings,
		AvatarKey:   m.AvatarKey,
		CreatedAt:   m.CreatedAt,
	}, nil
}

func projectFromDomain(p *domain.Project) *projectModel {
	return &projectModel{
		ID:          p.ID.String(),
		OwnerID:     p.OwnerID.String(),
		Name:        p.Name,
		Description: p.Description,
		Status:      p.Status,
		Settings:    p.Settings,
		AvatarKey:   p.AvatarKey,
		CreatedAt:   p.CreatedAt,
	}
}

func projectsToDomain(models []projectModel) ([]domain.Project, error) {
	out := make([]domain.Project, 0, len(models))
	for i := range models {
		p, err := projectToDomain(&models[i])
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func memberToDomain(m *projectMemberModel) (domain.ProjectMember, error) {
	id, err := uuid.Parse(m.ID)
	if err != nil {
		return domain.ProjectMember{}, fmt.Errorf("parse member id: %w", err)
	}
	projectID, err := uuid.Parse(m.ProjectID)
	if err != nil {
		return domain.ProjectMember{}, fmt.Errorf("parse project id: %w", err)
	}
	userID, err := uuid.Parse(m.UserID)
	if err != nil {
		return domain.ProjectMember{}, fmt.Errorf("parse user id: %w", err)
	}
	return domain.ProjectMember{
		ID:        id,
		ProjectID: projectID,
		UserID:    userID,
		Role:      domain.Role(m.Role),
		Status:    domain.MemberStatus(m.Status),
		InvitedAt: m.InvitedAt,
		JoinedAt:  m.JoinedAt,
	}, nil
}

func memberFromDomain(m *domain.ProjectMember) *projectMemberModel {
	return &projectMemberModel{
		ID:        m.ID.String(),
		ProjectID: m.ProjectID.String(),
		UserID:    m.UserID.String(),
		Role:      string(m.Role),
		Status:    string(m.Status),
		InvitedAt: m.InvitedAt,
		JoinedAt:  m.JoinedAt,
	}
}

func membersToDomain(models []projectMemberModel) ([]domain.ProjectMember, error) {
	out := make([]domain.ProjectMember, 0, len(models))
	for i := range models {
		m, err := memberToDomain(&models[i])
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// --- ProjectRepository ---

type projectRepository struct {
	db *gorm.DB
}

// NewProjectRepository tạo repository dự án. Trả về domain.ProjectRepository (interface).
func NewProjectRepository(db *gorm.DB) domain.ProjectRepository {
	return &projectRepository{db: db}
}

func (r *projectRepository) Create(ctx context.Context, p *domain.Project) error {
	model := projectFromDomain(p)
	if err := postgres.DBFrom(ctx, r.db).Create(model).Error; err != nil {
		return translate(err)
	}
	// GORM tự điền CreatedAt (autoCreateTime) vào model sau khi insert — đồng bộ
	// lại vào entity domain để caller (usecase) nhận giá trị thật, không phải zero value.
	p.CreatedAt = model.CreatedAt
	return nil
}

func (r *projectRepository) Update(ctx context.Context, p *domain.Project) error {
	res := postgres.DBFrom(ctx, r.db).Model(&projectModel{}).
		Where("id = ?", p.ID.String()).
		Updates(map[string]any{
			"name":        p.Name,
			"description": p.Description,
			"status":      p.Status,
			"settings":    p.Settings,
			"avatar_key":  p.AvatarKey,
		})
	if res.Error != nil {
		return translate(res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *projectRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Project, error) {
	var m projectModel
	if err := postgres.DBFrom(ctx, r.db).First(&m, "id = ?", id.String()).Error; err != nil {
		return nil, translate(err)
	}
	p, err := projectToDomain(&m)
	if err != nil {
		return nil, fmt.Errorf("map project %s: %w", m.ID, err)
	}
	return &p, nil
}

func (r *projectRepository) ListForUser(
	ctx context.Context, userID uuid.UUID, page pagination.Query,
) ([]domain.Project, int64, error) {
	base := postgres.DBFrom(ctx, r.db).Model(&projectModel{}).
		Where(
			"owner_id = ? OR id IN (SELECT project_id FROM project_members WHERE user_id = ? AND status = ?)",
			userID.String(), userID.String(), string(domain.MemberStatusActive),
		)

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, translate(err)
	}
	if total == 0 {
		return []domain.Project{}, 0, nil
	}

	var models []projectModel
	if err := base.Order(orderClause(page)).Limit(page.Limit).Offset(page.Offset()).Find(&models).Error; err != nil {
		return nil, 0, translate(err)
	}

	projects, err := projectsToDomain(models)
	if err != nil {
		return nil, 0, fmt.Errorf("map danh sách dự án: %w", err)
	}
	return projects, total, nil
}

func (r *projectRepository) Delete(ctx context.Context, id uuid.UUID) error {
	res := postgres.DBFrom(ctx, r.db).Where("id = ?", id.String()).Delete(&projectModel{})
	if res.Error != nil {
		return translate(res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// --- ProjectMemberRepository ---

type projectMemberRepository struct {
	db *gorm.DB
}

// NewProjectMemberRepository tạo repository thành viên dự án.
func NewProjectMemberRepository(db *gorm.DB) domain.ProjectMemberRepository {
	return &projectMemberRepository{db: db}
}

func (r *projectMemberRepository) Create(ctx context.Context, m *domain.ProjectMember) error {
	model := memberFromDomain(m)
	if err := postgres.DBFrom(ctx, r.db).Create(model).Error; err != nil {
		return translate(err)
	}
	// GORM tự điền InvitedAt (autoCreateTime) vào model sau khi insert — đồng bộ
	// lại vào entity domain để caller nhận giá trị thật, không phải zero value.
	m.InvitedAt = model.InvitedAt
	return nil
}

func (r *projectMemberRepository) FindByProjectAndUser(
	ctx context.Context, projectID, userID uuid.UUID,
) (*domain.ProjectMember, error) {
	var m projectMemberModel
	err := postgres.DBFrom(ctx, r.db).
		First(&m, "project_id = ? AND user_id = ?", projectID.String(), userID.String()).Error
	if err != nil {
		return nil, translate(err)
	}
	member, err := memberToDomain(&m)
	if err != nil {
		return nil, fmt.Errorf("map project member %s: %w", m.ID, err)
	}
	return &member, nil
}

func (r *projectMemberRepository) ListByProject(
	ctx context.Context, projectID uuid.UUID,
) ([]domain.ProjectMember, error) {
	var models []projectMemberModel
	err := postgres.DBFrom(ctx, r.db).
		Where("project_id = ?", projectID.String()).
		Order("invited_at ASC").
		Find(&models).Error
	if err != nil {
		return nil, translate(err)
	}
	members, err := membersToDomain(models)
	if err != nil {
		return nil, fmt.Errorf("map danh sách thành viên: %w", err)
	}
	return members, nil
}

func (r *projectMemberRepository) UpdateRole(
	ctx context.Context, projectID, userID uuid.UUID, role domain.Role,
) error {
	res := postgres.DBFrom(ctx, r.db).Model(&projectMemberModel{}).
		Where("project_id = ? AND user_id = ?", projectID.String(), userID.String()).
		Update("role", string(role))
	if res.Error != nil {
		return translate(res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNoRowsAffected
	}
	return nil
}

func (r *projectMemberRepository) UpdateStatus(ctx context.Context, m *domain.ProjectMember) error {
	res := postgres.DBFrom(ctx, r.db).Model(&projectMemberModel{}).
		Where("project_id = ? AND user_id = ?", m.ProjectID.String(), m.UserID.String()).
		Updates(map[string]any{
			"status":    string(m.Status),
			"joined_at": m.JoinedAt,
		})
	if res.Error != nil {
		return translate(res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNoRowsAffected
	}
	return nil
}

func (r *projectMemberRepository) Delete(ctx context.Context, projectID, userID uuid.UUID) error {
	res := postgres.DBFrom(ctx, r.db).
		Where("project_id = ? AND user_id = ?", projectID.String(), userID.String()).
		Delete(&projectMemberModel{})
	if res.Error != nil {
		return translate(res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// translate chuyển lỗi hạ tầng (qua postgres.Translate) sang sentinel của domain.
func translate(err error) error {
	if err == nil {
		return nil
	}
	switch mapped := postgres.Translate(err); {
	case errors.Is(mapped, postgres.ErrNotFound):
		return domain.ErrNotFound
	case errors.Is(mapped, postgres.ErrDuplicateKey):
		return domain.ErrDuplicate
	default:
		return fmt.Errorf("project repository: %w", mapped)
	}
}
