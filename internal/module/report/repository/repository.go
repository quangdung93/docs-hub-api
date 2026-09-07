// Package repository lưu report bằng PostgreSQL/GORM.
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/internal/infrastructure/database/postgres"
	"github.com/quangdung93/docs-hub-api/internal/module/report/domain"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

type reportModel struct {
	ID          string
	ProjectID   string
	ReportType  string
	Format      string
	FilePath    string
	GeneratedBy string
	CreatedAt   time.Time
}

func (reportModel) TableName() string { return "project_reports" }

type reportItemModel struct {
	ID              string
	ProjectReportID string
	SourceRef       string
	Title           string
	Detail          string
	Status          string
	SequenceNo      int
}

func (reportItemModel) TableName() string { return "project_report_items" }

func (r *Repository) MemberRole(ctx context.Context, projectID, actorID uuid.UUID) (string, error) {
	var role string
	err := postgres.DBFrom(ctx, r.db).Table("project_members").Select("role").
		Where("project_id=? AND user_id=?", projectID, actorID).Scan(&role).Error
	return role, err
}

func (r *Repository) ProjectMeta(ctx context.Context, projectID uuid.UUID) (string, string, error) {
	var m struct{ Name, Code string }
	err := postgres.DBFrom(ctx, r.db).Table("projects").Select("name,code").
		Where("id=? AND deleted_at IS NULL", projectID).Take(&m).Error
	if err != nil {
		return "", "", mapErr(err)
	}
	return m.Name, m.Code, nil
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

// SaveRAGFlowChatID ghi chatID nếu cột đang rỗng (idempotent — CHUNG cột với
// module chat, đứa nào tạo chat trước thì đứa đó thắng, không lỗi).
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

// Create lưu report + toàn bộ item trong 1 transaction — usecase gọi qua
// TxManager.Do nên db lấy được ở đây (postgres.DBFrom) đã nằm trong tx đó.
func (r *Repository) Create(ctx context.Context, report domain.Report, items []domain.ReportItem) error {
	db := postgres.DBFrom(ctx, r.db)
	rm := reportModel{
		ID: report.ID.String(), ProjectID: report.ProjectID.String(),
		ReportType: report.ReportType, Format: report.Format, FilePath: report.FileKey,
		GeneratedBy: report.GeneratedBy.String(), CreatedAt: report.CreatedAt,
	}
	if err := db.Create(&rm).Error; err != nil {
		return fmt.Errorf("tạo report: %w", postgres.Translate(err))
	}
	if len(items) == 0 {
		return nil
	}
	models := make([]reportItemModel, len(items))
	for i, item := range items {
		models[i] = reportItemModel{
			ID: item.ID.String(), ProjectReportID: item.ReportID.String(),
			SourceRef: item.SourceRef, Title: item.Title, Detail: item.Detail,
			Status: item.Status, SequenceNo: item.SequenceNo,
		}
	}
	if err := db.Create(&models).Error; err != nil {
		return fmt.Errorf("tạo report item: %w", postgres.Translate(err))
	}
	return nil
}

func (r *Repository) ListHistory(
	ctx context.Context, projectID uuid.UUID, page, limit int,
) ([]domain.Report, int64, error) {
	db := postgres.DBFrom(ctx, r.db)
	var total int64
	if err := db.Model(&reportModel{}).Where("project_id=?", projectID).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("đếm report: %w", postgres.Translate(err))
	}
	var rows []reportModel
	err := db.Where("project_id=?", projectID).
		Order("created_at DESC, id DESC").
		Offset((page - 1) * limit).Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("đọc lịch sử report: %w", postgres.Translate(err))
	}
	out := make([]domain.Report, len(rows))
	for i, row := range rows {
		out[i] = toReport(row)
	}
	return out, total, nil
}

func toReport(m reportModel) domain.Report {
	return domain.Report{
		ID: uuid.MustParse(m.ID), ProjectID: uuid.MustParse(m.ProjectID),
		ReportType: m.ReportType, Format: m.Format, FileKey: m.FilePath,
		GeneratedBy: uuid.MustParse(m.GeneratedBy), CreatedAt: m.CreatedAt,
	}
}

func mapErr(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrNotFound
	}
	return err
}
