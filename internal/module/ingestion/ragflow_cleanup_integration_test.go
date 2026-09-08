//go:build integration

package ingestion

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/internal/infrastructure/ai/ragflow"
)

type cleanupFixture struct {
	EventID    string
	DocumentID string
	RevisionID string
	RemoteID   string
	DatasetID  string
}

// seedCleanup dựng đúng cảnh sau khi người dùng bấm xoá tài liệu: document đã
// xoá mềm, revision còn giữ ragflow_document_id, và một outbox event
// 'document.cleanup' đang chờ ở 'pending'.
//
// Cũng như seedJob: dùng UUID ngẫu nhiên, chỉ dọn đúng bản ghi của mình, cố ý
// KHÔNG truncate để nếu ai lỡ trỏ DSN vào database thật thì test hỏng ồn ào.
func seedCleanup(t *testing.T, db *gorm.DB) cleanupFixture {
	t.Helper()
	userID, projectID := uuid.New(), uuid.New()
	versionID, documentID := uuid.New(), uuid.New()
	revisionID, eventID := uuid.New(), uuid.New()
	const datasetID = "ds-fixture"
	remoteID := "remote-" + revisionID.String()

	require.NoError(t, db.Exec(`INSERT INTO users(id,email,full_name,password_hash) VALUES(?,?,?,?)`,
		userID, userID.String()+"@test.local", "Cleanup", "hash").Error)
	require.NoError(t, db.Exec(`INSERT INTO projects(id,code,name,owner_id,ragflow_dataset_id)
		VALUES(?,?,?,?,?)`, projectID, "cl-"+projectID.String(), "Cleanup", userID, datasetID).Error)
	require.NoError(t, db.Exec(`INSERT INTO project_versions(id,project_id,label,sequence_no,status,created_by)
		VALUES(?,?,?,1,'draft',?)`, versionID, projectID, "v1", userID).Error)
	// deleted_at khác NULL: tài liệu đã bị xoá mềm, đúng như SoftDelete để lại.
	require.NoError(t, db.Exec(`INSERT INTO documents(id,project_id,title,document_key,created_by,deleted_at)
		VALUES(?,?,?,?,?,now())`, documentID, projectID, "Cleanup", documentID.String(), userID).Error)
	require.NoError(t, db.Exec(`INSERT INTO document_revisions(
		id,document_id,project_id,project_version_id,revision_no,file_name,media_type,size_bytes,
		sha256,object_key,status,ragflow_document_id,ragflow_sync_status,created_by)
		VALUES(?,?,?,?,1,'a.txt','text/plain',10,repeat('c',64),?,'ready',?,'ready',?)`,
		revisionID, documentID, projectID, versionID, "k/"+revisionID.String(), remoteID, userID).Error)
	require.NoError(t, db.Exec(`INSERT INTO outbox_events(id,topic,aggregate_type,aggregate_id,payload,status)
		VALUES(?,'document.cleanup','document',?,'{}','pending')`, eventID, documentID).Error)

	t.Cleanup(func() {
		db.Exec("DELETE FROM outbox_events WHERE id=?", eventID)
		db.Exec("DELETE FROM document_revisions WHERE id=?", revisionID)
		db.Exec("DELETE FROM documents WHERE id=?", documentID)
		db.Exec("DELETE FROM project_versions WHERE id=?", versionID)
		db.Exec("DELETE FROM projects WHERE id=?", projectID)
		db.Exec("DELETE FROM users WHERE id=?", userID)
	})
	return cleanupFixture{
		EventID: eventID.String(), DocumentID: documentID.String(),
		RevisionID: revisionID.String(), RemoteID: remoteID, DatasetID: datasetID,
	}
}

func runCleanupOnce(t *testing.T, db *gorm.DB) *ragStub {
	t.Helper()
	stub := &ragStub{datasetID: "ds-fixture"}
	p := NewRAGFlowProcessor(db, &integrationStore{objects: map[string][]byte{}}, stub,
		RAGFlowProcessorConfig{PollInterval: time.Millisecond, MaxPollDuration: time.Second})
	_, _ = p.processCleanup(context.Background())
	return stub
}

func readEventStatus(t *testing.T, db *gorm.DB, eventID string) string {
	t.Helper()
	var status string
	require.NoError(t, db.Table("outbox_events").Select("status").
		Where("id=?", eventID).Scan(&status).Error)
	return status
}

func readSyncStatus(t *testing.T, db *gorm.DB, revisionID string) string {
	t.Helper()
	var status string
	require.NoError(t, db.Table("document_revisions").Select("ragflow_sync_status").
		Where("id=?", revisionID).Scan(&status).Error)
	return status
}

// Đây là bug đã tái lập được trên production: app trả 204 nhưng tài liệu ở lại
// RAGFlow mãi mãi. Gốc là câu Scan mapping ghi đè cả struct, xoá trắng EventID
// và DocumentID, khiến mọi lệnh sau đó so cột uuid với chuỗi rỗng.
func TestRAGFlowCleanup_XoaThatSuBenRAGFlow(t *testing.T) {
	db := openTestDB(t)
	f := seedCleanup(t, db)

	stub := runCleanupOnce(t, db)

	require.Equal(t, []string{f.RemoteID}, stub.deletedIDs,
		"phải gọi RAGFlow xoá đúng tài liệu — đây là điều đã KHÔNG xảy ra trên production")
	require.Equal(t, f.DatasetID, stub.deletedDataset, "phải xoá trong đúng dataset")
	require.Equal(t, "succeeded", readEventStatus(t, db, f.EventID),
		"event không được kẹt ở 'processing'")
	require.Equal(t, "deleted", readSyncStatus(t, db, f.RevisionID))
}

