package usecase

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/common/errcode"
	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	"github.com/quangdung93/docs-hub-api/internal/common/port"
	documentdomain "github.com/quangdung93/docs-hub-api/internal/module/document/domain"
	documentusecase "github.com/quangdung93/docs-hub-api/internal/module/document/usecase"
	"github.com/quangdung93/docs-hub-api/internal/module/urd/domain"
)

// ---- fake document.domain.Repository — đủ dùng cho Analyze/finalizeAnalysis ----

type fakeDocRepo struct {
	doc       documentdomain.Document
	revisions []documentdomain.Revision
}

func (f *fakeDocRepo) MemberRole(context.Context, uuid.UUID, uuid.UUID) (string, error) {
	return "editor", nil
}
func (*fakeDocRepo) ScopeExists(context.Context, uuid.UUID, documentdomain.Scope) (bool, error) {
	return true, nil
}
func (f *fakeDocRepo) CreateRevision(
	_ context.Context, in documentdomain.CreateRevisionParams,
) (*documentdomain.Document, *documentdomain.Revision, error) {
	rev := documentdomain.Revision{
		ID: in.RevisionID, DocumentID: in.DocumentID, ProjectID: in.ProjectID, Scope: in.Scope,
		RevisionNo: len(f.revisions) + 1, FileName: in.FileName, MediaType: in.MediaType,
		SHA256: in.SHA256, ObjectKey: in.ObjectKey, SizeBytes: in.SizeBytes, Status: "queued",
	}
	f.revisions = append(f.revisions, rev)
	return &f.doc, &rev, nil
}
func (*fakeDocRepo) CreateUpload(context.Context, *documentdomain.Upload) error { return nil }
func (*fakeDocRepo) FindUpload(context.Context, uuid.UUID, uuid.UUID) (*documentdomain.Upload, error) {
	return nil, documentdomain.ErrNotFound
}
func (*fakeDocRepo) CompleteUpload(
	context.Context, *documentdomain.Upload,
) (*documentdomain.Document, *documentdomain.Revision, error) {
	return nil, nil, nil
}
func (*fakeDocRepo) List(
	context.Context, uuid.UUID, documentdomain.Filter, pagination.Query,
) ([]documentdomain.Document, int64, error) {
	return nil, 0, nil
}
func (f *fakeDocRepo) FindDocument(
	context.Context, uuid.UUID, uuid.UUID,
) (*documentdomain.Document, []documentdomain.Revision, error) {
	return &f.doc, f.revisions, nil
}
func (f *fakeDocRepo) FindRevision(_ context.Context, _, _, rid uuid.UUID) (*documentdomain.Revision, error) {
	for i := range f.revisions {
		if f.revisions[i].ID == rid {
			return &f.revisions[i], nil
		}
	}
	return nil, documentdomain.ErrNotFound
}
func (*fakeDocRepo) Update(
	context.Context, uuid.UUID, uuid.UUID, string, string, int,
) (*documentdomain.Document, error) {
	return nil, nil
}
func (*fakeDocRepo) SetDocType(
	context.Context, uuid.UUID, uuid.UUID, string, int, uuid.UUID,
) (*documentdomain.Document, error) {
	return nil, nil
}
func (*fakeDocRepo) Retry(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}
func (*fakeDocRepo) SoftDelete(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error { return nil }
func (*fakeDocRepo) ProjectMeta(context.Context, uuid.UUID) (string, string, error) {
	return "Demo Project", "DEMO", nil
}
func (*fakeDocRepo) ScopeMeta(context.Context, uuid.UUID, documentdomain.Scope) (string, string, error) {
	return "Toàn bộ tài liệu dự án", "", nil
}
func (*fakeDocRepo) UATItems(context.Context, uuid.UUID, documentdomain.Scope) ([]documentdomain.UATItem, error) {
	return nil, nil
}

// ---- fake ObjectStore (map key->bytes, đủ cho Put/PutReader/GetReader) ----

type fakeStore struct{ objects map[string][]byte }

func newFakeStore() *fakeStore { return &fakeStore{objects: map[string][]byte{}} }

func (f *fakeStore) Put(_ context.Context, key string, data []byte, ct string) (port.StoredObject, error) {
	f.objects[key] = data
	return port.StoredObject{Key: key, Size: int64(len(data)), ContentType: ct}, nil
}
func (f *fakeStore) PutReader(
	_ context.Context, key string, r io.Reader, _ int64, ct string,
) (port.StoredObject, error) {
	data, _ := io.ReadAll(r)
	f.objects[key] = data
	return port.StoredObject{Key: key, Size: int64(len(data)), ContentType: ct}, nil
}
func (f *fakeStore) Get(_ context.Context, key string) ([]byte, error) { return f.objects[key], nil }
func (f *fakeStore) GetReader(_ context.Context, key string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(f.objects[key])), nil
}
func (f *fakeStore) Stat(_ context.Context, key string) (port.StoredObject, error) {
	return port.StoredObject{Size: int64(len(f.objects[key]))}, nil
}
func (*fakeStore) PresignedPutURL(context.Context, string, time.Duration) (string, error) {
	return "", nil
}
func (*fakeStore) PresignedGetURL(context.Context, string, time.Duration) (string, error) {
	return "", nil
}
func (f *fakeStore) Delete(_ context.Context, key string) error { delete(f.objects, key); return nil }

type fakeTx struct{}

func (fakeTx) Do(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type fakeClock struct{}

func (fakeClock) Now() time.Time { return time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC) }

// ---- fake urd domain.Repository ----

type fakeUrdRepo struct {
	active      *domain.Analysis
	activeCases []domain.EdgeCase
	analyses    map[uuid.UUID]*domain.Analysis
	cases       map[uuid.UUID][]domain.EdgeCase
}

