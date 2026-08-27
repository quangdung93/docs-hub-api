package mcpserver

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/common/port"
	chatdomain "github.com/quangdung93/docs-hub-api/internal/module/chat/domain"
	retrievaldomain "github.com/quangdung93/docs-hub-api/internal/module/retrieval/domain"
	retrievaluc "github.com/quangdung93/docs-hub-api/internal/module/retrieval/usecase"
	"github.com/quangdung93/docs-hub-api/pkg/logger"
)

func (m *Module) allow(ctx context.Context, operation string) error {
	actor, ok := contextx.ActorFrom(ctx)
	if !ok || actor.UserID == "" {
		return errors.New("AUTH_401: Chưa xác thực")
	}
	if m.cache == nil {
		return nil
	}
	key := fmt.Sprintf("ratelimit:mcp:user:%s:operation:%s", actor.UserID, operation)
	count, err := m.cache.Incr(ctx, key, m.window)
	if err != nil {
		logger.FromContext(ctx).Error("MCP rate limiter fail-open", zap.String("operation", operation), zap.Error(err))
		return nil
	}
	if count > int64(m.requestsPerWindow) {
		return errors.New("RATE_429: Quá nhiều yêu cầu MCP, vui lòng thử lại sau")
	}
	return nil
}

func (m *Module) audit(ctx context.Context, operation string, projectID uuid.UUID) {
	if m.auditor == nil || projectID == uuid.Nil {
		return
	}
	actor, ok := contextx.ActorFrom(ctx)
	if !ok {
		return
	}
	err := m.auditor.Record(ctx, port.AuditEvent{
		ActorUserID: actor.UserID, ProjectID: projectID.String(), Action: "mcp." + operation,
		EntityType: "project", EntityID: projectID.String(), RequestID: contextx.RequestID(ctx),
		Metadata: map[string]any{"transport": "streamable_http", "read_only": true},
	})
	if err != nil {
		logger.FromContext(ctx).Error("ghi MCP audit thất bại", zap.String("operation", operation), zap.Error(err))
	}
}

func safeError(ctx context.Context, operation string, err error) error {
	if err == nil {
		return nil
	}
	logger.FromContext(ctx).Error("MCP operation thất bại", zap.String("operation", operation), zap.Error(err))
	if business, ok := apperr.AsBusiness(err); ok {
		return errors.New(business.Code + ": " + business.Message)
	}
	if technical, ok := apperr.AsTechnical(err); ok {
		return errors.New(technical.Code + ": " + technical.Message)
	}
	return errors.New("SYS_500: Không thể xử lý yêu cầu MCP")
}

func parseID(value, field string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return uuid.Nil, fmt.Errorf("REQ_400: %s phải là UUID hợp lệ", field)
	}
	return id, nil
}

func parseScope(input scopeInput) (retrievaldomain.Scope, error) {
	mode := strings.TrimSpace(input.Mode)
	if mode == "" {
		switch {
		case len(input.VersionIDs) > 0 && len(input.ChangeRequestIDs) == 0:
			mode = retrievaldomain.ScopeVersions
		case len(input.ChangeRequestIDs) > 0 && len(input.VersionIDs) == 0:
			mode = retrievaldomain.ScopeChangeRequests
		default:
			mode = retrievaldomain.ScopeAll
		}
	}
	if len(input.VersionIDs)+len(input.ChangeRequestIDs) > 20 {
		return retrievaldomain.Scope{}, errors.New("REQ_400: Scope không được vượt quá 20 mốc")
	}
	scope := retrievaldomain.Scope{Mode: mode}
	for _, raw := range input.VersionIDs {
		id, err := parseID(raw, "version_id")
		if err != nil {
			return retrievaldomain.Scope{}, err
		}
		scope.VersionIDs = append(scope.VersionIDs, id)
	}
	for _, raw := range input.ChangeRequestIDs {
		id, err := parseID(raw, "change_request_id")
		if err != nil {
			return retrievaldomain.Scope{}, err
		}
		scope.ChangeRequestIDs = append(scope.ChangeRequestIDs, id)
	}
	if !scope.Valid() {
		return retrievaldomain.Scope{}, errors.New("REQ_400: Scope không hợp lệ")
	}
	return scope, nil
}