// Mục #7, nửa BÌNH THƯỜNG của nhánh bỏ qua: tài liệu chưa từng lên tới RAGFlow
// (revision không có ragflow_document_id) thì không có gì để xoá — đánh
// 'succeeded' là đúng, và không được gọi RAGFlow.
func TestRAGFlowCleanup_ChuaLenRAGFlowThiCoiNhuXongLuon(t *testing.T) {
	db := openTestDB(t)
	f := seedCleanup(t, db)
	require.NoError(t, db.Exec(`UPDATE document_revisions SET ragflow_document_id=NULL WHERE id=?`,
		f.RevisionID).Error)

	stub := runCleanupOnce(t, db)

	require.Empty(t, stub.deletedIDs, "không có gì trên RAGFlow thì đừng gọi sang đó")
	require.Equal(t, "succeeded", readEventStatus(t, db, f.EventID))
}

// Mục #7, nửa BẤT THƯỜNG: revision vẫn giữ ragflow_document_id — tức tài liệu
// ĐANG nằm trên RAGFlow — nhưng project mất ragflow_dataset_id nên không có
// đường mà xoá.
//
// Bản cũ gộp hai nửa vào một điều kiện rồi luôn đánh 'succeeded', nên trường hợp
// này im lặng bỏ tài liệu ở lại RAGFlow: app báo xoá xong, dữ liệu vẫn còn.
func TestRAGFlowCleanup_MatDatasetIDThiKhongDuocBaoXong(t *testing.T) {
	db := openTestDB(t)
	f := seedCleanup(t, db)
	require.NoError(t, db.Exec(`UPDATE projects SET ragflow_dataset_id=NULL
		WHERE id=(SELECT project_id FROM documents WHERE id=?)`, f.DocumentID).Error)

	stub := &ragStub{datasetID: "ds-fixture"}
	p := NewRAGFlowProcessor(db, &integrationStore{objects: map[string][]byte{}}, stub,
		RAGFlowProcessorConfig{PollInterval: time.Millisecond, MaxPollDuration: time.Second})

	done, err := p.processCleanup(context.Background())

	require.True(t, done)
	require.Error(t, err, "phải kêu lên để worker ghi log, không được im lặng")
	require.Empty(t, stub.deletedIDs)
	// 'failed' chứ không phải 'pending': dataset id không tự mọc lại nên thử lại
	// 15 lượt trong 38 phút chỉ tốn công rồi cũng vào đúng chỗ này.
	require.Equal(t, "failed", readEventStatus(t, db, f.EventID))
	// Và tuyệt đối không được đánh revision là đã 'deleted' — nó vẫn còn trên RAGFlow.
	require.Equal(t, "ready", readSyncStatus(t, db, f.RevisionID))
}

// Chốt chặn cho phạm vi của bản sửa #7: lỗi TỪ RAGFlow vẫn phải được thử lại
// như trước, dù nó mang cờ Retryable=false.
//
// Nhánh cleanup hỏng thì event nằm lại 'failed' mà không có API nào đưa về
// 'pending' — phải vào tận DB production. Nên ở đây thà thử lại thừa còn hơn
// đánh hỏng nhầm; chỉ lỗi do chính code này gắn dấu permanent() mới dừng ngay.
func TestRAGFlowCleanup_LoiTuRAGFlowVanDuocThuLai(t *testing.T) {
	db := openTestDB(t)
	f := seedCleanup(t, db)

	stub := &ragStub{
		datasetID: "ds-fixture",
		// Đúng dạng lỗi RAGFlow hay trả: HTTP 200 kèm code khác 0, mà
		// decodeHTTPResponse gán cờ theo status nên thành Retryable=false.
		deleteErr: &ragflow.APIError{HTTPStatus: 200, Code: 102, Retryable: false},
	}
	p := NewRAGFlowProcessor(db, &integrationStore{objects: map[string][]byte{}}, stub,
		RAGFlowProcessorConfig{PollInterval: time.Millisecond, MaxPollDuration: time.Second})

	done, err := p.processCleanup(context.Background())

	require.True(t, done)
	require.Error(t, err)
	require.Equal(t, "pending", readEventStatus(t, db, f.EventID),
		"còn ngân sách thì phải xếp lại hàng đợi, không được đánh 'failed'")
	require.Equal(t, "ready", readSyncStatus(t, db, f.RevisionID),
		"xoá chưa xong thì đừng đánh dấu revision là đã dọn")
}

// Không còn event nào thì processCleanup phải im lặng nhường chỗ cho ingest,
// chứ không được coi là đã làm việc.
func TestRAGFlowCleanup_KhongCoViecThiKhongLamGi(t *testing.T) {
	db := openTestDB(t)
	stub := &ragStub{datasetID: "ds-fixture"}
	p := NewRAGFlowProcessor(db, &integrationStore{objects: map[string][]byte{}}, stub,
		RAGFlowProcessorConfig{PollInterval: time.Millisecond, MaxPollDuration: time.Second})

	done, err := p.processCleanup(context.Background())

	require.NoError(t, err)
	require.False(t, done)
	require.Empty(t, stub.deletedIDs)
}
