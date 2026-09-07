package usecase

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/common/port"
	"github.com/quangdung93/docs-hub-api/internal/module/report/domain"
)

type fakeRepo struct {
	role       string
	datasetID  string
	chatID     string
	projectErr error
	created    *domain.Report
	items      []domain.ReportItem
}

func (f *fakeRepo) MemberRole(context.Context, uuid.UUID, uuid.UUID) (string, error) {
	return f.role, nil
}
func (f *fakeRepo) ProjectMeta(context.Context, uuid.UUID) (string, string, error) {
	if f.projectErr != nil {
		return "", "", f.projectErr
	}
	return "Demo Project", "DEMO", nil
}
func (f *fakeRepo) RAGFlowDatasetID(context.Context, uuid.UUID) (string, error) {
	return f.datasetID, nil
}
func (f *fakeRepo) RAGFlowChatID(context.Context, uuid.UUID) (string, error) { return f.chatID, nil }
func (f *fakeRepo) SaveRAGFlowChatID(_ context.Context, _ uuid.UUID, chatID string) (string, error) {
	f.chatID = chatID
	return chatID, nil
}
func (f *fakeRepo) Create(_ context.Context, report domain.Report, items []domain.ReportItem) error {
	f.created = &report
	f.items = items
	return nil
}
func (*fakeRepo) ListHistory(context.Context, uuid.UUID, int, int) ([]domain.Report, int64, error) {
	return nil, 0, nil
}

type fakeRAG struct {
	completion    port.RAGChatCompletionResult
	completionErr error
}

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
	if f.completionErr != nil {
		return port.RAGChatCompletionResult{}, f.completionErr
	}
	return f.completion, nil
}

type fakeStore struct {
	putKey string
	putErr error
}

func (f *fakeStore) Put(_ context.Context, key string, _ []byte, _ string) (port.StoredObject, error) {
	if f.putErr != nil {
		return port.StoredObject{}, f.putErr
	}
	f.putKey = key
	return port.StoredObject{Key: key}, nil
}
func (*fakeStore) PutReader(context.Context, string, io.Reader, int64, string) (port.StoredObject, error) {
	return port.StoredObject{}, nil
}
func (*fakeStore) Get(context.Context, string) ([]byte, error) { return nil, nil }
func (*fakeStore) GetReader(context.Context, string) (io.ReadCloser, error) {
	return nil, nil
}
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

const validUATJSON = `{"items":[{"title":"Đăng nhập","steps":"Nhập user/pass hợp lệ",` +
	`"expected":"Đăng nhập thành công","source":"srs.docx"}]}`

func newService(repo *fakeRepo, rag *fakeRAG, store *fakeStore) *Service {
	return New(repo, fakeTx{}, rag, store, fixedClock{})
}

func TestGenerateUAT_ViewerBiTuChoi(t *testing.T) {
	t.Parallel()
	actor, pid := uuid.New(), uuid.New()
	svc := newService(&fakeRepo{role: "viewer", datasetID: "ds-1"}, &fakeRAG{}, &fakeStore{})
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actor.String()})
	_, err := svc.Generate(ctx, GenerateInput{ProjectID: pid, ReportType: domain.ReportTypeUAT})
	var technical *apperr.TechnicalError
	require.ErrorAs(t, err, &technical)
	require.Equal(t, 403, technical.HTTPStatus)
}

func TestGenerateUAT_ReportTypeChuaHoTro(t *testing.T) {
	t.Parallel()
	actor, pid := uuid.New(), uuid.New()
	svc := newService(&fakeRepo{role: "editor", datasetID: "ds-1"}, &fakeRAG{}, &fakeStore{})
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actor.String()})
	_, err := svc.Generate(ctx, GenerateInput{ProjectID: pid, ReportType: "invalid-type"})
	var technical *apperr.TechnicalError
	require.ErrorAs(t, err, &technical)
	require.Equal(t, 400, technical.HTTPStatus)
}

func TestGenerateUAT_DinhDangKhongHopLe(t *testing.T) {
	t.Parallel()
	actor, pid := uuid.New(), uuid.New()
	svc := newService(&fakeRepo{role: "editor", datasetID: "ds-1"}, &fakeRAG{}, &fakeStore{})
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actor.String()})
	_, err := svc.Generate(ctx, GenerateInput{ProjectID: pid, ReportType: domain.ReportTypeUAT, Format: "docx"})
	var technical *apperr.TechnicalError
	require.ErrorAs(t, err, &technical)
	require.Equal(t, 400, technical.HTTPStatus)
}

