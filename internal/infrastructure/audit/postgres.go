// Package audit hiện thực cổng ghi audit bằng PostgreSQL.
package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/internal/common/port"
	"github.com/quangdung93/docs-hub-api/internal/infrastructure/database/postgres"
)

// PostgreSQL ghi audit event vào bảng audit_logs hiện có.
type PostgreSQL struct{ db *gorm.DB }

// NewPostgreSQL tạo audit writer dùng connection pool chung.
func NewPostgreSQL(db *gorm.DB) *PostgreSQL { return &PostgreSQL{db: db} }

// Record ghi một event; lỗi serialize hoặc database được bọc để caller chỉ log.
func (a *PostgreSQL) Record(ctx context.Context, event port.AuditEvent) error {
	metadata, err := json.Marshal(event.Metadata)
	if err != nil {
		return fmt.Errorf("marshal audit metadata: %w", err)
	}
	record := map[string]any{
		"id": uuid.New(), "actor_user_id": event.ActorUserID, "project_id": event.ProjectID,
		"action": event.Action, "entity_type": event.EntityType, "entity_id": event.EntityID,
		"request_id": event.RequestID, "metadata": string(metadata),
	}
	if err = postgres.DBFrom(ctx, a.db).Table("audit_logs").Create(record).Error; err != nil {
		return fmt.Errorf("ghi audit event %s: %w", event.Action, err)
	}
	return nil
}
