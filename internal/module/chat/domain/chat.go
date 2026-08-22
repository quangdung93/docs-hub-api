// Package domain định nghĩa entity và repository cho hội thoại tra cứu.
package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	retrievaldomain "github.com/quangdung93/docs-hub-api/internal/module/retrieval/domain"
)

var ErrNotFound = errors.New("không tìm thấy conversation")

type Conversation struct {
	ID          uuid.UUID              `json:"id"`
	ProjectID   uuid.UUID              `json:"project_id"`
	UserID      uuid.UUID              `json:"user_id"`
	Title       string                 `json:"title"`
	ActiveScope *retrievaldomain.Scope `json:"active_scope,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	Messages    []Message              `json:"messages,omitempty"`
}

type Message struct {
	ID            uuid.UUID                       `json:"id"`
	Role          string                          `json:"role"`
	Content       string                          `json:"content"`
	Intent        string                          `json:"intent,omitempty"`
	ResolvedScope []retrievaldomain.ResolvedScope `json:"resolved_scope,omitempty"`
	Model         string                          `json:"model,omitempty"`
	PromptVersion string                          `json:"prompt_version,omitempty"`
	LatencyMS     int64                           `json:"latency_ms,omitempty"`
	CreatedAt     time.Time                       `json:"created_at"`
	Citations     []Citation                      `json:"citations,omitempty"`
}

type Citation struct {
	Key                string    `json:"key"`
	ChunkID            string    `json:"chunk_id"`
	DocumentID         uuid.UUID `json:"document_id"`
	DocumentRevisionID uuid.UUID `json:"document_revision_id"`
	DocumentTitle      string    `json:"-"`
	DocumentName       string    `json:"document_name"`
	ScopeType          string    `json:"scope_type"`
	ScopeLabel         string    `json:"scope_label"`
	LineStart          *int      `json:"line_start"`
	LineEnd            *int      `json:"line_end"`
	PageStart          *int      `json:"page_start"`
	PageEnd            *int      `json:"page_end"`
	Excerpt            string    `json:"excerpt"`
	SourceURL          string    `json:"source_url"`
}

type Exchange struct {
	Question, Answer, Intent, Model, PromptVersion string
	Scope                                          retrievaldomain.Scope
	ResolvedScope                                  []retrievaldomain.ResolvedScope
	LatencyMS                                      int64
	Citations                                      []Citation
}

type Repository interface {
	MemberRole(ctx context.Context, projectID, actorID uuid.UUID) (string, error)
	Create(ctx context.Context, conversation *Conversation) error
	List(ctx context.Context, projectID, actorID uuid.UUID, page pagination.Query) ([]Conversation, int64, error)
	Get(ctx context.Context, projectID, actorID, conversationID uuid.UUID) (*Conversation, error)
	SaveExchange(ctx context.Context, conversationID uuid.UUID, exchange Exchange) (*Message, error)
	RAGFlowChatID(ctx context.Context, projectID uuid.UUID) (string, error)
	SaveRAGFlowChatID(ctx context.Context, projectID uuid.UUID, proposed string) (string, error)
}
