package ragflow

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/common/port"
)

func TestClient_DatasetDocumentAndRetrieval(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/system/healthz":
			_, _ = io.WriteString(w, `{"status":"ok"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/datasets":
			_, _ = io.WriteString(w, `{"code":0,"data":{"id":"ds-1","name":"project_1"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/datasets":
			require.Equal(t, "project_1", r.URL.Query().Get("name"))
			_, _ = io.WriteString(w, `{"code":0,"data":[{"id":"ds-1","name":"project_1"}]}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/datasets/ds-1":
			_, _ = io.WriteString(w, `{"code":0}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/datasets":
			_, _ = io.WriteString(w, `{"code":0}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/datasets/ds-1/documents":
			require.NoError(t, r.ParseMultipartForm(1<<20))
			file, header, err := r.FormFile("file")
			require.NoError(t, err)
			defer file.Close()
			raw, err := io.ReadAll(file)
			require.NoError(t, err)
			require.Equal(t, "revision.txt", header.Filename)
			require.Equal(t, "hello", string(raw))
			_, _ = io.WriteString(w, `{"code":0,"data":[{"id":"doc-1","name":"revision.txt","run":"UNSTART"}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/datasets/ds-1/documents":
			require.Equal(t, "doc-1", r.URL.Query().Get("id"))
			_, _ = io.WriteString(w, `{"code":0,"data":{"docs":[{"id":"doc-1","name":"revision.txt","run":3,"progress":1}]}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/datasets/ds-1/metadata/update":
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			selector := body["selector"].(map[string]any)
			require.Equal(t, []any{"doc-1"}, selector["document_ids"])
			_, _ = io.WriteString(w, `{"code":0,"data":{"updated":1}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/retrieval":
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, []any{"doc-1"}, body["document_ids"])
			_, _ = io.WriteString(w, `{"code":0,"data":{"total":1,"chunks":[{"id":"chunk-1","dataset_id":"ds-1","document_id":"doc-1","content":"answer","similarity":0.9}]}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/chats":
			require.Equal(t, "chat_project_1", r.URL.Query().Get("name"))
			_, _ = io.WriteString(w, `{"code":0,"data":{"chats":[]}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/chats":
			_, _ = io.WriteString(w, `{"code":0,"data":{"id":"chat-1","name":"chat_project_1","dataset_ids":["ds-1"]}}`)
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/chats/chat-1":
			_, _ = io.WriteString(w, `{"code":0,"data":{"id":"chat-1"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/openai/chat-1/chat/completions":
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, false, body["stream"])
			_, _ = io.WriteString(w, `{"model":"qwen@ragflow","choices":[{"message":{"content":"final answer","reference":{"chunks":{"2":{"id":"chunk-2","dataset_id":"ds-1","document_id":"doc-1","document_name":"revision.txt","content":"evidence","similarity":0.8}}}}}]}`)
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := New(server.URL, "test-key", 2*time.Second, 2*time.Second)
	require.NoError(t, client.Health(context.Background()))
	dataset, err := client.CreateDataset(context.Background(), "project_1", "Project")
	require.NoError(t, err)
	require.Equal(t, "ds-1", dataset.ID)
	found, err := client.FindDatasetByName(context.Background(), "project_1")
	require.NoError(t, err)
	require.Equal(t, "ds-1", found.ID)
	require.NoError(t, client.UpdateDataset(context.Background(), "ds-1", "project_1", "Project"))
	document, err := client.UploadDocument(context.Background(), "ds-1", port.RAGDocumentFile{
		Name: "revision.txt", ContentType: "text/plain", Reader: strings.NewReader("hello"),
	})
	require.NoError(t, err)
	require.Equal(t, "doc-1", document.ID)
	document, err = client.GetDocument(context.Background(), "ds-1", "doc-1")
	require.NoError(t, err)
	require.True(t, document.Ready())
	require.NoError(t, client.UpdateDocumentMetadata(context.Background(), "ds-1", []string{"doc-1"}, map[string]string{
		"docs_hub_scope_id": "version-1",
	}))
	result, err := client.Retrieve(context.Background(), port.RAGRetrievalRequest{
		Question: "question", DatasetIDs: []string{"ds-1"}, DocumentIDs: []string{"doc-1"},
		Page: 1, PageSize: 10, SimilarityThreshold: 0.2, VectorSimilarityWeight: 0.3,
	})
	require.NoError(t, err)
	require.Len(t, result.Chunks, 1)
	require.Equal(t, "answer", result.Chunks[0].Content)
	chat, err := client.FindChatByName(context.Background(), "chat_project_1")
	require.NoError(t, err)
	require.Nil(t, chat)
	createdChat, err := client.CreateChat(context.Background(), "chat_project_1", []string{"ds-1"})
	require.NoError(t, err)
	require.Equal(t, "chat-1", createdChat.ID)
	require.NoError(t, client.UpdateChatDatasets(context.Background(), "chat-1", []string{"ds-1"}))
	completion, err := client.CompleteChat(context.Background(), port.RAGChatCompletionRequest{
		ChatID: "chat-1", Messages: []port.RAGChatMessage{{Role: "user", Content: "question"}},
		MetadataConditions: []port.RAGMetadataCondition{{Name: "docs_hub_scope_id", Operator: "is", Value: "version-1"}},
	})
	require.NoError(t, err)
	require.Equal(t, "final answer", completion.Content)
	require.Equal(t, "qwen@ragflow", completion.Model)
	require.Len(t, completion.References, 1)
	require.Equal(t, "chunk-2", completion.References[0].ID)
	require.NoError(t, client.DeleteDatasets(context.Background(), []string{"ds-1"}))
}

func TestClient_MapsEnvelopeError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"code":102,"message":"invalid key"}`)
	}))
	defer server.Close()
	client := New(server.URL, "bad", time.Second, time.Second)
	_, err := client.CreateDataset(context.Background(), "x", "")
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusUnauthorized, apiErr.HTTPStatus)
	require.False(t, apiErr.Retryable)
}

func TestClient_CompleteChat_ReferenceLaMang(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/openai/chat-1/chat/completions", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"model":"gpt-4.1-mini","choices":[{"message":{"content":"final answer","reference":[{"id":"chunk-1","dataset_id":"ds-1","document_id":"doc-1","document_name":"revision.txt","content":"evidence","similarity":0.9}]}}]}`)
	}))
	defer server.Close()

	client := New(server.URL, "test-key", time.Second, time.Second)
	completion, err := client.CompleteChat(context.Background(), port.RAGChatCompletionRequest{
		ChatID: "chat-1", Messages: []port.RAGChatMessage{{Role: "user", Content: "question"}},
	})

	require.NoError(t, err)
	require.Equal(t, "final answer", completion.Content)
	require.Len(t, completion.References, 1)
	require.Equal(t, "chunk-1", completion.References[0].ID)
}

func TestClient_FindDocumentByName_DatasetRongKhongBiCode102(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/datasets/ds-empty/documents", r.URL.Path)
		// RAGFlow thật trả code=102 nếu truyền name mà document chưa tồn tại.
		if r.URL.Query().Get("name") != "" {
			_, _ = io.WriteString(w, `{"code":102,"message":"you don't own the document"}`)
			return
		}
		require.Equal(t, "revision__file.md", r.URL.Query().Get("keywords"))
		_, _ = io.WriteString(w, `{"code":0,"message":"success","data":{"total":0,"docs":[]}}`)
	}))
	defer server.Close()

	client := New(server.URL, "test-key", time.Second, time.Second)
	document, err := client.FindDocumentByName(context.Background(), "ds-empty", "revision__file.md")

	require.NoError(t, err)
	require.Nil(t, document)
}