func TestGenerateUAT_ChuaDongBoRAGFlow(t *testing.T) {
	t.Parallel()
	actor, pid := uuid.New(), uuid.New()
	svc := newService(&fakeRepo{role: "editor", datasetID: ""}, &fakeRAG{}, &fakeStore{})
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actor.String()})
	_, err := svc.Generate(ctx, GenerateInput{ProjectID: pid, ReportType: domain.ReportTypeUAT})
	var technical *apperr.TechnicalError
	require.ErrorAs(t, err, &technical)
	require.Equal(t, 504, technical.HTTPStatus)
}

func TestGenerateUAT_RAGFlowTraJSONHong(t *testing.T) {
	t.Parallel()
	actor, pid := uuid.New(), uuid.New()
	rag := &fakeRAG{completion: port.RAGChatCompletionResult{Content: "không phải json"}}
	svc := newService(&fakeRepo{role: "editor", datasetID: "ds-1"}, rag, &fakeStore{})
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actor.String()})
	_, err := svc.Generate(ctx, GenerateInput{ProjectID: pid, ReportType: domain.ReportTypeUAT})
	var technical *apperr.TechnicalError
	require.ErrorAs(t, err, &technical)
	require.Equal(t, 504, technical.HTTPStatus)
}

func TestGenerateUAT_KhongCoItemTraLoi400(t *testing.T) {
	t.Parallel()
	actor, pid := uuid.New(), uuid.New()
	rag := &fakeRAG{completion: port.RAGChatCompletionResult{Content: `{"items":[]}`}}
	svc := newService(&fakeRepo{role: "editor", datasetID: "ds-1"}, rag, &fakeStore{})
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actor.String()})
	_, err := svc.Generate(ctx, GenerateInput{ProjectID: pid, ReportType: domain.ReportTypeUAT})
	var technical *apperr.TechnicalError
	require.ErrorAs(t, err, &technical)
	require.Equal(t, 400, technical.HTTPStatus)
}

func TestGenerateUAT_VuotGioiHanChiCatBotKhongLoi(t *testing.T) {
	t.Parallel()
	actor, pid := uuid.New(), uuid.New()
	items := make([]uatRAGItem, maxUATItems+20)
	for i := range items {
		items[i] = uatRAGItem{Title: "Case", Steps: "Bước", Expected: "Kết quả"}
	}
	response := uatRAGResponse{Items: items}
	raw, err := json.Marshal(response)
	require.NoError(t, err)
	rag := &fakeRAG{completion: port.RAGChatCompletionResult{Content: string(raw)}}
	repo := &fakeRepo{role: "editor", datasetID: "ds-1"}
	store := &fakeStore{}
	svc := newService(repo, rag, store)
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actor.String()})
	result, err := svc.Generate(ctx, GenerateInput{ProjectID: pid, ReportType: domain.ReportTypeUAT})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, repo.items, maxUATItems)
}

func TestGenerateUAT_HappyPathXLSXVaPDF(t *testing.T) {
	t.Parallel()
	for _, format := range []string{"", domain.FormatXLSX, domain.FormatPDF} {
		actor, pid := uuid.New(), uuid.New()
		rag := &fakeRAG{completion: port.RAGChatCompletionResult{Content: validUATJSON}}
		repo := &fakeRepo{role: "editor", datasetID: "ds-1"}
		store := &fakeStore{}
		svc := newService(repo, rag, store)
		ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actor.String()})
		result, err := svc.Generate(ctx, GenerateInput{ProjectID: pid, ReportType: domain.ReportTypeUAT, Format: format})
		require.NoError(t, err)
		require.NotEmpty(t, result.DownloadURL)
		require.NotNil(t, repo.created)
		require.Len(t, repo.items, 1)
		require.Equal(t, store.putKey, repo.created.FileKey)
	}
}

func TestGenerateUAT_ChatIDDuocTaiSuDung(t *testing.T) {
	t.Parallel()
	actor, pid := uuid.New(), uuid.New()
	rag := &fakeRAG{completion: port.RAGChatCompletionResult{Content: validUATJSON}}
	repo := &fakeRepo{role: "editor", datasetID: "ds-1", chatID: "existing-chat"}
	svc := newService(repo, rag, &fakeStore{})
	ctx := contextx.WithActor(context.Background(), contextx.Actor{UserID: actor.String()})
	_, err := svc.Generate(ctx, GenerateInput{ProjectID: pid, ReportType: domain.ReportTypeUAT})
	require.NoError(t, err)
	require.Equal(t, "existing-chat", repo.chatID)
}
