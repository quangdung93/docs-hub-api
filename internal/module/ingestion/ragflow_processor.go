package ingestion

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/internal/common/port"
)

type RAGFlowProcessorConfig struct {
	PollInterval    time.Duration
	MaxPollDuration time.Duration
	DatasetPrefix   string
}

// RAGFlowProcessor đồng bộ file gốc từ ObjectStore sang RAGFlow. PostgreSQL
// vẫn giữ project/revision/version và remote ID làm source of truth nghiệp vụ.
type RAGFlowProcessor struct {
	db    *gorm.DB
	store port.ObjectStore
	rag   port.RAGClient
	cfg   RAGFlowProcessorConfig
}

func NewRAGFlowProcessor(db *gorm.DB, store port.ObjectStore, rag port.RAGClient, cfg RAGFlowProcessorConfig) *RAGFlowProcessor {
	return &RAGFlowProcessor{db: db, store: store, rag: rag, cfg: cfg}
}

type ragWork struct {
	JobID            string  `gorm:"column:job_id"`
	RevisionID       string  `gorm:"column:revision_id"`
	DocumentID       string  `gorm:"column:document_id"`
	ProjectID        string  `gorm:"column:project_id"`
	ProjectName      string  `gorm:"column:project_name"`
	DatasetID        *string `gorm:"column:ragflow_dataset_id"`
	RemoteDocumentID *string `gorm:"column:ragflow_document_id"`
	ProjectVersionID *string `gorm:"column:project_version_id"`
	ChangeRequestID  *string `gorm:"column:change_request_id"`
	ObjectKey        string  `gorm:"column:object_key"`
	FileName         string  `gorm:"column:file_name"`
	MediaType        string  `gorm:"column:media_type"`
}

func (p *RAGFlowProcessor) ProcessNext(ctx context.Context) (bool, error) {
	cleaned, err := p.processCleanup(ctx)
	if err != nil || cleaned {
		return cleaned, err
	}
	w, err := p.claim(ctx)
	if err != nil {
		return false, err
	}
	if w == nil {
		return false, nil
	}
	if err = p.process(ctx, w); err != nil {
		p.fail(ctx, w, err)
		return true, err
	}
	return true, nil
}

type cleanupWork struct {
	EventID    string `gorm:"column:event_id"`
	DocumentID string `gorm:"column:document_id"`
	ProjectID  string `gorm:"column:project_id"`
	DatasetID  string `gorm:"column:dataset_id"`
	Attempt    int    `gorm:"column:attempt"`
}

func (p *RAGFlowProcessor) processCleanup(ctx context.Context) (bool, error) {
	var claimed struct {
		EventID     string `gorm:"column:event_id"`
		AggregateID string `gorm:"column:aggregate_id"`
		Attempt     int    `gorm:"column:attempt"`
	}
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		const sql = `WITH next AS (
			SELECT id FROM outbox_events
			WHERE topic='document.cleanup' AND status='pending' AND available_at<=now()
			ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1
		) UPDATE outbox_events e
		SET status='processing',attempt=attempt+1
		FROM next WHERE e.id=next.id
		RETURNING e.id AS event_id,e.aggregate_id,e.attempt`
		return tx.Raw(sql).Scan(&claimed).Error
	})
	if err != nil {
		return false, fmt.Errorf("claim RAGFlow cleanup event: %w", err)
	}
	if claimed.EventID == "" {
		return false, nil
	}
	w := cleanupWork{EventID: claimed.EventID, DocumentID: claimed.AggregateID, Attempt: claimed.Attempt}
	const mappingSQL = `SELECT d.project_id,p.ragflow_dataset_id AS dataset_id
		FROM documents d JOIN projects p ON p.id=d.project_id WHERE d.id=?`
	if err = p.db.WithContext(ctx).Raw(mappingSQL, w.DocumentID).Scan(&w).Error; err != nil {
		p.failCleanup(ctx, w, err)
		return true, err
	}
	var documentIDs []string
	if err = p.db.WithContext(ctx).Table("document_revisions").Where("document_id=? AND ragflow_document_id IS NOT NULL", w.DocumentID).
		Pluck("ragflow_document_id", &documentIDs).Error; err != nil {
		p.failCleanup(ctx, w, err)
		return true, err
	}
	if w.DatasetID != "" && len(documentIDs) > 0 {
		if err = p.rag.DeleteDocuments(ctx, w.DatasetID, documentIDs); err != nil {
			wrapped := fmt.Errorf("xóa RAGFlow documents: %w", err)
			p.failCleanup(ctx, w, wrapped)
			return true, wrapped
		}
	}
	err = p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("document_revisions").Where("document_id=?", w.DocumentID).Updates(map[string]any{
			"ragflow_sync_status": "deleted", "ragflow_document_id": nil,
			"ragflow_last_error": nil, "updated_at": time.Now().UTC(),
		}).Error; err != nil {
			return err
		}
		return tx.Table("outbox_events").Where("id=?", w.EventID).
			Updates(map[string]any{"status": "succeeded"}).Error
	})
	if err != nil {
		p.failCleanup(ctx, w, err)
		return true, fmt.Errorf("hoàn tất RAGFlow cleanup: %w", err)
	}
	return true, nil
}