func (m *Module) readSource(
	ctx context.Context, projectID, documentID, revisionID uuid.UUID, start, end int,
) (*sourceOutput, error) {
	start, end, requestedEnd, err := normalizeLineRange(start, end, m.maxSourceLines)
	if err != nil {
		return nil, err
	}
	revision, reader, err := m.documents.CanonicalSource(ctx, projectID, documentID, revisionID)
	if err != nil {
		return nil, safeError(ctx, "get_document_source", err)
	}
	defer reader.Close()

	lines, err := markedLines(reader, start, end)
	if err != nil {
		return nil, safeError(ctx, "get_document_source", fmt.Errorf("đọc canonical source: %w", err))
	}
	if len(lines) == 0 {
		return nil, errors.New("NOT_FOUND: Khoảng dòng không tồn tại")
	}
	text, truncated := truncateRunes(strings.Join(lines, "\n"), m.maxExcerptChars)
	actualEnd := start + len(lines) - 1
	return &sourceOutput{
		ProjectID: projectID.String(), DocumentID: documentID.String(), RevisionID: revisionID.String(),
		FileName: revision.FileName, LineStart: start, LineEnd: actualEnd, Text: text,
		Truncated: truncated || requestedEnd == 0 || requestedEnd > end,
	}, nil
}

func normalizeLineRange(start, end, maxLines int) (int, int, int, error) {
	if start == 0 {
		start = 1
	}
	if start < 1 || start > 1_000_000 || end < 0 || end > 1_000_000 || end > 0 && end < start {
		return 0, 0, 0, errors.New("REQ_400: Khoảng dòng không hợp lệ")
	}
	requestedEnd := end
	if end == 0 || end-start+1 > maxLines {
		end = start + maxLines - 1
	}
	return start, end, requestedEnd, nil
}

func markedLines(reader io.Reader, start, end int) ([]string, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	lines, current := make([]string, 0, end-start+1), 0
	for scanner.Scan() {
		current++
		if current >= start && current <= end {
			lines = append(lines, fmt.Sprintf("%d: %s", current, scanner.Text()))
		}
		if current >= end {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan canonical lines: %w", err)
	}
	return lines, nil
}

func truncateRunes(value string, maxChars int) (string, bool) {
	runes := []rune(value)
	if len(runes) <= maxChars {
		return value, false
	}
	return string(runes[:maxChars]), true
}

func resolvedScopes(items []retrievaldomain.ResolvedScope) []resolvedScopeOutput {
	out := make([]resolvedScopeOutput, 0, len(items))
	for _, item := range items {
		out = append(out, resolvedScopeOutput{ID: item.ID.String(), Type: item.Type, Label: item.Label})
	}
	return out
}

func retrievalCitations(items []retrievaluc.Citation) []citationOutput {
	out := make([]citationOutput, 0, len(items))
	for _, item := range items {
		score := item.Score
		out = append(out, citationOutput{
			Key: item.Key, ChunkID: item.ChunkID, DocumentID: item.DocumentID.String(),
			DocumentRevisionID: item.DocumentRevisionID.String(), DocumentName: item.DocumentName,
			ScopeType: item.ScopeType, ScopeLabel: item.ScopeLabel, LineStart: item.LineStart,
			LineEnd: item.LineEnd, PageStart: item.PageStart, PageEnd: item.PageEnd,
			Excerpt: item.Excerpt, SourceURL: item.SourceURL, Score: &score,
		})
	}
	return out
}

func chatCitations(items []chatdomain.Citation) []citationOutput {
	out := make([]citationOutput, 0, len(items))
	for _, item := range items {
		out = append(out, citationOutput{
			Key: item.Key, ChunkID: item.ChunkID, DocumentID: item.DocumentID.String(),
			DocumentRevisionID: item.DocumentRevisionID.String(), DocumentName: item.DocumentName,
			ScopeType: item.ScopeType, ScopeLabel: item.ScopeLabel, LineStart: item.LineStart,
			LineEnd: item.LineEnd, PageStart: item.PageStart, PageEnd: item.PageEnd,
			Excerpt: item.Excerpt, SourceURL: item.SourceURL,
		})
	}
	return out
}
