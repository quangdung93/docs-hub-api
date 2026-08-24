// Package repository lưu hội thoại và citation trong PostgreSQL.
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	"github.com/quangdung93/docs-hub-api/internal/infrastructure/database/postgres"
	"github.com/quangdung93/docs-hub-api/internal/module/chat/domain"
	retrievaldomain "github.com/quangdung93/docs-hub-api/internal/module/retrieval/domain"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) MemberRole(ctx context.Context, projectID, actorID uuid.UUID) (string, error) {
	var role string
	err := postgres.DBFrom(ctx, r.db).Table("project_members").Select("role").
		Where("project_id=? AND user_id=? AND status='active'", projectID, actorID).Scan(&role).Error
	return role, err
}

func (r *Repository) Create(ctx context.Context, conversation *domain.Conversation) error {
	scope, err := marshalJSON(conversation.ActiveScope)
	if err != nil {
		return err
	}
	const query = `INSERT INTO conversations(id,project_id,user_id,title,active_scope,created_at,updated_at)
		VALUES(?,?,?,?,?::jsonb,?,?)`
	if err = postgres.DBFrom(ctx, r.db).Exec(query, conversation.ID, conversation.ProjectID,
		conversation.UserID, conversation.Title, scope, conversation.CreatedAt, conversation.UpdatedAt).Error; err != nil {
		return fmt.Errorf("tạo conversation: %w", postgres.Translate(err))
	}
	return nil
}

type conversationRow struct {
	ID, ProjectID, UserID string
	Title                 string
	ActiveScope           []byte `gorm:"column:active_scope"`
	CreatedAt, UpdatedAt  time.Time
}

func (r *Repository) List(
	ctx context.Context, projectID, actorID uuid.UUID, page pagination.Query,
) ([]domain.Conversation, int64, error) {
	db := postgres.DBFrom(ctx, r.db)
	base := db.Table("conversations").Where(
		"project_id=? AND user_id=? AND deleted_at IS NULL", projectID, actorID)
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("đếm conversation: %w", postgres.Translate(err))
	}
	var rows []conversationRow
	if err := base.Select("id,project_id,user_id,title,active_scope,created_at,updated_at").
		Order("updated_at DESC,id DESC").Limit(page.Limit).Offset(page.Offset()).Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("liệt kê conversation: %w", postgres.Translate(err))
	}
	items, err := mapConversations(rows)
	return items, total, err
}