func (p *RAGFlowProcessor) failCleanup(ctx context.Context, w cleanupWork, cause error) {
	status := "pending"
	if w.Attempt >= 5 {
		status = "failed"
	}
	backoff := time.Duration(1<<min(w.Attempt, 6)) * time.Second
	_ = p.db.WithContext(ctx).Table("outbox_events").Where("id=?", w.EventID).Updates(map[string]any{
		"status": status, "available_at": time.Now().UTC().Add(backoff),
	}).Error
}

func (p *RAGFlowProcessor) claim(ctx context.Context) (*ragWork, error) {
	var claimed struct {
		JobID              string `gorm:"column:job_id"`
		DocumentRevisionID string `gorm:"column:document_revision_id"`
	}
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		const sql = `WITH next AS (
			SELECT id FROM ingestion_jobs
			WHERE status='pending' AND available_at<=now() AND attempt<max_attempts
			ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1
		) UPDATE ingestion_jobs j
		SET status='running',attempt=attempt+1,updated_at=now()
		FROM next WHERE j.id=next.id
		RETURNING j.id AS job_id,j.document_revision_id`
		return tx.Raw(sql).Scan(&claimed).Error
	})
	if err != nil {
		return nil, fmt.Errorf("claim RAGFlow ingestion job: %w", err)
	}
	if claimed.JobID == "" {
		return nil, nil
	}
	var w ragWork
	const sql = `SELECT r.id AS revision_id,r.document_id,r.project_id,p.name AS project_name,
		p.ragflow_dataset_id,r.ragflow_document_id,r.project_version_id,r.change_request_id,
		r.object_key,r.file_name,r.media_type
		FROM document_revisions r JOIN projects p ON p.id=r.project_id
		WHERE r.id=? AND p.deleted_at IS NULL`
	if err = p.db.WithContext(ctx).Raw(sql, claimed.DocumentRevisionID).Scan(&w).Error; err != nil {
		return nil, fmt.Errorf("đọc RAGFlow revision: %w", err)
	}
	if w.RevisionID == "" {
		return nil, fmt.Errorf("revision %s không tồn tại", claimed.DocumentRevisionID)
	}
	w.JobID = claimed.JobID
	return &w, nil
}

func (p *RAGFlowProcessor) process(ctx context.Context, w *ragWork) error {
	datasetID, err := p.ensureDataset(ctx, w)
	if err != nil {
		return err
	}
	remoteID := value(w.RemoteDocumentID)
	remoteName := w.RevisionID + "__" + sanitizeRemoteName(w.FileName)
	if remoteID == "" {
		// Reconcile trường hợp remote upload thành công nhưng local update bị timeout.
		existing, findErr := p.rag.FindDocumentByName(ctx, datasetID, remoteName)
		if findErr != nil {
			return fmt.Errorf("reconcile RAGFlow document: %w", findErr)
		}
		if existing != nil {
			remoteID = existing.ID
		} else {
			if err = p.updateRevision(ctx, w.RevisionID, map[string]any{
				"status": "processing", "ragflow_sync_status": "uploading", "ragflow_last_error": nil,
			}); err != nil {
				return err
			}
			reader, openErr := p.store.GetReader(ctx, w.ObjectKey)
			if openErr != nil {
				return fmt.Errorf("mở object để upload RAGFlow: %w", openErr)
			}
			remote, uploadErr := p.rag.UploadDocument(ctx, datasetID, port.RAGDocumentFile{
				Name: remoteName, ContentType: w.MediaType, Reader: reader,
			})
			closeErr := reader.Close()
			if uploadErr != nil {
				return fmt.Errorf("upload RAGFlow document: %w", uploadErr)
			}
			if closeErr != nil {
				return fmt.Errorf("đóng object sau upload: %w", closeErr)
			}
			remoteID = remote.ID
		}
		if err = p.updateRevision(ctx, w.RevisionID, map[string]any{
			"ragflow_document_id": remoteID, "ragflow_sync_status": "parsing", "updated_at": time.Now().UTC(),
		}); err != nil {
			return err
		}
	}
	metadata := map[string]string{"docs_hub_revision_id": w.RevisionID}
	if scopeID := value(w.ProjectVersionID); scopeID != "" {
		metadata["docs_hub_scope_id"] = scopeID
		metadata["docs_hub_scope_type"] = "version"
	} else if scopeID := value(w.ChangeRequestID); scopeID != "" {
		metadata["docs_hub_scope_id"] = scopeID
		metadata["docs_hub_scope_type"] = "change_request"
	}
	if err = p.rag.UpdateDocumentMetadata(ctx, datasetID, []string{remoteID}, metadata); err != nil {
		return fmt.Errorf("cập nhật RAGFlow document metadata: %w", err)
	}
	remote, err := p.rag.GetDocument(ctx, datasetID, remoteID)
	if err != nil {
		return fmt.Errorf("đọc RAGFlow document trước khi parse: %w", err)
	}
	if !remote.Ready() {
		if err = p.rag.StartParsing(ctx, datasetID, []string{remoteID}); err != nil {
			return fmt.Errorf("bắt đầu RAGFlow parsing: %w", err)
		}
		if err = p.waitReady(ctx, datasetID, remoteID); err != nil {
			return err
		}
	}
	return p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		if err := tx.Table("document_revisions").Where("id=?", w.RevisionID).Updates(map[string]any{
			"status": "ready", "ragflow_sync_status": "ready", "ragflow_synced_at": now,
			"ragflow_last_error": nil, "error_code": nil, "error_detail_sanitized": nil, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		return tx.Table("ingestion_jobs").Where("id=?", w.JobID).
			Updates(map[string]any{"status": "succeeded", "last_error": nil, "updated_at": now}).Error
	})
}