func newFakeUrdRepo() *fakeUrdRepo {
	return &fakeUrdRepo{analyses: map[uuid.UUID]*domain.Analysis{}, cases: map[uuid.UUID][]domain.EdgeCase{}}
}

func (f *fakeUrdRepo) CreateAnalysis(_ context.Context, a domain.Analysis, cases []domain.EdgeCase) error {
	cp := a
	f.analyses[a.ID] = &cp
	f.cases[a.ID] = cases
	return nil
}
func (f *fakeUrdRepo) GetActiveAnalysis(context.Context, uuid.UUID) (*domain.Analysis, []domain.EdgeCase, error) {
	return f.active, f.activeCases, nil
}
func (f *fakeUrdRepo) GetAnalysis(_ context.Context, id uuid.UUID) (*domain.Analysis, []domain.EdgeCase, error) {
	a, ok := f.analyses[id]
	if !ok {
		return nil, nil, domain.ErrNotFound
	}
	return a, f.cases[id], nil
}
func (f *fakeUrdRepo) SaveResolutions(
	_ context.Context, id uuid.UUID, cases []domain.EdgeCase,
) (*domain.Analysis, error) {
	byID := make(map[uuid.UUID]domain.EdgeCase, len(cases))
	for _, c := range cases {
		byID[c.ID] = c
	}
	existing := f.cases[id]
	resolved := 0
	for i, c := range existing {
		if updated, ok := byID[c.ID]; ok {
			existing[i] = updated
		}
		if existing[i].Resolved {
			resolved++
		}
	}
	f.cases[id] = existing
	a := f.analyses[id]
	a.ResolvedCases = resolved
	return a, nil
}
func (f *fakeUrdRepo) MarkCompleted(_ context.Context, id uuid.UUID) (*domain.Analysis, error) {
	a := f.analyses[id]
	a.Status = domain.StatusCompleted
	return a, nil
}