func (r *Repository) Get(
	ctx context.Context, projectID, actorID, conversationID uuid.UUID,
) (*domain.Conversation, error) {
	db := postgres.DBFrom(ctx, r.db)
	var row conversationRow
	result := db.Table("conversations").Select(
		"id,project_id,user_id,title,active_scope,created_at,updated_at").
		Where("id=? AND project_id=? AND user_id=? AND deleted_at IS NULL",
			conversationID, projectID, actorID).Take(&row)
	if result.Error != nil {
		translated := postgres.Translate(result.Error)
		if errors.Is(translated, postgres.ErrNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("đọc conversation: %w", translated)
	}
	items, err := mapConversations([]conversationRow{row})
	if err != nil {
		return nil, err
	}
	if err = r.loadMessages(ctx, db, &items[0]); err != nil {
		return nil, err
	}
	return &items[0], nil
}

type messageRow struct {
	ID, Role, Content, Intent, Model, PromptVersion string
	ResolvedScope                                   []byte `gorm:"column:resolved_scope"`
	LatencyMS                                       int64
	CreatedAt                                       time.Time
}

func (r *Repository) loadMessages(ctx context.Context, db *gorm.DB, conversation *domain.Conversation) error {
	var rows []messageRow
	if err := db.WithContext(ctx).Table("messages").Select(
		"id,role,content,intent,resolved_scope,model,prompt_version,latency_ms,created_at").
		Where("conversation_id=?", conversation.ID).Order("created_at,id").Scan(&rows).Error; err != nil {
		return fmt.Errorf("đọc messages: %w", postgres.Translate(err))
	}
	conversation.Messages = make([]domain.Message, len(rows))
	for i, row := range rows {
		messageID, err := uuid.Parse(row.ID)
		if err != nil {
			return fmt.Errorf("parse message id: %w", err)
		}
		message := domain.Message{
			ID: messageID, Role: row.Role, Content: row.Content, Intent: row.Intent,
			Model: row.Model, PromptVersion: row.PromptVersion, LatencyMS: row.LatencyMS, CreatedAt: row.CreatedAt,
		}
		if len(row.ResolvedScope) > 0 {
			if err = json.Unmarshal(row.ResolvedScope, &message.ResolvedScope); err != nil {
				return fmt.Errorf("decode resolved scope: %w", err)
			}
		}
		if err = r.loadCitations(ctx, db, &message); err != nil {
			return err
		}
		conversation.Messages[i] = message
	}
	return nil
}

func (r *Repository) loadCitations(ctx context.Context, db *gorm.DB, message *domain.Message) error {
	var rows []struct {
		ChunkID, DocumentID, RevisionID, DocumentName, ScopeType, ScopeLabel, Excerpt, SourceURL string
		CitationOrder                                                                            int
		LineStart, LineEnd, PageStart, PageEnd                                                   *int
	}
	if err := db.WithContext(ctx).Table("message_citations").Select(`chunk_id,document_id,
		document_revision_id AS revision_id,document_name_snapshot AS document_name,scope_type,
		scope_label_snapshot AS scope_label,quoted_text AS excerpt,source_url,citation_order,
		line_start,line_end,page_start,page_end`).Where("message_id=?", message.ID).
		Order("citation_order").Scan(&rows).Error; err != nil {
		return fmt.Errorf("đọc citations: %w", postgres.Translate(err))
	}
	message.Citations = make([]domain.Citation, len(rows))
	for i, row := range rows {
		documentID, err := uuid.Parse(row.DocumentID)
		if err != nil {
			return fmt.Errorf("parse citation document id: %w", err)
		}
		revisionID, err := uuid.Parse(row.RevisionID)
		if err != nil {
			return fmt.Errorf("parse citation revision id: %w", err)
		}
		message.Citations[i] = domain.Citation{
			Key: fmt.Sprintf("S%d", row.CitationOrder), ChunkID: row.ChunkID,
			DocumentID: documentID, DocumentRevisionID: revisionID, DocumentName: row.DocumentName,
			ScopeType: row.ScopeType, ScopeLabel: row.ScopeLabel, Excerpt: row.Excerpt,
			SourceURL: row.SourceURL, LineStart: row.LineStart, LineEnd: row.LineEnd,
			PageStart: row.PageStart, PageEnd: row.PageEnd,
		}
	}
	return nil
}

func (r *Repository) SaveExchange(
	ctx context.Context, conversationID uuid.UUID, exchange domain.Exchange,
) (*domain.Message, error) {
	now := time.Now().UTC()
	userID, assistantID := uuid.New(), uuid.New()
	scopeJSON, err := marshalJSON(exchange.Scope)
	if err != nil {
		return nil, err
	}
	resolvedJSON, err := marshalJSON(exchange.ResolvedScope)
	if err != nil {
		return nil, err
	}
	err = postgres.DBFrom(ctx, r.db).Transaction(func(tx *gorm.DB) error {
		const insert = `INSERT INTO messages(id,conversation_id,role,content,intent,resolved_scope,
			model,prompt_version,latency_ms,created_at) VALUES(?,?,?,?,?,?::jsonb,?,?,?,?)`
		if saveErr := tx.Exec(insert, userID, conversationID, "user", exchange.Question,
			"", nil, "", "", 0, now).Error; saveErr != nil {
			return saveErr
		}
		if saveErr := tx.Exec(insert, assistantID, conversationID, "assistant", exchange.Answer,
			exchange.Intent, resolvedJSON, exchange.Model, exchange.PromptVersion,
			exchange.LatencyMS, now.Add(time.Nanosecond)).Error; saveErr != nil {
			return saveErr
		}
		for i, citation := range exchange.Citations {
			if saveErr := insertCitation(tx, assistantID, i+1, citation); saveErr != nil {
				return saveErr
			}
		}
		return tx.Exec(`UPDATE conversations SET active_scope=?::jsonb,updated_at=? WHERE id=?`,
			scopeJSON, now, conversationID).Error
	})
	if err != nil {
		return nil, fmt.Errorf("lưu exchange: %w", postgres.Translate(err))
	}
	return &domain.Message{
		ID: assistantID, Role: "assistant", Content: exchange.Answer, Intent: exchange.Intent,
		ResolvedScope: exchange.ResolvedScope, Model: exchange.Model,
		PromptVersion: exchange.PromptVersion, LatencyMS: exchange.LatencyMS,
		CreatedAt: now.Add(time.Nanosecond), Citations: exchange.Citations,
	}, nil
}

func (r *Repository) RAGFlowChatID(ctx context.Context, projectID uuid.UUID) (string, error) {
	var chatID string
	if err := postgres.DBFrom(ctx, r.db).Table("projects").Select("COALESCE(ragflow_chat_id,'')").
		Where("id=? AND deleted_at IS NULL", projectID).Scan(&chatID).Error; err != nil {
		return "", fmt.Errorf("đọc RAGFlow chat mapping: %w", postgres.Translate(err))
	}
	return chatID, nil
}

func (r *Repository) SaveRAGFlowChatID(
	ctx context.Context, projectID uuid.UUID, proposed string,
) (string, error) {
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
	if row.ChatID == "" {
		return "", domain.ErrNotFound
	}
	return row.ChatID, nil
}

func insertCitation(tx *gorm.DB, messageID uuid.UUID, order int, citation domain.Citation) error {
	const query = `INSERT INTO message_citations(id,message_id,chunk_id,citation_order,quoted_text,
		document_id,document_revision_id,document_title_snapshot,document_name_snapshot,scope_type,
		scope_label_snapshot,line_start,line_end,page_start,page_end,source_url)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	return tx.Exec(query, uuid.New(), messageID, citation.ChunkID, order, truncate(citation.Excerpt, 1000),
		citation.DocumentID, citation.DocumentRevisionID, citation.DocumentTitle, citation.DocumentName,
		citation.ScopeType, citation.ScopeLabel, citation.LineStart, citation.LineEnd,
		citation.PageStart, citation.PageEnd, citation.SourceURL).Error
}

func mapConversations(rows []conversationRow) ([]domain.Conversation, error) {
	items := make([]domain.Conversation, len(rows))
	for i, row := range rows {
		id, err := uuid.Parse(row.ID)
		if err != nil {
			return nil, fmt.Errorf("parse conversation id: %w", err)
		}
		projectID, err := uuid.Parse(row.ProjectID)
		if err != nil {
			return nil, fmt.Errorf("parse conversation project id: %w", err)
		}
		userID, err := uuid.Parse(row.UserID)
		if err != nil {
			return nil, fmt.Errorf("parse conversation user id: %w", err)
		}
		items[i] = domain.Conversation{
			ID: id, ProjectID: projectID, UserID: userID, Title: row.Title,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		}
		if len(row.ActiveScope) > 0 {
			var scope retrievaldomain.Scope
			if err = json.Unmarshal(row.ActiveScope, &scope); err != nil {
				return nil, fmt.Errorf("decode active scope: %w", err)
			}
			items[i].ActiveScope = &scope
		}
	}
	return items, nil
}

func marshalJSON(value any) (string, error) {
	if value == nil {
		return "null", nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode JSON: %w", err)
	}
	return string(raw), nil
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