func (p *RAGFlowProcessor) ensureDataset(ctx context.Context, w *ragWork) (string, error) {
	if id := value(w.DatasetID); id != "" {
		return id, nil
	}
	prefix := strings.Trim(p.cfg.DatasetPrefix, "_-")
	if prefix == "" {
		prefix = "docs_hub"
	}
	name := prefix + "_" + strings.ReplaceAll(w.ProjectID, "-", "")
	dataset, err := p.rag.FindDatasetByName(ctx, name)
	if err != nil {
		return "", fmt.Errorf("tìm RAGFlow dataset: %w", err)
	}
	if dataset == nil {
		created, createErr := p.rag.CreateDataset(ctx, name, w.ProjectName)
		if createErr != nil {
			return "", fmt.Errorf("tạo RAGFlow dataset: %w", createErr)
		}
		dataset = &created
	}
	res := p.db.WithContext(ctx).Table("projects").Where("id=? AND ragflow_dataset_id IS NULL", w.ProjectID).
		Updates(map[string]any{"ragflow_dataset_id": dataset.ID, "ragflow_sync_status": "ready", "ragflow_last_error": nil})
	if res.Error != nil {
		return "", fmt.Errorf("lưu RAGFlow dataset mapping: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		var current string
		if err = p.db.WithContext(ctx).Table("projects").Select("ragflow_dataset_id").Where("id=?", w.ProjectID).Scan(&current).Error; err != nil {
			return "", fmt.Errorf("đọc RAGFlow dataset mapping: %w", err)
		}
		if current != dataset.ID {
			return "", fmt.Errorf("project đã map tới RAGFlow dataset khác")
		}
	}
	return dataset.ID, nil
}

func (p *RAGFlowProcessor) waitReady(ctx context.Context, datasetID, documentID string) error {
	ctx, cancel := context.WithTimeout(ctx, p.cfg.MaxPollDuration)
	defer cancel()
	ticker := time.NewTicker(p.cfg.PollInterval)
	defer ticker.Stop()
	for {
		document, err := p.rag.GetDocument(ctx, datasetID, documentID)
		if err != nil {
			return fmt.Errorf("đọc trạng thái RAGFlow document: %w", err)
		}
		if document.Failed() {
			return fmt.Errorf("RAGFlow parse thất bại: %s", sanitizeDetail(document.ProgressMsg))
		}
		if document.Ready() {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("chờ RAGFlow parse: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func (p *RAGFlowProcessor) updateRevision(ctx context.Context, revisionID string, updates map[string]any) error {
	if err := p.db.WithContext(ctx).Table("document_revisions").Where("id=?", revisionID).Updates(updates).Error; err != nil {
		return fmt.Errorf("cập nhật RAGFlow revision: %w", err)
	}
	return nil
}

func (p *RAGFlowProcessor) fail(ctx context.Context, w *ragWork, cause error) {
	detail := sanitizeDetail(cause.Error())
	_ = p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_ = tx.Table("document_revisions").Where("id=?", w.RevisionID).Updates(map[string]any{
			"status": "failed", "ragflow_sync_status": "failed", "ragflow_last_error": detail,
			"error_code": "RAGFLOW_INGESTION_FAILED", "error_detail_sanitized": detail,
		}).Error
		return tx.Table("ingestion_jobs").Where("id=?", w.JobID).
			Updates(map[string]any{"status": "failed", "last_error": detail, "updated_at": time.Now().UTC()}).Error
	})
}

func value(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func sanitizeRemoteName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "\x00", "")
	name = strings.NewReplacer("\r", "_", "\n", "_").Replace(name)
	if name == "" || name == "." {
		return "document"
	}
	const maxNameBytes = 180
	if len(name) > maxNameBytes {
		ext := filepath.Ext(name)
		baseLimit := maxNameBytes - len(ext)
		if baseLimit < 1 {
			baseLimit = maxNameBytes
			ext = ""
		}
		name = name[:baseLimit] + ext
	}
	return name
}

func sanitizeDetail(detail string) string {
	detail = strings.TrimSpace(detail)
	if len(detail) > 500 {
		return detail[:500]
	}
	return detail
}
