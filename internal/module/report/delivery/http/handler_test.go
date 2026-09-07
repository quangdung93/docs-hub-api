package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/common/port"
	"github.com/quangdung93/docs-hub-api/internal/middleware"
	reporthttp "github.com/quangdung93/docs-hub-api/internal/module/report/delivery/http"
	"github.com/quangdung93/docs-hub-api/internal/module/report/domain"
	"github.com/quangdung93/docs-hub-api/internal/module/report/usecase"
)

type fakeRepo struct{ role string }

func (f *fakeRepo) MemberRole(context.Context, uuid.UUID, uuid.UUID) (string, error) {
	return f.role, nil
}
func (*fakeRepo) ProjectMeta(context.Context, uuid.UUID) (string, string, error) {
	return "Demo", "DEMO", nil
}
func (*fakeRepo) RAGFlowDatasetID(context.Context, uuid.UUID) (string, error) { return "ds-1", nil }
func (*fakeRepo) RAGFlowChatID(context.Context, uuid.UUID) (string, error)    { return "chat-1", nil }
func (*fakeRepo) SaveRAGFlowChatID(_ context.Context, _ uuid.UUID, chatID string) (string, error) {
	return chatID, nil
}
func (*fakeRepo) Create(context.Context, domain.Report, []domain.ReportItem) error { return nil }
func (*fakeRepo) ListHistory(context.Context, uuid.UUID, int, int) ([]domain.Report, int64, error) {
	return nil, 0, nil
}

type fakeRAG struct{ content string }

func (*fakeRAG) Health(context.Context) error { return nil }
func (*fakeRAG) CreateDataset(context.Context, string, string) (port.RAGDataset, error) {
	return port.RAGDataset{}, nil
}
func (*fakeRAG) FindDatasetByName(context.Context, string) (*port.RAGDataset, error) { return nil, nil }
func (*fakeRAG) UpdateDataset(context.Context, string, string, string) error         { return nil }
func (*fakeRAG) DeleteDatasets(context.Context, []string) error                      { return nil }
func (*fakeRAG) UploadDocument(context.Context, string, port.RAGDocumentFile) (port.RAGDocument, error) {
	return port.RAGDocument{}, nil
}
func (*fakeRAG) GetDocument(context.Context, string, string) (port.RAGDocument, error) {
	return port.RAGDocument{}, nil
}
func (*fakeRAG) FindDocumentByName(context.Context, string, string) (*port.RAGDocument, error) {
	return nil, nil
}
func (*fakeRAG) StartParsing(context.Context, string, []string) error    { return nil }
func (*fakeRAG) StopParsing(context.Context, string, []string) error     { return nil }
func (*fakeRAG) DeleteDocuments(context.Context, string, []string) error { return nil }
func (*fakeRAG) UpdateDocumentMetadata(context.Context, string, []string, map[string]string) error {
	return nil
}
func (*fakeRAG) Retrieve(context.Context, port.RAGRetrievalRequest) (port.RAGRetrievalResult, error) {
	return port.RAGRetrievalResult{}, nil
}
func (*fakeRAG) CreateChat(_ context.Context, name string, datasetIDs []string) (port.RAGChat, error) {
	return port.RAGChat{ID: "chat-1", Name: name, DatasetIDs: datasetIDs}, nil
}
func (*fakeRAG) FindChatByName(context.Context, string) (*port.RAGChat, error) { return nil, nil }
func (*fakeRAG) UpdateChatDatasets(context.Context, string, []string) error    { return nil }
func (f *fakeRAG) CompleteChat(context.Context, port.RAGChatCompletionRequest) (port.RAGChatCompletionResult, error) {
	return port.RAGChatCompletionResult{Content: f.content}, nil
}

type fakeStore struct{}

func (*fakeStore) Put(context.Context, string, []byte, string) (port.StoredObject, error) {
	return port.StoredObject{}, nil
}
func (*fakeStore) PutReader(context.Context, string, io.Reader, int64, string) (port.StoredObject, error) {
	return port.StoredObject{}, nil
}
func (*fakeStore) Get(context.Context, string) ([]byte, error)              { return nil, nil }
func (*fakeStore) GetReader(context.Context, string) (io.ReadCloser, error) { return nil, nil }
func (*fakeStore) Stat(context.Context, string) (port.StoredObject, error) {
	return port.StoredObject{}, nil
}
func (*fakeStore) PresignedPutURL(context.Context, string, time.Duration) (string, error) {
	return "", nil
}
func (*fakeStore) PresignedGetURL(context.Context, string, time.Duration) (string, error) {
	return "https://example.local/download", nil
}
func (*fakeStore) Delete(context.Context, string) error { return nil }

type fakeTx struct{}

func (fakeTx) Do(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC) }

func fakeAuth(userID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request = c.Request.WithContext(contextx.WithActor(c.Request.Context(), contextx.Actor{UserID: userID}))
		c.Next()
	}
}

func setupRouter(t *testing.T, svc *usecase.Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID(), middleware.ErrorHandler(), fakeAuth(uuid.NewString()))
	reporthttp.Register(r.Group("/internal/api/v1"), reporthttp.New(svc))
	return r
}

func TestGenerate_BindLoiTra400(t *testing.T) {
	t.Parallel()
	svc := usecase.New(&fakeRepo{role: "editor"}, fakeTx{}, &fakeRAG{}, &fakeStore{}, fixedClock{})
	r := setupRouter(t, svc)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		"/internal/api/v1/projects/"+uuid.NewString()+"/reports", bytes.NewBufferString("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGenerate_HappyPathTraEnvelopeChuan(t *testing.T) {
	t.Parallel()
	const validJSON = `{"items":[{"title":"Đăng nhập","steps":"B","expected":"C","source":"D"}]}`
	svc := usecase.New(&fakeRepo{role: "editor"}, fakeTx{}, &fakeRAG{content: validJSON}, &fakeStore{}, fixedClock{})
	r := setupRouter(t, svc)
	body, err := json.Marshal(reporthttp.GenerateRequest{ReportType: domain.ReportTypeUAT, Format: domain.FormatXLSX})
	require.NoError(t, err)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		"/internal/api/v1/projects/"+uuid.NewString()+"/reports", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	var env map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.True(t, env["success"].(bool))
	data := env["data"].(map[string]any)
	require.NotEmpty(t, data["download_url"])
}
