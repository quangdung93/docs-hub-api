package ingestion

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/internal/common/port"
	"github.com/quangdung93/docs-hub-api/internal/infrastructure/database/postgres"
)

type Embeddings interface {
	Embed(context.Context, []string) ([][]float32, error)
}
type Config struct {
	ChunkLines, OverlapLines, BatchSize int
	EmbeddingModel                      string
	EmbeddingDimension                  int
}
type Processor struct {
	db       *gorm.DB
	store    port.ObjectStore
	embedder Embeddings
	parsers  *ParserRegistry
	cfg      Config
}

func NewProcessor(db *gorm.DB, store port.ObjectStore, embedder Embeddings, cfg Config) *Processor {
	return &Processor{db: db, store: store, embedder: embedder, parsers: NewParserRegistry(), cfg: cfg}
}

type work struct {
	JobID, RevisionID, DocumentID, ProjectID, ObjectKey, MediaType string
	VersionID, ChangeRequestID                                     *string
}

func (p *Processor) ProcessNext(ctx context.Context) (bool, error) {
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
func (p *Processor) claim(ctx context.Context) (*work, error) { //nolint:lll
	var w work
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		const claimSQL = `WITH next AS (
			SELECT id FROM ingestion_jobs WHERE status='pending' AND available_at<=now()
			ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1
		) UPDATE ingestion_jobs j SET status='running',attempt=attempt+1,updated_at=now()
		FROM next WHERE j.id=next.id
		RETURNING j.id AS job_id,j.document_revision_id AS revision_id`
		return tx.Raw(claimSQL).Scan(&w).Error
	})
	if err != nil {
		return nil, fmt.Errorf("claim ingestion job: %w", err)
	}
	if w.JobID == "" {
		return nil, nil
	}
	const revisionSQL = `SELECT document_id,project_id,object_key,media_type,
		project_version_id AS version_id,change_request_id
		FROM document_revisions WHERE id=?`
	if err := p.db.WithContext(ctx).Raw(revisionSQL, w.RevisionID).Scan(&w).Error; err != nil {
		return nil, fmt.Errorf("đọc revision: %w", err)
	}
	return &w, nil
}
func (p *Processor) process(ctx context.Context, w *work) error { //nolint:lll
	if err := p.db.WithContext(ctx).Table("document_revisions").
		Where("id=?", w.RevisionID).Update("status", "processing").Error; err != nil {
		return fmt.Errorf("đổi trạng thái processing: %w", err)
	}
	reader, err := p.store.GetReader(ctx, w.ObjectKey)
	if err != nil {
		return fmt.Errorf("mở object: %w", err)
	}
	defer reader.Close()
	parsed, err := p.parsers.Parse(ctx, w.MediaType, reader)
	if err != nil {
		return fmt.Errorf("parse object: %w", err)
	}
	chunks := ChunkText(parsed.Text, p.cfg.ChunkLines, p.cfg.OverlapLines)
	if len(chunks) == 0 {
		return fmt.Errorf("tài liệu không có nội dung")
	}
	if err = p.embed(ctx, chunks); err != nil {
		return err
	}
	canonicalKey := w.ObjectKey + ".canonical.txt"
	stored, err := p.store.PutReader(ctx, canonicalKey, strings.NewReader(parsed.Text),
		int64(len([]byte(parsed.Text))), "text/plain; charset=utf-8")
	if err != nil {
		return fmt.Errorf("lưu canonical text: %w", err)
	}
	if err = p.save(ctx, w, chunks, stored.Key, parsed.ParserVersion); err != nil {
		_ = p.store.Delete(ctx, stored.Key)
		return err
	}
	return nil
}

