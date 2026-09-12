// Package repository lưu phân tích edge case URD bằng PostgreSQL/GORM.
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/internal/infrastructure/database/postgres"
	"github.com/quangdung93/docs-hub-api/internal/module/urd/domain"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

type analysisModel struct {
	ID, DocumentID, DocumentRevisionID string
	Status                             string
	TotalCases, ResolvedCases          int
	CreatedBy                          string
	ErrorCode                          string
	ErrorDetail                        string `gorm:"column:error_detail_sanitized"`
	CreatedAt, UpdatedAt               time.Time
}

func (analysisModel) TableName() string { return "urd_analyses" }

type edgeCaseModel struct {
	ID, AnalysisID string
	SequenceNo     int
	Description    string
	Resolution     string
	ImageObjectKey *string
	Resolved       bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (edgeCaseModel) TableName() string { return "urd_edge_cases" }

func (r *Repository) CreateAnalysis(ctx context.Context, a domain.Analysis, cases []domain.EdgeCase) error {
	db := postgres.DBFrom(ctx, r.db)
	am := fromAnalysis(a)
	if err := db.Create(&am).Error; err != nil {
		return fmt.Errorf("tạo phân tích edge case: %w", postgres.Translate(err))
	}
	if len(cases) == 0 {
		return nil
	}
	models := make([]edgeCaseModel, len(cases))
	for i, c := range cases {
		models[i] = fromEdgeCase(c)
	}
	if err := db.Create(&models).Error; err != nil {
		return fmt.Errorf("tạo edge case: %w", postgres.Translate(err))
	}
	return nil
}

func (r *Repository) GetActiveAnalysis(ctx context.Context, documentID uuid.UUID) (*domain.Analysis, []domain.EdgeCase, error) {
	var am analysisModel
	err := postgres.DBFrom(ctx, r.db).
		Where("document_id=? AND status IN ('analyzing','awaiting_input')", documentID).
		Order("created_at DESC").First(&am).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("đọc phân tích đang hoạt động: %w", postgres.Translate(err))
	}
	cases, err := r.listCases(ctx, am.ID)
	if err != nil {
		return nil, nil, err
	}
	a := toAnalysis(am)
	return &a, cases, nil
}

func (r *Repository) GetAnalysis(ctx context.Context, analysisID uuid.UUID) (*domain.Analysis, []domain.EdgeCase, error) {
	var am analysisModel
	if err := postgres.DBFrom(ctx, r.db).First(&am, "id=?", analysisID).Error; err != nil {
		return nil, nil, mapErr(err)
	}
	cases, err := r.listCases(ctx, am.ID)
	if err != nil {
		return nil, nil, err
	}
	a := toAnalysis(am)
	return &a, cases, nil
}

func (r *Repository) listCases(ctx context.Context, analysisID string) ([]domain.EdgeCase, error) {
	var models []edgeCaseModel
	if err := postgres.DBFrom(ctx, r.db).Where("analysis_id=?", analysisID).
		Order("sequence_no ASC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("đọc edge case: %w", postgres.Translate(err))
	}
	out := make([]domain.EdgeCase, len(models))
	for i, m := range models {
		out[i] = toEdgeCase(m)
	}
	return out, nil
}

// SaveResolutions cập nhật resolution/ảnh cho từng case rồi tính lại
// resolved_cases trên analysis — tất cả trong 1 transaction (usecase tự bọc
// qua TxManager). KHÔNG tự chuyển status sang "completed": việc đó do usecase
// quyết định sau khi tạo xong phiên bản URD mới (MarkCompleted riêng).
func (r *Repository) SaveResolutions(ctx context.Context, analysisID uuid.UUID, cases []domain.EdgeCase) (*domain.Analysis, error) {
	db := postgres.DBFrom(ctx, r.db)
	for _, c := range cases {
		updates := map[string]any{
			"resolution": c.Resolution, "resolved": true, "updated_at": time.Now().UTC(),
		}
		if c.ImageObjectKey != "" {
			updates["image_object_key"] = c.ImageObjectKey
		}
		res := db.Model(&edgeCaseModel{}).Where("id=? AND analysis_id=?", c.ID, analysisID).Updates(updates)
		if res.Error != nil {
			return nil, fmt.Errorf("lưu hướng giải quyết: %w", postgres.Translate(res.Error))
		}
		if res.RowsAffected == 0 {
			return nil, domain.ErrNotFound
		}
	}
	var resolvedCount int64
	if err := db.Model(&edgeCaseModel{}).Where("analysis_id=? AND resolved=true", analysisID).
		Count(&resolvedCount).Error; err != nil {
		return nil, fmt.Errorf("đếm case đã xử lý: %w", postgres.Translate(err))
	}
	if err := db.Model(&analysisModel{}).Where("id=?", analysisID).
		Update("resolved_cases", resolvedCount).Error; err != nil {
		return nil, fmt.Errorf("cập nhật số case đã xử lý: %w", postgres.Translate(err))
	}
	var am analysisModel
	if err := db.First(&am, "id=?", analysisID).Error; err != nil {
		return nil, mapErr(err)
	}
	a := toAnalysis(am)
	return &a, nil
}

func (r *Repository) MarkCompleted(ctx context.Context, analysisID uuid.UUID) (*domain.Analysis, error) {
	db := postgres.DBFrom(ctx, r.db)
	if err := db.Model(&analysisModel{}).Where("id=?", analysisID).
		Update("status", domain.StatusCompleted).Error; err != nil {
		return nil, fmt.Errorf("cập nhật trạng thái hoàn tất: %w", postgres.Translate(err))
	}
	var am analysisModel
	if err := db.First(&am, "id=?", analysisID).Error; err != nil {
		return nil, mapErr(err)
	}
	a := toAnalysis(am)
	return &a, nil
}