// Cancel bắt chước câu UPDATE có điều kiện status của repository thật: chỉ đổi
// được khi phân tích còn đang hoạt động.
func (f *fakeUrdRepo) Cancel(_ context.Context, id uuid.UUID) (*domain.Analysis, error) {
	a, ok := f.analyses[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	if a.Status != domain.StatusAnalyzing && a.Status != domain.StatusAwaitingInput {
		return nil, domain.ErrAnalysisNotActive
	}
	a.Status = domain.StatusCancelled
	return a, nil
}
func (*fakeUrdRepo) Summaries(context.Context, uuid.UUID) (map[uuid.UUID]domain.Analysis, error) {
	return nil, nil
}
func (*fakeUrdRepo) MemberRole(context.Context, uuid.UUID, uuid.UUID) (string, error) {
	return "editor", nil
}
func (*fakeUrdRepo) RAGFlowDatasetID(context.Context, uuid.UUID) (string, error) { return "ds-1", nil }
func (*fakeUrdRepo) RAGFlowChatID(context.Context, uuid.UUID) (string, error)    { return "chat-1", nil }
func (*fakeUrdRepo) SaveRAGFlowChatID(_ context.Context, _ uuid.UUID, proposed string) (string, error) {
	return proposed, nil
}

// ---- fake port.RAGClient (chỉ CompleteChat trả nội dung cấu hình sẵn, phần
// còn lại chỉ để thỏa interface — ensureChat gọi FindChatByName/CreateChat) ----

type fakeRAG struct {
	completeChatContent string
	completeChatErr     error
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
func (*fakeRAG) CreateChat(context.Context, string, []string) (port.RAGChat, error) {
	return port.RAGChat{ID: "chat-1"}, nil
}
func (*fakeRAG) FindChatByName(context.Context, string) (*port.RAGChat, error) {
	return &port.RAGChat{ID: "chat-1", DatasetIDs: []string{"ds-1"}}, nil
}
func (*fakeRAG) UpdateChatDatasets(context.Context, string, []string) error { return nil }
func (f *fakeRAG) CompleteChat(
	context.Context, port.RAGChatCompletionRequest,
) (port.RAGChatCompletionResult, error) {
	if f.completeChatErr != nil {
		return port.RAGChatCompletionResult{}, f.completeChatErr
	}
	return port.RAGChatCompletionResult{Content: f.completeChatContent}, nil
}

// minimalDocx dựng 1 file .docx tối giản (1 entry word/document.xml, 1 đoạn
// văn + 1 sectPr) — đủ để docxmerge.Merge chèn nội dung.
func minimalDocx(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("word/document.xml")
	require.NoError(t, err)
	_, err = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body><w:p><w:r><w:t>Noi dung URD goc</w:t></w:r></w:p>
<w:sectPr><w:pgSz w:w="12240" w:h="15840"/></w:sectPr></w:body></w:document>`))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

const testMediaTypeDOCX = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

// newTestService dựng urd usecase.Service với document.Service thật (bọc fake
// bên dưới) — tái dùng đúng logic ACL/CreateRevision/CanonicalSource thay vì
// fake lại toàn bộ, giống cách document module tự test usecase của nó.
func newTestService(
	t *testing.T, docRepo *fakeDocRepo, store *fakeStore, urdRepo *fakeUrdRepo, rag *fakeRAG,
) *Service {
	t.Helper()
	docSvc := documentusecase.New(docRepo, fakeTx{}, store, fakeClock{}, documentusecase.WithProjectACLBypass(true))
	return New(urdRepo, fakeTx{}, rag, store, fakeClock{}, docSvc, WithProjectACLBypass(true))
}

func withActor(ctx context.Context) context.Context {
	return contextx.WithActor(ctx, contextx.Actor{UserID: uuid.New().String()})
}

func businessCode(t *testing.T, err error) string {
	t.Helper()
	var be *apperr.BusinessError
	require.ErrorAs(t, err, &be)
	return be.Code
}

// dungPhanTichDangDo dựng sẵn 1 phân tích awaiting_input để thử luồng huỷ.
func dungPhanTichDangDo(t *testing.T) (*Service, *fakeUrdRepo, uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	pid, did, rid, aid := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	docRepo := &fakeDocRepo{
		doc: documentdomain.Document{ID: did, ProjectID: pid, DocType: documentdomain.DocTypeURD},
		revisions: []documentdomain.Revision{
			{ID: rid, Status: revisionStatusReady, CanonicalTextKey: "canon"},
		},
	}
	urdRepo := newFakeUrdRepo()
	urdRepo.analyses[aid] = &domain.Analysis{
		ID: aid, DocumentID: did, RevisionID: rid,
		Status: domain.StatusAwaitingInput, TotalCases: 29,
	}
	return newTestService(t, docRepo, newFakeStore(), urdRepo, &fakeRAG{}), urdRepo, pid, did, aid
}

// Huỷ là lối ra DUY NHẤT cho tài liệu bị phân tích nhầm: trước khi có API này
// phân tích chỉ rời awaiting_input bằng cách nhập đủ hướng giải quyết cho MỌI
// case, mà uk_urd_analyses_active lại chặn phân tích lại.
func TestCancel_GoKhoaTaiLieuDangKet(t *testing.T) {
	svc, urdRepo, pid, did, aid := dungPhanTichDangDo(t)

	a, err := svc.Cancel(withActor(context.Background()), pid, did, aid)

	require.NoError(t, err)
	require.Equal(t, domain.StatusCancelled, a.Status)
	require.Equal(t, domain.StatusCancelled, urdRepo.analyses[aid].Status,
		"phải ghi xuống repo chứ không chỉ đổi bản sao trả về")
	require.NotContains(t, domain.ActiveStatuses(), a.Status,
		"trạng thái sau khi huỷ phải nằm NGOÀI uk_urd_analyses_active")
}

// Huỷ lần hai (hoặc huỷ phân tích đã completed) là lỗi nghiệp vụ riêng, không
// phải NotFound — client cần nói đúng "đã xong rồi" thay vì "không tìm thấy".
func TestCancel_PhanTichDaKetThuc_TraLoiNghiepVuRieng(t *testing.T) {
	svc, urdRepo, pid, did, aid := dungPhanTichDangDo(t)
	urdRepo.analyses[aid].Status = domain.StatusCompleted

	_, err := svc.Cancel(withActor(context.Background()), pid, did, aid)

	require.Equal(t, errcode.URDAnalysisNotActive, businessCode(t, err))
}

// Id phân tích đúng nhưng thuộc tài liệu khác: coi như không tồn tại, không
// được huỷ chéo tài liệu.
func TestCancel_PhanTichThuocTaiLieuKhac(t *testing.T) {
	svc, _, pid, _, aid := dungPhanTichDangDo(t)

	_, err := svc.Cancel(withActor(context.Background()), pid, uuid.New(), aid)

	require.Error(t, err)
	var be *apperr.BusinessError
	require.NotErrorAs(t, err, &be, "phải là lỗi kỹ thuật 404, không phải lỗi nghiệp vụ")
}

func TestAnalyze_ChuaXacNhanURD(t *testing.T) {
	pid, did := uuid.New(), uuid.New()
	docRepo := &fakeDocRepo{doc: documentdomain.Document{ID: did, ProjectID: pid, DocType: ""}}
	svc := newTestService(t, docRepo, newFakeStore(), newFakeUrdRepo(), &fakeRAG{})

	_, _, err := svc.Analyze(withActor(context.Background()), pid, did)

	require.Equal(t, errcode.URDNotConfirmed, businessCode(t, err))
}

func TestAnalyze_RevisionChuaSanSang(t *testing.T) {
	pid, did := uuid.New(), uuid.New()
	docRepo := &fakeDocRepo{
		doc:       documentdomain.Document{ID: did, ProjectID: pid, DocType: documentdomain.DocTypeURD},
		revisions: []documentdomain.Revision{{ID: uuid.New(), Status: "queued"}},
	}
	svc := newTestService(t, docRepo, newFakeStore(), newFakeUrdRepo(), &fakeRAG{})

	_, _, err := svc.Analyze(withActor(context.Background()), pid, did)

	require.Equal(t, errcode.URDRevisionNotReady, businessCode(t, err))
}

func TestAnalyze_DangCoPhanTichActive(t *testing.T) {
	pid, did, rid := uuid.New(), uuid.New(), uuid.New()
	docRepo := &fakeDocRepo{
		doc: documentdomain.Document{ID: did, ProjectID: pid, DocType: documentdomain.DocTypeURD},
		revisions: []documentdomain.Revision{
			{ID: rid, Status: revisionStatusReady, CanonicalTextKey: "canon"},
		},
	}
	urdRepo := newFakeUrdRepo()
	activeID := uuid.New()
	urdRepo.active = &domain.Analysis{ID: activeID, DocumentID: did, Status: domain.StatusAwaitingInput}
	svc := newTestService(t, docRepo, newFakeStore(), urdRepo, &fakeRAG{})

	_, _, err := svc.Analyze(withActor(context.Background()), pid, did)

	require.Equal(t, errcode.URDAnalysisActive, businessCode(t, err))
	// Lỗi PHẢI kèm analysis_id: không có nó thì người dùng mất đường vào phân
	// tích đang dở và tài liệu kẹt vĩnh viễn (đo trên production 2026-09-16).
	var be *apperr.BusinessError
	require.ErrorAs(t, err, &be)
	details, ok := be.Details.(map[string]any)
	require.True(t, ok, "details phải là object để client đọc analysis_id")
	require.Equal(t, activeID, details["analysis_id"])
}

func TestAnalyze_ThanhCong_TaoPhanTich(t *testing.T) {
	pid, did, rid := uuid.New(), uuid.New(), uuid.New()
	docRepo := &fakeDocRepo{
		doc: documentdomain.Document{ID: did, ProjectID: pid, DocType: documentdomain.DocTypeURD, Title: "URD Demo"},
		revisions: []documentdomain.Revision{
			{ID: rid, Status: revisionStatusReady, CanonicalTextKey: "canon"},
		},
	}
	store := newFakeStore()
	store.objects["canon"] = []byte("Noi dung URD da trich xuat")
	rag := &fakeRAG{completeChatContent: `{"cases":[{"description":"Case A"},{"description":"Case B"}]}`}
	svc := newTestService(t, docRepo, store, newFakeUrdRepo(), rag)

	a, cases, err := svc.Analyze(withActor(context.Background()), pid, did)

	require.NoError(t, err)
	require.Equal(t, domain.StatusAwaitingInput, a.Status)
	require.Equal(t, 2, a.TotalCases)
	require.Len(t, cases, 2)
	require.Equal(t, "Case A", cases[0].Description)
}

// TestAnalyze_TraVeCoMocThoiGian canh lỗi đã đo ở local 2026-09-16: response
// của Analyze mang created_at/updated_at rỗng ("0001-01-01T00:00:00Z") vì
// usecase dựng struct rồi trả luôn, để DB tự điền giờ. Client đọc ra ngày năm
// 1, còn GET analyses sau đó lại ra giờ thật — hai nơi lệch nhau.
func TestAnalyze_TraVeCoMocThoiGian(t *testing.T) {
	pid, did, rid := uuid.New(), uuid.New(), uuid.New()
	docRepo := &fakeDocRepo{
		doc: documentdomain.Document{ID: did, ProjectID: pid, DocType: documentdomain.DocTypeURD, Title: "URD Demo"},
		revisions: []documentdomain.Revision{
			{ID: rid, Status: revisionStatusReady, CanonicalTextKey: "canon"},
		},
	}
	store := newFakeStore()
	store.objects["canon"] = []byte("Noi dung URD da trich xuat")
	rag := &fakeRAG{completeChatContent: `{"cases":[{"description":"Case A"}]}`}
	urdRepo := newFakeUrdRepo()
	svc := newTestService(t, docRepo, store, urdRepo, rag)
	truoc := time.Now().UTC().Add(-time.Second)

	a, _, err := svc.Analyze(withActor(context.Background()), pid, did)

	require.NoError(t, err)
	require.False(t, a.CreatedAt.IsZero(), "created_at không được rỗng")
	require.False(t, a.UpdatedAt.IsZero(), "updated_at không được rỗng")
	require.True(t, a.CreatedAt.After(truoc), "created_at phải là giờ hiện tại")
	// Giá trị trả cho client phải đúng bằng giá trị đưa xuống repository, để
	// bản ghi trong DB và response không lệch nhau.
	daLuu := urdRepo.analyses[a.ID]
	require.NotNil(t, daLuu, "phân tích phải được lưu xuống repository")
	require.Equal(t, daLuu.CreatedAt, a.CreatedAt)
	require.Equal(t, daLuu.UpdatedAt, a.UpdatedAt)
}

func TestSubmitResolutions_CaseKhongThuocPhanTich(t *testing.T) {
	pid, did := uuid.New(), uuid.New()
	analysisID, caseID := uuid.New(), uuid.New()
	urdRepo := newFakeUrdRepo()
	urdRepo.analyses[analysisID] = &domain.Analysis{ID: analysisID, DocumentID: did, TotalCases: 1}
	urdRepo.cases[analysisID] = []domain.EdgeCase{{ID: caseID, AnalysisID: analysisID, SequenceNo: 1}}
	docRepo := &fakeDocRepo{doc: documentdomain.Document{ID: did, ProjectID: pid}}
	svc := newTestService(t, docRepo, newFakeStore(), urdRepo, &fakeRAG{})

	_, _, err := svc.SubmitResolutions(withActor(context.Background()), pid, did, analysisID,
		[]ResolutionInput{{CaseID: uuid.New(), Resolution: "abc"}})

	var te *apperr.TechnicalError
	require.ErrorAs(t, err, &te)
}

func TestSubmitResolutions_ThieuHuongGiaiQuyet(t *testing.T) {
	pid, did := uuid.New(), uuid.New()
	analysisID, caseID := uuid.New(), uuid.New()
	urdRepo := newFakeUrdRepo()
	urdRepo.analyses[analysisID] = &domain.Analysis{ID: analysisID, DocumentID: did, TotalCases: 1}
	urdRepo.cases[analysisID] = []domain.EdgeCase{{ID: caseID, AnalysisID: analysisID, SequenceNo: 1}}
	docRepo := &fakeDocRepo{doc: documentdomain.Document{ID: did, ProjectID: pid}}
	svc := newTestService(t, docRepo, newFakeStore(), urdRepo, &fakeRAG{})

	_, _, err := svc.SubmitResolutions(withActor(context.Background()), pid, did, analysisID,
		[]ResolutionInput{{CaseID: caseID, Resolution: "  "}})

	var te *apperr.TechnicalError
	require.ErrorAs(t, err, &te)
}

func TestSubmitResolutions_HoanTat_TaoPhienBanURDMoi(t *testing.T) {
	pid, did, rid := uuid.New(), uuid.New(), uuid.New()
	analysisID, caseID := uuid.New(), uuid.New()

	docRepo := &fakeDocRepo{
		doc: documentdomain.Document{ID: did, ProjectID: pid, DocType: documentdomain.DocTypeURD},
		revisions: []documentdomain.Revision{
			{
				ID: rid, DocumentID: did, ProjectID: pid, Status: revisionStatusReady,
				FileName: "urd.docx", MediaType: testMediaTypeDOCX, ObjectKey: "orig-key",
				Scope: documentdomain.Scope{VersionID: uuidPtr(uuid.New())},
			},
		},
	}
	store := newFakeStore()
	store.objects["orig-key"] = minimalDocx(t)

	urdRepo := newFakeUrdRepo()
	urdRepo.analyses[analysisID] = &domain.Analysis{
		ID: analysisID, DocumentID: did, RevisionID: rid, Status: domain.StatusAwaitingInput, TotalCases: 1,
	}
	urdRepo.cases[analysisID] = []domain.EdgeCase{
		{ID: caseID, AnalysisID: analysisID, SequenceNo: 1, Description: "Nhap sai dinh dang ngay thang"},
	}

	svc := newTestService(t, docRepo, store, urdRepo, &fakeRAG{})

	result, daTaoPhienBanMoi, err := svc.SubmitResolutions(withActor(context.Background()), pid, did, analysisID,
		[]ResolutionInput{{CaseID: caseID, Resolution: "Validate dinh dang dd/mm/yyyy"}})

	require.NoError(t, err)
	require.True(t, daTaoPhienBanMoi, ".docx thì phải sinh phiên bản URD mới")
	require.Equal(t, domain.StatusCompleted, result.Status)
	require.Equal(t, 1, result.ResolvedCases)
	// finalizeAnalysis phải đã tạo 1 revision mới qua docSvc.CreateRevisionFromBytes.
	require.Len(t, docRepo.revisions, 2)
	newRevision := docRepo.revisions[1]
	merged := store.objects[newRevision.ObjectKey]
	require.NotEmpty(t, merged)
	zr, err := zip.NewReader(bytes.NewReader(merged), int64(len(merged)))
	require.NoError(t, err)
	f, err := zr.Open("word/document.xml")
	require.NoError(t, err)
	content, err := io.ReadAll(f)
	require.NoError(t, err)
	require.Contains(t, string(content), "Nhap sai dinh dang ngay thang")
	require.Contains(t, string(content), "Validate dinh dang dd/mm/yyyy")
}

// Case có hướng giải quyết dạng "không áp dụng" (IncludeInDocument=false) thì
// vẫn được lưu resolved bình thường nhưng KHÔNG xuất hiện trong phụ lục URD
// mới — chỉ case còn lại (IncludeInDocument mặc định true vì không gửi) mới
// được đưa vào tài liệu.
func TestSubmitResolutions_CaseKhongDuaVaoTaiLieu_BiLoaiKhoiPhuLuc(t *testing.T) {
	pid, did, rid := uuid.New(), uuid.New(), uuid.New()
	analysisID, caseDua, caseKhongDua := uuid.New(), uuid.New(), uuid.New()

	docRepo := &fakeDocRepo{
		doc: documentdomain.Document{ID: did, ProjectID: pid, DocType: documentdomain.DocTypeURD},
		revisions: []documentdomain.Revision{
			{
				ID: rid, DocumentID: did, ProjectID: pid, Status: revisionStatusReady,
				FileName: "urd.docx", MediaType: testMediaTypeDOCX, ObjectKey: "orig-key",
				Scope: documentdomain.Scope{VersionID: uuidPtr(uuid.New())},
			},
		},
	}
	store := newFakeStore()
	store.objects["orig-key"] = minimalDocx(t)

	urdRepo := newFakeUrdRepo()
	urdRepo.analyses[analysisID] = &domain.Analysis{
		ID: analysisID, DocumentID: did, RevisionID: rid, Status: domain.StatusAwaitingInput, TotalCases: 2,
	}
	urdRepo.cases[analysisID] = []domain.EdgeCase{
		{ID: caseDua, AnalysisID: analysisID, SequenceNo: 1, Description: "Nhap sai dinh dang ngay thang"},
		{ID: caseKhongDua, AnalysisID: analysisID, SequenceNo: 2, Description: "Dang nhap qua nhieu lan"},
	}

	svc := newTestService(t, docRepo, store, urdRepo, &fakeRAG{})
	khongDua := false

	result, daTaoPhienBanMoi, err := svc.SubmitResolutions(withActor(context.Background()), pid, did, analysisID,
		[]ResolutionInput{
			{CaseID: caseDua, Resolution: "Validate dinh dang dd/mm/yyyy"},
			{CaseID: caseKhongDua, Resolution: "Khong", IncludeInDocument: &khongDua},
		})

	require.NoError(t, err)
	require.True(t, daTaoPhienBanMoi, "còn case được chọn đưa vào tài liệu thì vẫn phải sinh phiên bản mới")
	require.Equal(t, domain.StatusCompleted, result.Status)
	require.Equal(t, 2, result.ResolvedCases, "cả 2 case đều được lưu resolved")

	require.Len(t, docRepo.revisions, 2)
	newRevision := docRepo.revisions[1]
	merged := store.objects[newRevision.ObjectKey]
	zr, err := zip.NewReader(bytes.NewReader(merged), int64(len(merged)))
	require.NoError(t, err)
	f, err := zr.Open("word/document.xml")
	require.NoError(t, err)
	content, err := io.ReadAll(f)
	require.NoError(t, err)
	require.Contains(t, string(content), "Nhap sai dinh dang ngay thang")
	require.NotContains(t, string(content), "Dang nhap qua nhieu lan",
		"case IncludeInDocument=false không được đưa vào phụ lục")
}

// Không cần FE gửi include_in_document: hướng giải quyết dạng "Không" khớp
// danh sách no_action_resolution.go thì TỰ ĐỘNG bị loại khỏi phụ lục, còn
// hướng giải quyết có nội dung thật thì tự động được đưa vào — chỉ dựa vào
// text người dùng đã gõ sẵn.
func TestSubmitResolutions_TuDongLoaiKhoiTaiLieu_KhongCanFEGuiCoTuongMinh(t *testing.T) {
	pid, did, rid := uuid.New(), uuid.New(), uuid.New()
	analysisID, caseDua, caseKhongDua := uuid.New(), uuid.New(), uuid.New()

	docRepo := &fakeDocRepo{
		doc: documentdomain.Document{ID: did, ProjectID: pid, DocType: documentdomain.DocTypeURD},
		revisions: []documentdomain.Revision{
			{
				ID: rid, DocumentID: did, ProjectID: pid, Status: revisionStatusReady,
				FileName: "urd.docx", MediaType: testMediaTypeDOCX, ObjectKey: "orig-key",
				Scope: documentdomain.Scope{VersionID: uuidPtr(uuid.New())},
			},
		},
	}
	store := newFakeStore()
	store.objects["orig-key"] = minimalDocx(t)

	urdRepo := newFakeUrdRepo()
	urdRepo.analyses[analysisID] = &domain.Analysis{
		ID: analysisID, DocumentID: did, RevisionID: rid, Status: domain.StatusAwaitingInput, TotalCases: 2,
	}
	urdRepo.cases[analysisID] = []domain.EdgeCase{
		{ID: caseDua, AnalysisID: analysisID, SequenceNo: 1, Description: "Nhap sai dinh dang ngay thang"},
		{ID: caseKhongDua, AnalysisID: analysisID, SequenceNo: 2, Description: "Dang nhap qua nhieu lan"},
	}

	svc := newTestService(t, docRepo, store, urdRepo, &fakeRAG{})

	// KHÔNG set IncludeInDocument ở cả 2 item — không phải FE nào cũng gửi.
	result, daTaoPhienBanMoi, err := svc.SubmitResolutions(withActor(context.Background()), pid, did, analysisID,
		[]ResolutionInput{
			{CaseID: caseDua, Resolution: "Validate dinh dang dd/mm/yyyy"},
			{CaseID: caseKhongDua, Resolution: "Không"},
		})

	require.NoError(t, err)
	require.True(t, daTaoPhienBanMoi)
	require.Equal(t, domain.StatusCompleted, result.Status)
	require.Equal(t, 2, result.ResolvedCases, "cả 2 case đều lưu resolved dù 1 case không vào tài liệu")

	newRevision := docRepo.revisions[1]
	merged := store.objects[newRevision.ObjectKey]
	zr, err := zip.NewReader(bytes.NewReader(merged), int64(len(merged)))
	require.NoError(t, err)
	f, err := zr.Open("word/document.xml")
	require.NoError(t, err)
	content, err := io.ReadAll(f)
	require.NoError(t, err)
	require.Contains(t, string(content), "Nhap sai dinh dang ngay thang")
	require.NotContains(t, string(content), "Dang nhap qua nhieu lan",
		"hướng giải quyết 'Không' phải tự động bị loại dù FE không gửi include_in_document")
}

// FE gửi include_in_document tường minh thì luôn thắng suy đoán tự động từ
// nội dung Resolution — kể cả khi suy đoán và giá trị gửi lên trái ngược nhau.
func TestSubmitResolutions_CoTuongMinhThangSuyDoanTuDong(t *testing.T) {
	pid, did, rid := uuid.New(), uuid.New(), uuid.New()
	analysisID, caseID := uuid.New(), uuid.New()

	docRepo := &fakeDocRepo{
		doc: documentdomain.Document{ID: did, ProjectID: pid, DocType: documentdomain.DocTypeURD},
		revisions: []documentdomain.Revision{
			{
				ID: rid, DocumentID: did, ProjectID: pid, Status: revisionStatusReady,
				FileName: "urd.docx", MediaType: testMediaTypeDOCX, ObjectKey: "orig-key",
				Scope: documentdomain.Scope{VersionID: uuidPtr(uuid.New())},
			},
		},
	}
	store := newFakeStore()
	store.objects["orig-key"] = minimalDocx(t)

	urdRepo := newFakeUrdRepo()
	urdRepo.analyses[analysisID] = &domain.Analysis{
		ID: analysisID, DocumentID: did, RevisionID: rid, Status: domain.StatusAwaitingInput, TotalCases: 1,
	}
	urdRepo.cases[analysisID] = []domain.EdgeCase{
		// "Không" tự suy đoán ra sẽ bị loại — nhưng FE ép include=true.
		{ID: caseID, AnalysisID: analysisID, SequenceNo: 1, Description: "Co tu dong gia han khong"},
	}

	svc := newTestService(t, docRepo, store, urdRepo, &fakeRAG{})
	epInclude := true

	result, daTaoPhienBanMoi, err := svc.SubmitResolutions(withActor(context.Background()), pid, did, analysisID,
		[]ResolutionInput{{CaseID: caseID, Resolution: "Không", IncludeInDocument: &epInclude}})

	require.NoError(t, err)
	require.True(t, daTaoPhienBanMoi, "gửi tường minh true thì phải thắng suy đoán tự động")
	require.Equal(t, domain.StatusCompleted, result.Status)

	newRevision := docRepo.revisions[1]
	merged := store.objects[newRevision.ObjectKey]
	zr, err := zip.NewReader(bytes.NewReader(merged), int64(len(merged)))
	require.NoError(t, err)
	f, err := zr.Open("word/document.xml")
	require.NoError(t, err)
	content, err := io.ReadAll(f)
	require.NoError(t, err)
	require.Contains(t, string(content), "Co tu dong gia han khong")
}

// Toàn bộ case đều đánh dấu "không đưa vào tài liệu" (vd không áp dụng) thì
// không có nội dung gì để bổ sung — dù tài liệu gốc là .docx cũng không sinh
// phiên bản mới, tránh tạo ra 1 file mới rỗng nội dung bổ sung.
func TestSubmitResolutions_ToanBoKhongDuaVaoTaiLieu_KhongTaoPhienBanMoi(t *testing.T) {
	pid, did, rid := uuid.New(), uuid.New(), uuid.New()
	analysisID, caseID := uuid.New(), uuid.New()

	docRepo := &fakeDocRepo{
		doc: documentdomain.Document{ID: did, ProjectID: pid, DocType: documentdomain.DocTypeURD},
		revisions: []documentdomain.Revision{
			{
				ID: rid, DocumentID: did, ProjectID: pid, Status: revisionStatusReady,
				FileName: "urd.docx", MediaType: testMediaTypeDOCX, ObjectKey: "orig-key",
				Scope: documentdomain.Scope{VersionID: uuidPtr(uuid.New())},
			},
		},
	}
	store := newFakeStore()
	store.objects["orig-key"] = minimalDocx(t)

	urdRepo := newFakeUrdRepo()
	urdRepo.analyses[analysisID] = &domain.Analysis{
		ID: analysisID, DocumentID: did, RevisionID: rid, Status: domain.StatusAwaitingInput, TotalCases: 1,
	}
	urdRepo.cases[analysisID] = []domain.EdgeCase{
		{ID: caseID, AnalysisID: analysisID, SequenceNo: 1, Description: "Dang nhap qua nhieu lan"},
	}

	svc := newTestService(t, docRepo, store, urdRepo, &fakeRAG{})
	khongDua := false

	result, daTaoPhienBanMoi, err := svc.SubmitResolutions(withActor(context.Background()), pid, did, analysisID,
		[]ResolutionInput{{CaseID: caseID, Resolution: "Khong", IncludeInDocument: &khongDua}})

	require.NoError(t, err)
	require.False(t, daTaoPhienBanMoi, "không còn case nào để đưa vào tài liệu thì không sinh phiên bản mới")
	require.Equal(t, domain.StatusCompleted, result.Status)
	require.Len(t, docRepo.revisions, 1, "không được tạo thêm revision nào")
}

// URD dạng PDF: vẫn nhập đủ hướng giải quyết và hoàn tất bình thường, chỉ KHÔNG
// sinh phiên bản mới. Trước bản sửa, docxmerge chạy trên file PDF rồi hỏng, trả
// SYS_500 và để phân tích kẹt awaiting_input với resolved==total — tài liệu khoá
// vĩnh viễn, đã tái lập trên production 2026-09-16 (mục #28).
func TestSubmitResolutions_KhongPhaiDocx_HoanTatMaKhongTaoPhienBanMoi(t *testing.T) {
	pid, did, rid := uuid.New(), uuid.New(), uuid.New()
	analysisID, caseID := uuid.New(), uuid.New()

	docRepo := &fakeDocRepo{
		doc: documentdomain.Document{ID: did, ProjectID: pid, DocType: documentdomain.DocTypeURD},
		revisions: []documentdomain.Revision{
			{
				ID: rid, DocumentID: did, ProjectID: pid, Status: revisionStatusReady,
				FileName: "urd.pdf", MediaType: "application/pdf", ObjectKey: "orig-key",
				Scope: documentdomain.Scope{VersionID: uuidPtr(uuid.New())},
			},
		},
	}
	store := newFakeStore()
	// Nội dung KHÔNG phải zip: docxmerge sẽ hỏng nếu bị gọi tới.
	store.objects["orig-key"] = []byte("%PDF-1.4 noi dung gia lap")

	urdRepo := newFakeUrdRepo()
	urdRepo.analyses[analysisID] = &domain.Analysis{
		ID: analysisID, DocumentID: did, RevisionID: rid, Status: domain.StatusAwaitingInput, TotalCases: 1,
	}
	urdRepo.cases[analysisID] = []domain.EdgeCase{
		{ID: caseID, AnalysisID: analysisID, SequenceNo: 1, Description: "Chua neu cach xu ly khi het cho"},
	}

	svc := newTestService(t, docRepo, store, urdRepo, &fakeRAG{})

	result, daTaoPhienBanMoi, err := svc.SubmitResolutions(withActor(context.Background()), pid, did, analysisID,
		[]ResolutionInput{{CaseID: caseID, Resolution: "Bao het cho va goi y khung gio khac"}})

	require.NoError(t, err, "PDF không được làm hỏng cả lời gọi")
	require.False(t, daTaoPhienBanMoi, "PDF thì không sinh phiên bản URD mới")
	require.Equal(t, domain.StatusCompleted, result.Status, "phân tích vẫn phải hoàn tất, không kẹt awaiting_input")
	require.Equal(t, 1, result.ResolvedCases)
	require.Len(t, docRepo.revisions, 1, "không được tạo thêm revision nào")
}

// Chưa nhập đủ hướng giải quyết thì chưa chạy tới bước tạo phiên bản mới.
func TestSubmitResolutions_ChuaDu_ThiChuaTaoPhienBanMoi(t *testing.T) {
	pid, did, rid := uuid.New(), uuid.New(), uuid.New()
	analysisID, case1, case2 := uuid.New(), uuid.New(), uuid.New()

	docRepo := &fakeDocRepo{doc: documentdomain.Document{ID: did, ProjectID: pid}}
	urdRepo := newFakeUrdRepo()
	urdRepo.analyses[analysisID] = &domain.Analysis{
		ID: analysisID, DocumentID: did, RevisionID: rid, Status: domain.StatusAwaitingInput, TotalCases: 2,
	}
	urdRepo.cases[analysisID] = []domain.EdgeCase{
		{ID: case1, AnalysisID: analysisID, SequenceNo: 1},
		{ID: case2, AnalysisID: analysisID, SequenceNo: 2},
	}
	svc := newTestService(t, docRepo, newFakeStore(), urdRepo, &fakeRAG{})

	result, daTaoPhienBanMoi, err := svc.SubmitResolutions(withActor(context.Background()), pid, did, analysisID,
		[]ResolutionInput{{CaseID: case1, Resolution: "Xu ly A"}})

	require.NoError(t, err)
	require.False(t, daTaoPhienBanMoi)
	require.Equal(t, domain.StatusAwaitingInput, result.Status)
}

func uuidPtr(id uuid.UUID) *uuid.UUID { return &id }

// structuredDocx dựng .docx có tiêu đề mục (style Heading) để kiểm tra chèn
// nội dung THẲNG vào mục thay vì gom vào phụ lục.
func structuredDocx(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("word/document.xml")
	require.NoError(t, err)
	_, err = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>
<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Chuc nang Dang nhap</w:t></w:r></w:p>
<w:p><w:pPr><w:pStyle w:val="Heading2"/></w:pPr><w:r><w:t>Tieu chi chap nhan</w:t></w:r></w:p>
<w:p><w:r><w:t>AC-01 Dang nhap dung thi vao trang chu</w:t></w:r></w:p>
<w:p><w:pPr><w:pStyle w:val="Heading2"/></w:pPr><w:r><w:t>Quy tac nghiep vu</w:t></w:r></w:p>
<w:p><w:r><w:t>BR-01 Mat khau toi thieu 8 ky tu</w:t></w:r></w:p>
<w:sectPr><w:pgSz w:w="12240" w:h="15840"/></w:sectPr></w:body></w:document>`))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// Chuỗi đầy đủ, KHÔNG cần FE gửi thêm gì: AI trả kèm target_heading khi phân
// tích → lưu xuống case → lúc tạo phiên bản mới, nội dung được chèn thẳng
// vào đúng mục AC, không sinh phụ lục.
func TestAnalyzeRoiSubmit_ChenThangVaoMucAC_KhongCanFE(t *testing.T) {
	pid, did, rid := uuid.New(), uuid.New(), uuid.New()
	docRepo := &fakeDocRepo{
		doc: documentdomain.Document{
			ID: did, ProjectID: pid, DocType: documentdomain.DocTypeURD, Title: "URD Dang nhap",
		},
		revisions: []documentdomain.Revision{
			{
				ID: rid, DocumentID: did, ProjectID: pid, Status: revisionStatusReady,
				FileName: "urd.docx", MediaType: testMediaTypeDOCX, ObjectKey: "orig-key",
				CanonicalTextKey: "canon", Scope: documentdomain.Scope{VersionID: uuidPtr(uuid.New())},
			},
		},
	}
	store := newFakeStore()
	store.objects["orig-key"] = structuredDocx(t)
	store.objects["canon"] = []byte("# Chuc nang Dang nhap\n## Tieu chi chap nhan\nAC-01 ...")
	rag := &fakeRAG{completeChatContent: `{"cases":[
		{"description":"Dang nhap sai qua 5 lan thi sao?","target_heading":"Tieu chi chap nhan"}
	]}`}
	urdRepo := newFakeUrdRepo()
	svc := newTestService(t, docRepo, store, urdRepo, rag)

	a, cases, err := svc.Analyze(withActor(context.Background()), pid, did)
	require.NoError(t, err)
	require.Len(t, cases, 1)
	require.Equal(t, "Tieu chi chap nhan", cases[0].TargetHeading, "target_heading phải được lưu lại")

	// FE gửi ĐÚNG payload như cũ: chỉ case_id + resolution.
	_, daTaoPhienBanMoi, err := svc.SubmitResolutions(withActor(context.Background()), pid, did, a.ID,
		[]ResolutionInput{{CaseID: cases[0].ID, Resolution: "Khoa tai khoan 15 phut"}})

	require.NoError(t, err)
	require.True(t, daTaoPhienBanMoi)
	require.Len(t, docRepo.revisions, 2)
	merged := store.objects[docRepo.revisions[1].ObjectKey]
	zr, err := zip.NewReader(bytes.NewReader(merged), int64(len(merged)))
	require.NoError(t, err)
	f, err := zr.Open("word/document.xml")
	require.NoError(t, err)
	raw, err := io.ReadAll(f)
	require.NoError(t, err)
	content := string(raw)

	require.NotContains(t, content, "Phụ lục", "phải chèn thẳng vào mục, không tạo phụ lục")
	require.Greater(t, strings.Index(content, "Khoa tai khoan 15 phut"),
		strings.Index(content, "AC-01 Dang nhap dung thi vao trang chu"))
	require.Less(t, strings.Index(content, "Khoa tai khoan 15 phut"),
		strings.Index(content, "Quy tac nghiep vu"))
}
