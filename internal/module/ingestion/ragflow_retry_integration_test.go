//go:build integration

package ingestion

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/internal/common/port"
)

// ragStub trả lỗi định sẵn ở GetDocument — đúng chỗ đã hỏng thật trên
// production (worker poll trạng thái rồi timeout một nhịp).
type ragStub struct {
	getDocumentErr error
	datasetID      string
	// onGetDocument chạy ngay trước khi GetDocument trả lỗi. Dùng để giả lập
	// SIGTERM ập tới ĐÚNG LÚC đang chờ RAGFlow — huỷ ctx sớm hơn thì claim đã
	// hỏng trước và job không bao giờ được nhặt, tức là không tái hiện đúng cảnh.
	onGetDocument func()
	// deletedDataset/deletedIDs ghi lại lời gọi DeleteDocuments. Nhánh cleanup
	// chỉ kiểm chứng được bằng cách hỏi "RAGFlow CÓ được yêu cầu xoá không";
	// nhìn trạng thái trong DB là chưa đủ, vì lỗi cũ đánh 'succeeded' mà không xoá.
	deletedDataset string
	deletedIDs     []string
}

func (*ragStub) Health(context.Context) error { return nil }
func (s *ragStub) CreateDataset(context.Context, string, string) (port.RAGDataset, error) {
	return port.RAGDataset{ID: s.datasetID}, nil
}
func (s *ragStub) FindDatasetByName(context.Context, string) (*port.RAGDataset, error) {
	return &port.RAGDataset{ID: s.datasetID}, nil
}
func (*ragStub) UpdateDataset(context.Context, string, string, string) error { return nil }
func (*ragStub) DeleteDatasets(context.Context, []string) error              { return nil }
func (s *ragStub) DeleteDocuments(_ context.Context, datasetID string, ids []string) error {
	s.deletedDataset = datasetID
	s.deletedIDs = append(s.deletedIDs, ids...)
	return nil
}
func (*ragStub) StartParsing(context.Context, string, []string) error { return nil }
func (*ragStub) StopParsing(context.Context, string, []string) error  { return nil }
func (*ragStub) UpdateDocumentMetadata(context.Context, string, []string, map[string]string) error {
	return nil
}
func (s *ragStub) UploadDocument(context.Context, string, port.RAGDocumentFile) (port.RAGDocument, error) {
	return port.RAGDocument{ID: "remote-" + s.datasetID}, nil
}
func (s *ragStub) GetDocument(context.Context, string, string) (port.RAGDocument, error) {
	if s.onGetDocument != nil {
		s.onGetDocument()
	}
	return port.RAGDocument{}, s.getDocumentErr
}
func (*ragStub) FindDocumentByName(context.Context, string, string) (*port.RAGDocument, error) {
	return nil, nil
}
func (*ragStub) Retrieve(context.Context, port.RAGRetrievalRequest) (port.RAGRetrievalResult, error) {
	return port.RAGRetrievalResult{}, nil
}
func (*ragStub) CreateChat(context.Context, string, []string) (port.RAGChat, error) {
	return port.RAGChat{}, nil
}
func (*ragStub) FindChatByName(context.Context, string) (*port.RAGChat, error) { return nil, nil }
func (*ragStub) UpdateChatDatasets(context.Context, string, []string) error    { return nil }
func (*ragStub) CompleteChat(context.Context, port.RAGChatCompletionRequest) (port.RAGChatCompletionResult, error) {
	return port.RAGChatCompletionResult{}, nil
}

type jobState struct {
	Status      string
	Attempt     int
	AvailableAt time.Time `gorm:"column:available_at"`
}

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("cần TEST_POSTGRES_DSN để chạy test này")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	return db
}

// seedJob dựng đủ dữ liệu cho MỘT job và trả về (jobID, revisionID).
// Dùng UUID ngẫu nhiên và chỉ xoá đúng bản ghi của mình — cố ý KHÔNG truncate,
// để nếu ai lỡ trỏ DSN vào database thật thì test hỏng ồn ào chứ không xoá dữ
// liệu của họ. Tham số attempt là số lượt ĐÃ dùng trước lượt này; claim sẽ +1.
func seedJob(t *testing.T, db *gorm.DB, attempt int) (string, string) {
	t.Helper()
	// Khớp DEFAULT của cột max_attempts trong migration 000007.
	const maxAttempts = 3
	userID, projectID := uuid.New(), uuid.New()
	versionID, documentID := uuid.New(), uuid.New()
	revisionID, jobID := uuid.New(), uuid.New()

	require.NoError(t, db.Exec(`INSERT INTO users(id,email,full_name,password_hash) VALUES(?,?,?,?)`,
		userID, userID.String()+"@test.local", "Retry", "hash").Error)
	require.NoError(t, db.Exec(`INSERT INTO projects(id,code,name,owner_id,ragflow_dataset_id)
		VALUES(?,?,?,?,?)`, projectID, "rt-"+projectID.String(), "Retry", userID, "ds-fixture").Error)
	require.NoError(t, db.Exec(`INSERT INTO project_versions(id,project_id,label,sequence_no,status,created_by)
		VALUES(?,?,?,1,'draft',?)`, versionID, projectID, "v1", userID).Error)
	require.NoError(t, db.Exec(`INSERT INTO documents(id,project_id,title,document_key,created_by)
		VALUES(?,?,?,?,?)`, documentID, projectID, "Retry", documentID.String(), userID).Error)
	require.NoError(t, db.Exec(`INSERT INTO document_revisions(
		id,document_id,project_id,project_version_id,revision_no,file_name,media_type,size_bytes,
		sha256,object_key,status,ragflow_document_id,created_by)
		VALUES(?,?,?,?,1,'a.txt','text/plain',10,repeat('b',64),?,'processing','remote-ds',?)`,
		revisionID, documentID, projectID, versionID, "k/"+revisionID.String(), userID).Error)
	require.NoError(t, db.Exec(`INSERT INTO ingestion_jobs(id,document_revision_id,status,attempt,max_attempts)
		VALUES(?,?,'pending',?,?)`, jobID, revisionID, attempt, maxAttempts).Error)

	t.Cleanup(func() {
		db.Exec("DELETE FROM ingestion_jobs WHERE id=?", jobID)
		db.Exec("DELETE FROM document_revisions WHERE id=?", revisionID)
		db.Exec("DELETE FROM documents WHERE id=?", documentID)
		db.Exec("DELETE FROM project_versions WHERE id=?", versionID)
		db.Exec("DELETE FROM projects WHERE id=?", projectID)
		db.Exec("DELETE FROM users WHERE id=?", userID)
	})
	return jobID.String(), revisionID.String()
}

