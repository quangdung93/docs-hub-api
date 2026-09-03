//go:build integration

package ingestion

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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