// Summaries trả phân tích MỚI NHẤT (theo created_at) của mỗi document còn
// sống thuộc project — dùng hiển thị cột "Hoàn thiện" trong bảng danh sách
// tài liệu. Lấy toàn bộ rồi chọn bản ghi đầu tiên mỗi document_id trong Go
// (đã ORDER BY created_at DESC trong từng nhóm) thay vì DISTINCT ON — tránh
// phụ thuộc cách gorm.Distinct() ghép chuỗi SELECT, vốn chưa có tiền lệ dùng
// trong repo.
func (r *Repository) Summaries(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID]domain.Analysis, error) {
	var models []analysisModel
	err := postgres.DBFrom(ctx, r.db).
		Table("urd_analyses AS ua").Select("ua.*").
		Joins("JOIN documents d ON d.id = ua.document_id").
		Where("d.project_id = ? AND d.deleted_at IS NULL", projectID).
		Order("ua.document_id, ua.created_at DESC").
		Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("đọc tóm tắt phân tích: %w", postgres.Translate(err))
	}
	out := make(map[uuid.UUID]domain.Analysis, len(models))
	for _, m := range models {
		a := toAnalysis(m)
		if _, exists := out[a.DocumentID]; !exists {
			out[a.DocumentID] = a
		}
	}
	return out, nil
}

func (r *Repository) MemberRole(ctx context.Context, projectID, actorID uuid.UUID) (string, error) {
	var role string
	err := postgres.DBFrom(ctx, r.db).Table("project_members").Select("role").
		Where("project_id=? AND user_id=?", projectID, actorID).Scan(&role).Error
	return role, err
}

func (r *Repository) RAGFlowDatasetID(ctx context.Context, projectID uuid.UUID) (string, error) {
	var datasetID string
	err := postgres.DBFrom(ctx, r.db).Table("projects").Select("COALESCE(ragflow_dataset_id,'')").
		Where("id=? AND deleted_at IS NULL", projectID).Scan(&datasetID).Error
	if err != nil {
		return "", fmt.Errorf("đọc RAGFlow dataset mapping: %w", postgres.Translate(err))
	}
	return datasetID, nil
}

func (r *Repository) RAGFlowChatID(ctx context.Context, projectID uuid.UUID) (string, error) {
	var chatID string
	err := postgres.DBFrom(ctx, r.db).Table("projects").Select("COALESCE(ragflow_chat_id,'')").
		Where("id=? AND deleted_at IS NULL", projectID).Scan(&chatID).Error
	if err != nil {
		return "", fmt.Errorf("đọc RAGFlow chat mapping: %w", postgres.Translate(err))
	}
	return chatID, nil
}

// SaveRAGFlowChatID ghi chatID nếu cột đang rỗng — CHUNG cột projects.ragflow_chat_id
// với module chat/report (xem report/repository/repository.go).
func (r *Repository) SaveRAGFlowChatID(ctx context.Context, projectID uuid.UUID, proposed string) (string, error) {
	var row struct {
		ChatID string `gorm:"column:chat_id"`
	}
	const query = `UPDATE projects
		SET ragflow_chat_id=CASE WHEN COALESCE(ragflow_chat_id,'')='' THEN ? ELSE ragflow_chat_id END,
			updated_at=now()
		WHERE id=? AND deleted_at IS NULL
		RETURNING ragflow_chat_id AS chat_id`
	if err := postgres.DBFrom(ctx, r.db).Raw(query, proposed, projectID).Scan(&row).Error; err != nil {
		return "", fmt.Errorf("lưu RAGFlow chat mapping: %w", postgres.Translate(err))
	}
	return row.ChatID, nil
}

func fromAnalysis(a domain.Analysis) analysisModel {
	return analysisModel{
		ID: a.ID.String(), DocumentID: a.DocumentID.String(), DocumentRevisionID: a.RevisionID.String(),
		Status: a.Status, TotalCases: a.TotalCases, ResolvedCases: a.ResolvedCases,
		CreatedBy: a.CreatedBy.String(), ErrorCode: a.ErrorCode, ErrorDetail: a.ErrorDetail,
	}
}

func toAnalysis(m analysisModel) domain.Analysis {
	return domain.Analysis{
		ID: uuid.MustParse(m.ID), DocumentID: uuid.MustParse(m.DocumentID),
		RevisionID: uuid.MustParse(m.DocumentRevisionID), Status: m.Status,
		TotalCases: m.TotalCases, ResolvedCases: m.ResolvedCases, CreatedBy: uuid.MustParse(m.CreatedBy),
		ErrorCode: m.ErrorCode, ErrorDetail: m.ErrorDetail, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func fromEdgeCase(c domain.EdgeCase) edgeCaseModel {
	m := edgeCaseModel{
		ID: c.ID.String(), AnalysisID: c.AnalysisID.String(), SequenceNo: c.SequenceNo,
		Description: c.Description, Resolution: c.Resolution, Resolved: c.Resolved,
	}
	if c.ImageObjectKey != "" {
		key := c.ImageObjectKey
		m.ImageObjectKey = &key
	}
	return m
}

func toEdgeCase(m edgeCaseModel) domain.EdgeCase {
	c := domain.EdgeCase{
		ID: uuid.MustParse(m.ID), AnalysisID: uuid.MustParse(m.AnalysisID), SequenceNo: m.SequenceNo,
		Description: m.Description, Resolution: m.Resolution, Resolved: m.Resolved,
	}
	if m.ImageObjectKey != nil {
		c.ImageObjectKey = *m.ImageObjectKey
	}
	return c
}

func mapErr(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrNotFound
	}
	return err
}