func runOnce(ctx context.Context, t *testing.T, db *gorm.DB, cause error, onGetDocument func()) {
	t.Helper()
	p := NewRAGFlowProcessor(db, &integrationStore{objects: map[string][]byte{}},
		&ragStub{getDocumentErr: cause, datasetID: "ds-fixture", onGetDocument: onGetDocument},
		RAGFlowProcessorConfig{PollInterval: time.Millisecond, MaxPollDuration: time.Second})
	_, _ = p.ProcessNext(ctx)
}

func readJob(t *testing.T, db *gorm.DB, jobID string) jobState {
	t.Helper()
	var state jobState
	require.NoError(t, db.Table("ingestion_jobs").Select("status,attempt,available_at").
		Where("id=?", jobID).Scan(&state).Error)
	return state
}

func readRevisionStatus(t *testing.T, db *gorm.DB, revisionID string) string {
	t.Helper()
	var status string
	require.NoError(t, db.Table("document_revisions").Select("status").
		Where("id=?", revisionID).Scan(&status).Error)
	return status
}

// Đây là sự cố gốc: timeout một nhịp KHÔNG được giết tài liệu.
func TestRAGFlowRetry_LoiTamThoiThiXepLaiHangDoi(t *testing.T) {
	db := openTestDB(t)
	jobID, revisionID := seedJob(t, db, 0)

	before := time.Now().UTC()
	runOnce(context.Background(), t, db, context.DeadlineExceeded, nil)

	state := readJob(t, db, jobID)
	require.Equal(t, "pending", state.Status, "job phải quay về hàng đợi, không được 'failed'")
	require.Equal(t, 1, state.Attempt, "claim phải đã tăng attempt")
	require.True(t, state.AvailableAt.After(before), "phải lùi available_at để backoff")
	require.Equal(t, "processing", readRevisionStatus(t, db, revisionID),
		"người dùng không nên thấy tài liệu báo hỏng chỉ vì một lượt trượt")
}

// DOCX hỏng thì thử lại vô ích — phải dừng ngay từ lượt đầu.
func TestRAGFlowRetry_LoiVinhVienThiHongNgay(t *testing.T) {
	db := openTestDB(t)
	jobID, revisionID := seedJob(t, db, 0)

	runOnce(context.Background(), t, db,
		permanent(errors.New("RAGFlow parse thất bại: thiếu word/media/image22.png")), nil)

	require.Equal(t, "failed", readJob(t, db, jobID).Status)
	require.Equal(t, "failed", readRevisionStatus(t, db, revisionID))
}

// Hết lượt thì lỗi tạm thời cũng phải dừng, không thử lại vô tận.
func TestRAGFlowRetry_HetLuotThiHong(t *testing.T) {
	db := openTestDB(t)
	jobID, revisionID := seedJob(t, db, 2) // claim +1 -> attempt=3 = max

	runOnce(context.Background(), t, db, context.DeadlineExceeded, nil)

	require.Equal(t, "failed", readJob(t, db, jobID).Status)
	require.Equal(t, "failed", readRevisionStatus(t, db, revisionID))
}

// Ca deploy: worker nhận SIGTERM giữa lúc chờ RAGFlow nên ctx công việc bị huỷ.
// Trước bản sửa, fail() ghi DB bằng chính ctx đó -> GORM từ chối, lỗi bị nuốt,
// job kẹt vĩnh viễn ở 'running' vì claim chỉ nhặt 'pending'.
func TestRAGFlowRetry_CtxBiHuyVanGhiDuocTrangThai(t *testing.T) {
	db := openTestDB(t)
	jobID, _ := seedJob(t, db, 0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runOnce(ctx, t, db, context.Canceled, cancel)

	state := readJob(t, db, jobID)
	require.NotEqual(t, "running", state.Status, "job không được kẹt ở 'running'")
	require.Equal(t, "pending", state.Status, "deploy giữa chừng thì phải xếp lại hàng đợi")
}
