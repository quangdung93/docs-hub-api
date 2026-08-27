package mcpserver

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/common/port"
	documentdomain "github.com/quangdung93/docs-hub-api/internal/module/document/domain"
	retrievaldomain "github.com/quangdung93/docs-hub-api/internal/module/retrieval/domain"
	retrievaluc "github.com/quangdung93/docs-hub-api/internal/module/retrieval/usecase"
)

type fakeRetrieval struct {
	input  retrievaluc.Input
	result *retrievaluc.Result
}

func (f *fakeRetrieval) Retrieve(_ context.Context, input retrievaluc.Input) (*retrievaluc.Result, error) {
	f.input = input
	return f.result, nil
}

type fakeDocuments struct {
	revision *documentdomain.Revision
	content  string
}

func (f *fakeDocuments) CanonicalSource(
	context.Context, uuid.UUID, uuid.UUID, uuid.UUID,
) (*documentdomain.Revision, io.ReadCloser, error) {
	return f.revision, io.NopCloser(strings.NewReader(f.content)), nil
}

type fakeCache struct{ count int64 }

func (*fakeCache) Get(context.Context, string) (string, error)              { return "", port.ErrCacheMiss }
func (*fakeCache) Set(context.Context, string, string, time.Duration) error { return nil }
func (*fakeCache) Del(context.Context, ...string) error                     { return nil }
func (f *fakeCache) Incr(context.Context, string, time.Duration) (int64, error) {
	f.count++
	return f.count, nil
}

func actorContext() context.Context {
	return contextx.WithActor(context.Background(), contextx.Actor{UserID: uuid.NewString()})
}

func TestServer_DangKyDuSauToolsVaBaResourceTemplates(t *testing.T) {
	module := New(Deps{RequestsPerWindow: 30, Window: time.Minute, MaxSourceLines: 20, MaxExcerptChars: 4096})
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	_, err := module.server.Connect(context.Background(), serverTransport, nil)
	require.NoError(t, err)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1"}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	toolCount := 0
	for _, err = range session.Tools(context.Background(), nil) {
		require.NoError(t, err)
		toolCount++
	}
	templateCount := 0
	for _, err = range session.ResourceTemplates(context.Background(), nil) {
		require.NoError(t, err)
		templateCount++
	}
	require.Equal(t, 6, toolCount)
	require.Equal(t, 3, templateCount)
}

func TestStreamableHTTP_HandshakeJSON(t *testing.T) {
	module := New(Deps{RequestsPerWindow: 30, Window: time.Minute, MaxSourceLines: 20, MaxExcerptChars: 4096})
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{` +
		`"protocolVersion":"2025-11-25","capabilities":{},` +
		`"clientInfo":{"name":"test","version":"v1"}}}`)
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	response := httptest.NewRecorder()

	module.Handler().ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"protocolVersion":"2025-11-25"`)
	require.Contains(t, response.Body.String(), `"tools"`)
}

func TestSearchProject_ChuyenScopeVaTraCitationCoCauTruc(t *testing.T) {
	projectID, versionID := uuid.New(), uuid.New()
	retrieval := &fakeRetrieval{result: &retrievaluc.Result{
		Query: "quyen", ResolvedScope: []retrievaldomain.ResolvedScope{{ID: versionID, Type: "version", Label: "v1"}},
		Citations: []retrievaluc.Citation{{Key: "S1", DocumentID: uuid.New(), DocumentRevisionID: uuid.New(), Excerpt: "nguon"}},
		Total:     1,
	}}
	module := New(Deps{Retrieval: retrieval, RequestsPerWindow: 30, Window: time.Minute, MaxSourceLines: 20, MaxExcerptChars: 4096})

	_, output, err := module.searchProject(actorContext(), nil, searchProjectInput{
		ProjectID: projectID.String(), Query: "quyen",
		Scope: scopeInput{VersionIDs: []string{versionID.String()}}, Limit: 5,
	})

	require.NoError(t, err)
	require.Equal(t, retrievaldomain.ScopeVersions, retrieval.input.Scope.Mode)
	require.Equal(t, versionID, retrieval.input.Scope.VersionIDs[0])
	require.Len(t, output.Citations, 1)
	require.Equal(t, "S1", output.Citations[0].Key)
}

func TestDocumentSource_GioiHanDongVaKyTu(t *testing.T) {
	projectID, documentID, revisionID := uuid.New(), uuid.New(), uuid.New()
	module := New(Deps{
		Documents: &fakeDocuments{
			revision: &documentdomain.Revision{ID: revisionID, FileName: "spec.md"},
			content:  "dòng một\ndòng hai rất dài\ndòng ba\ndòng bốn",
		},
		RequestsPerWindow: 30, Window: time.Minute, MaxSourceLines: 2, MaxExcerptChars: 12,
	})

	_, output, err := module.getDocumentSource(actorContext(), nil, sourceInput{
		ProjectID: projectID.String(), DocumentID: documentID.String(), RevisionID: revisionID.String(),
		LineStart: 2, LineEnd: 4,
	})

	require.NoError(t, err)
	require.Equal(t, 2, output.LineStart)
	require.Equal(t, 3, output.LineEnd)
	require.LessOrEqual(t, len([]rune(output.Text)), 12)
	require.True(t, output.Truncated)
}

func TestRateLimit_TheoPrincipalVaOperation(t *testing.T) {
	cache := &fakeCache{}
	module := New(Deps{Cache: cache, RequestsPerWindow: 1, Window: time.Minute, MaxSourceLines: 20, MaxExcerptChars: 4096})
	ctx := actorContext()

	require.NoError(t, module.allow(ctx, "search_project"))
	err := module.allow(ctx, "search_project")

	require.EqualError(t, err, "RATE_429: Quá nhiều yêu cầu MCP, vui lòng thử lại sau")
}

func TestSafeError_KhongLoNguyenNhanNoiBo(t *testing.T) {
	err := apperr.Database("Không thể truy vấn tài liệu").WithCause(errors.New("SELECT secret FROM users"))

	safe := safeError(context.Background(), "search_project", err)

	require.Contains(t, safe.Error(), "Không thể truy vấn tài liệu")
	require.NotContains(t, safe.Error(), "SELECT secret")
}
