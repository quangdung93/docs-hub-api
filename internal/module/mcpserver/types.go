package mcpserver

import (
	"time"

	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
)

type scopeInput struct {
	Mode             string   `json:"mode,omitempty" jsonschema:"Scope mode: all, versions, or change_requests"`
	VersionIDs       []string `json:"version_ids,omitempty" jsonschema:"Project version UUIDs"`
	ChangeRequestIDs []string `json:"change_request_ids,omitempty" jsonschema:"Change request UUIDs"`
}

type projectSummary struct {
	ID          string    `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Version     int       `json:"version"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type versionSummary struct {
	ID         string     `json:"id"`
	ProjectID  string     `json:"project_id"`
	Label      string     `json:"label"`
	SequenceNo int64      `json:"sequence_no"`
	Status     string     `json:"status"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
}

type citationOutput struct {
	Key                string   `json:"key"`
	ChunkID            string   `json:"chunk_id"`
	DocumentID         string   `json:"document_id"`
	DocumentRevisionID string   `json:"document_revision_id"`
	DocumentName       string   `json:"document_name"`
	ScopeType          string   `json:"scope_type"`
	ScopeLabel         string   `json:"scope_label"`
	LineStart          *int     `json:"line_start,omitempty"`
	LineEnd            *int     `json:"line_end,omitempty"`
	PageStart          *int     `json:"page_start,omitempty"`
	PageEnd            *int     `json:"page_end,omitempty"`
	Excerpt            string   `json:"excerpt"`
	SourceURL          string   `json:"source_url"`
	Score              *float64 `json:"score,omitempty"`
}

type resolvedScopeOutput struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Label string `json:"label"`
}

type listProjectsOutput struct {
	Projects []projectSummary `json:"projects"`
	Page     pagination.Meta  `json:"page"`
}

type listVersionsOutput struct {
	Versions []versionSummary `json:"versions"`
	Page     pagination.Meta  `json:"page"`
}

type searchOutput struct {
	Query         string                `json:"query"`
	ResolvedScope []resolvedScopeOutput `json:"resolved_scope"`
	Citations     []citationOutput      `json:"citations"`
	Total         int                   `json:"total"`
}

type answerOutput struct {
	ConversationID string                `json:"conversation_id"`
	Answer         string                `json:"answer"`
	Intent         string                `json:"intent"`
	ResolvedScope  []resolvedScopeOutput `json:"resolved_scope"`
	Citations      []citationOutput      `json:"citations"`
	Grounded       bool                  `json:"grounded"`
}

type sourceOutput struct {
	ProjectID  string `json:"project_id"`
	DocumentID string `json:"document_id"`
	RevisionID string `json:"revision_id"`
	FileName   string `json:"file_name"`
	LineStart  int    `json:"line_start"`
	LineEnd    int    `json:"line_end"`
	Text       string `json:"text"`
	Truncated  bool   `json:"truncated"`
}
