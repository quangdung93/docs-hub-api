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
	ID                string                 `gorm:"column:id;type:uuid;primaryKey"`
	OwnerID           string                 `gorm:"column:owner_id;type:uuid"`
	Code              string                 `gorm:"column:code;type:varchar(64);not null"`
	Name              string                 `gorm:"column:name;type:text;not null"`
	Description       string                 `gorm:"column:description;type:text"`
	Status            string                 `gorm:"column:status;type:text;not null;default:active"`
	Settings          domain.ProjectSettings `gorm:"column:settings;type:jsonb;not null"`
	AvatarKey         string                 `gorm:"column:avatar_key;type:text"`
	Version           int                    `gorm:"column:version;not null;default:1"`
	RAGFlowDatasetID  *string                `gorm:"column:ragflow_dataset_id"`
	RAGFlowSyncStatus string                 `gorm:"column:ragflow_sync_status"`
	RAGFlowLastError  string                 `gorm:"column:ragflow_last_error"`
	CreatedAt         time.Time              `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt         time.Time              `gorm:"column:updated_at;autoUpdateTime"`
}

func (projectModel) TableName() string { return "projects" }

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func stringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

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

// Tên cột lặp lại ở nhiều truy vấn — gom thành hằng để tránh gõ sai.
const (
	colCreatedAt = "created_at"
	colName      = "name"
	colStatus    = "status"
)

// sortableProjectColumns là WHITELIST cột được phép sort cho ListForUser — hàng
// rào chống SQL injection qua tham số sort_by (chỉ giá trị có trong map mới được
// ghép vào ORDER BY).
var sortableProjectColumns = map[string]string{ //nolint:gochecknoglobals // bảng tra cứu bất biến
	"created_at": colCreatedAt,
	"name":       colName,
	"status":     colStatus,
}

// orderClause dựng ORDER BY an toàn (cột từ whitelist, order chỉ asc/desc).
func orderClause(p pagination.Query) string {
	col, ok := sortableProjectColumns[p.SortBy]
	if !ok {
		col = colCreatedAt // mặc định an toàn
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
		ID:                id,
		OwnerID:           ownerID,
		Code:              m.Code,
		Name:              m.Name,
		Description:       m.Description,
		Status:            m.Status,
		Settings:          m.Settings,
		AvatarKey:         m.AvatarKey,
		Version:           m.Version,
		RAGFlowDatasetID:  stringOrEmpty(m.RAGFlowDatasetID),
		RAGFlowSyncStatus: m.RAGFlowSyncStatus,
		RAGFlowLastError:  m.RAGFlowLastError,
		CreatedAt:         m.CreatedAt,
		UpdatedAt:         m.UpdatedAt,
	}, nil
}

func projectFromDomain(p *domain.Project) *projectModel {
	return &projectModel{
		ID:                p.ID.String(),
		OwnerID:           p.OwnerID.String(),
		Code:              p.Code,
		Name:              p.Name,
		Description:       p.Description,
		Status:            p.Status,
		Settings:          p.Settings,
		AvatarKey:         p.AvatarKey,
		Version:           p.Version,
		RAGFlowDatasetID:  nullableString(p.RAGFlowDatasetID),
		RAGFlowSyncStatus: p.RAGFlowSyncStatus,
		RAGFlowLastError:  p.RAGFlowLastError,
		CreatedAt:         p.CreatedAt,
		UpdatedAt:         p.UpdatedAt,
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
	p.UpdatedAt = model.UpdatedAt
	return nil
}

func (r *projectRepository) Update(ctx context.Context, p *domain.Project) error {
	res := postgres.DBFrom(ctx, r.db).Model(&projectModel{}).
		Where("id = ?", p.ID.String()).
		Updates(map[string]any{
			colName:               p.Name,
			"description":         p.Description,
			colStatus:             p.Status,
			"settings":            p.Settings,
			"avatar_key":          p.AvatarKey,
			"ragflow_dataset_id":  nullableString(p.RAGFlowDatasetID),
			"ragflow_sync_status": p.RAGFlowSyncStatus,
			"ragflow_last_error":  p.RAGFlowLastError,
			"updated_at":          gorm.Expr("now()"),
		})
	if res.Error != nil {
		return translate(res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// statsRow là một dòng kết quả đếm cho một dự án.
type statsRow struct {
	ProjectID     string `gorm:"column:project_id"`
	DocumentCount int64  `gorm:"column:document_count"`
	MemberCount   int64  `gorm:"column:member_count"`
	ChunkCount    int64  `gorm:"column:chunk_count"`
}

// Stats đếm bằng subquery tương quan thay vì JOIN nhiều bảng: JOIN ba bảng
// một lúc sẽ nhân bản dòng và làm sai kết quả đếm (fan-out). Cả ba bảng đều
// đã có index trên project_id.
//
// documents dùng xóa mềm nên PHẢI lọc deleted_at IS NULL — đây là SQL thô, GORM
// không tự áp scope soft-delete. Thiếu điều kiện này thì tài liệu đã xóa vẫn
// được đếm (đã xảy ra thật: danh sách rỗng nhưng document_count vẫn báo 5).
// project_members và document_chunks không có cột deleted_at nên đếm thẳng.
func (r *projectRepository) Stats(
	ctx context.Context, ids []uuid.UUID,
) (map[uuid.UUID]domain.ProjectStats, error) {
	out := make(map[uuid.UUID]domain.ProjectStats, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	strIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		strIDs = append(strIDs, id.String())
	}

	const statsSQL = `
		SELECT p.id AS project_id,
		       (SELECT count(*) FROM documents d
		         WHERE d.project_id = p.id AND d.deleted_at IS NULL) AS document_count,
		       (SELECT count(*) FROM project_members m WHERE m.project_id = p.id) AS member_count,
		       (SELECT count(*) FROM document_chunks c WHERE c.project_id = p.id) AS chunk_count
		FROM projects p WHERE p.id IN ?`

	var rows []statsRow
	if err := postgres.DBFrom(ctx, r.db).Raw(statsSQL, strIDs).Scan(&rows).Error; err != nil {
		return nil, translate(err)
	}

	for _, row := range rows {
		id, err := uuid.Parse(row.ProjectID)
		if err != nil {
			return nil, fmt.Errorf("parse project id %q: %w", row.ProjectID, err)
		}
		out[id] = domain.ProjectStats{
			DocumentCount: row.DocumentCount,
			MemberCount:   row.MemberCount,
			ChunkCount:    row.ChunkCount,
		}
	}
	return out, nil
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

// memberWithUserRow là kết quả join project_members với users.
//
// Khai báo PHẲNG chứ không nhúng projectMemberModel: Scan của GORM không đổ
// dữ liệu vào struct lồng (kể cả có thẻ embedded), kết quả là mọi trường đều
// rỗng mà không báo lỗi gì.
type memberWithUserRow struct {
	ID        string     `gorm:"column:id"`
	ProjectID string     `gorm:"column:project_id"`
	UserID    string     `gorm:"column:user_id"`
	Role      string     `gorm:"column:role"`
	Status    string     `gorm:"column:status"`
	InvitedAt time.Time  `gorm:"column:invited_at"`
	JoinedAt  *time.Time `gorm:"column:joined_at"`
	FullName  string     `gorm:"column:full_name"`
	Email     string     `gorm:"column:email"`
}

// ListByProject trả kèm tên và email trong MỘT truy vấn. LEFT JOIN chứ không
// INNER: user bị xóa thì vẫn phải thấy bản ghi thành viên, chỉ là thiếu tên.
func (r *projectMemberRepository) ListByProject(
	ctx context.Context, projectID uuid.UUID,
) ([]domain.MemberWithUser, error) {
	var rows []memberWithUserRow
	err := postgres.DBFrom(ctx, r.db).
		Table("project_members AS pm").
		Select("pm.*, u.full_name, u.email").
		Joins("LEFT JOIN users AS u ON u.id = pm.user_id").
		Where("pm.project_id = ?", projectID.String()).
		Order("pm.invited_at ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, translate(err)
	}

	out := make([]domain.MemberWithUser, 0, len(rows))
	for _, row := range rows {
		member, mapErr := memberToDomain(&projectMemberModel{
			ID: row.ID, ProjectID: row.ProjectID, UserID: row.UserID,
			Role: row.Role, Status: row.Status,
			InvitedAt: row.InvitedAt, JoinedAt: row.JoinedAt,
		})
		if mapErr != nil {
			return nil, fmt.Errorf("map danh sách thành viên: %w", mapErr)
		}
		out = append(out, domain.MemberWithUser{
			ProjectMember: member,
			FullName:      row.FullName,
			Email:         row.Email,
		})
	}
	return out, nil
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
			colStatus:   string(m.Status),
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