func TestClient_FindDocumentByName_LocExactNameTuKeywords(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"code":0,"data":{"total":2,"docs":[`+
			`{"id":"doc-near","name":"revision__file.md.bak"},`+
			`{"id":"doc-exact","name":"revision__file.md"}]}}`)
	}))
	defer server.Close()

	client := New(server.URL, "test-key", time.Second, time.Second)
	document, err := client.FindDocumentByName(context.Background(), "ds-1", "revision__file.md")

	require.NoError(t, err)
	require.NotNil(t, document)
	require.Equal(t, "doc-exact", document.ID)
}

// TestClient_FindDatasetByName_CoiCodeNotOwnedLaKhongTimThay chốt lỗi đã gặp
// thật trên production: RAGFlow trả code=102 "lacks permission" cho MỌI tên
// dataset user không sở hữu, kể cả tên chưa từng tồn tại. Coi đó là lỗi thì
// nhánh tạo dataset không bao giờ chạy, và mọi tài liệu kẹt ở trạng thái lỗi.
func TestClient_FindDatasetByName_CoiCodeNotOwnedLaKhongTimThay(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":102,"message":"User 'u1' lacks permission for dataset 'ds_moi'"}`)
	}))
	defer server.Close()

	client := New(server.URL, "test-key", time.Second, time.Second)
	dataset, err := client.FindDatasetByName(context.Background(), "ds_moi")

	require.NoError(t, err, "code=102 phải hiểu là chưa có, không phải lỗi")
	require.Nil(t, dataset)
}

// TestClient_FindDatasetByName_LoiKhacVanBaoLoi: chỉ codeNotOwned mới được bỏ
// qua; lỗi xác thực hay lỗi hệ thống vẫn phải nổi lên.
func TestClient_FindDatasetByName_LoiKhacVanBaoLoi(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"code":401,"message":"Unauthorized"}`)
	}))
	defer server.Close()

	client := New(server.URL, "test-key", time.Second, time.Second)
	_, err := client.FindDatasetByName(context.Background(), "ds_moi")

	require.Error(t, err, "khóa sai mà nuốt lỗi thì worker im lặng tạo dataset hỏng")
}