func (p *Processor) embed(ctx context.Context, chunks []Chunk) error {
	batch := p.cfg.BatchSize
	if batch < 1 {
		batch = 16
	}
	for start := 0; start < len(chunks); start += batch {
		end := start + batch
		if end > len(chunks) {
			end = len(chunks)
		}
		texts := make([]string, end-start)
		for i := range texts {
			texts[i] = chunks[start+i].Content
		}
		vectors, err := p.embedder.Embed(ctx, texts)
		if err != nil {
			return fmt.Errorf("tạo embedding: %w", err)
		}
		if err = validateVectors(vectors, len(texts), p.cfg.EmbeddingDimension); err != nil {
			return err
		}
		for i := range vectors {
			chunks[start+i].Embedding = vectors[i]
		}
	}
	return nil
}

func (p *Processor) save(
	ctx context.Context, w *work, chunks []Chunk, canonicalKey, parserVersion string,
) error { //nolint:lll
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM document_chunks WHERE document_revision_id=?", w.RevisionID).Error; err != nil {
			return err
		}
		for _, c := range chunks {
			id := uuid.New()
			vector := vectorLiteral(c.Embedding)
			const insertSQL = `INSERT INTO document_chunks(
				id,project_id,document_id,document_revision_id,project_version_id,change_request_id,
				ordinal,content,embedding,line_start,line_end,token_count,content_hash,
				embedding_model,embedding_dimension)
				VALUES(?,?,?,?,?,?,?,?,?::vector,?,?,?,?,?,?)`
			err := tx.Exec(insertSQL, id, w.ProjectID, w.DocumentID, w.RevisionID,
				w.VersionID, w.ChangeRequestID, c.Ordinal, c.Content, vector,
				c.LineStart, c.LineEnd, c.TokenCount, c.Hash,
				p.cfg.EmbeddingModel, len(c.Embedding)).Error
			if err != nil {
				return fmt.Errorf("insert chunk %d: %w", c.Ordinal, err)
			}
		}
		revisionUpdates := map[string]any{
			"status": "ready", "embedding_model": p.cfg.EmbeddingModel,
			"canonical_text_key": canonicalKey, "parser_version": parserVersion,
			"error_code": nil, "error_detail_sanitized": nil,
			"updated_at": time.Now().UTC(),
		}
		if err := tx.Table("document_revisions").Where("id=?", w.RevisionID).
			Updates(revisionUpdates).Error; err != nil {
			return err
		}
		jobUpdates := map[string]any{"status": "succeeded", "updated_at": time.Now().UTC()}
		return tx.Table("ingestion_jobs").Where("id=?", w.JobID).Updates(jobUpdates).Error
	})
	if err != nil {
		return fmt.Errorf("lưu chunk generation: %w", err)
	}
	return nil
}

func validateVectors(vectors [][]float32, expectedCount, configuredDimension int) error {
	if len(vectors) != expectedCount {
		return fmt.Errorf("LocalAI trả %d embedding, cần %d", len(vectors), expectedCount)
	}
	dimension := configuredDimension
	for i, vector := range vectors {
		if len(vector) == 0 {
			return fmt.Errorf("embedding %d rỗng", i)
		}
		if dimension == 0 {
			dimension = len(vector)
		}
		if len(vector) != dimension {
			return fmt.Errorf("embedding %d có dimension %d, cần %d", i, len(vector), dimension)
		}
		for _, value := range vector {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return fmt.Errorf("embedding %d chứa giá trị không hữu hạn", i)
			}
		}
	}
	return nil
}
func (p *Processor) fail(ctx context.Context, w *work, cause error) { //nolint:lll
	detail := cause.Error()
	if len(detail) > 500 {
		detail = detail[:500]
	}
	_ = postgres.DBFrom(ctx, p.db).Transaction(func(tx *gorm.DB) error {
		revisionUpdates := map[string]any{
			"status": "failed", "error_code": "INGESTION_FAILED",
			"error_detail_sanitized": detail,
		}
		_ = tx.Table("document_revisions").Where("id=?", w.RevisionID).Updates(revisionUpdates).Error
		return tx.Table("ingestion_jobs").Where("id=?", w.JobID).Updates(map[string]any{"status": "failed", "last_error": detail}).Error
	})
}
func vectorLiteral(v []float32) string {
	parts := make([]string, len(v))
	for i, n := range v {
		parts[i] = fmt.Sprintf("%g", n)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
