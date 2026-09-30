//go:build integration

// Integration test cho chốt chặn unarchivedRevision. Phải chạy trên Postgres
// thật: thứ đang kiểm là mệnh đề WHERE có lọc đúng hay không — với database
// giả thì câu nào cũng "chạy xong" và test luôn xanh.
package ingestion

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// dungRevision dựng một revision thật mang trạng thái cho trước.
func dungRevision(t *testing.T, db *gorm.DB, trangThai string) uuid.UUID {
	t.Helper()
	userID, projectID := uuid.New(), uuid.New()
	versionID, documentID, revisionID := uuid.New(), uuid.New(), uuid.New()

	require.NoError(t, db.Exec(`INSERT INTO users(id,email,full_name,password_hash) VALUES(?,?,?,?)`,
		userID, userID.String()+"@test.local", "Guard", "hash").Error)
	require.NoError(t, db.Exec(`INSERT INTO projects(id,code,name,owner_id) VALUES(?,?,?,?)`,
		projectID, "gd-"+projectID.String(), "Guard", userID).Error)
	require.NoError(t, db.Exec(`INSERT INTO project_versions(id,project_id,label,sequence_no,status,created_by)
		VALUES(?,?,?,1,'draft',?)`, versionID, projectID, "v1", userID).Error)
	require.NoError(t, db.Exec(`INSERT INTO documents(id,project_id,title,document_key,created_by)
		VALUES(?,?,?,?,?)`, documentID, projectID, "Guard", documentID.String(), userID).Error)
	require.NoError(t, db.Exec(`INSERT INTO document_revisions(
		id,document_id,project_id,project_version_id,revision_no,file_name,media_type,size_bytes,
		sha256,object_key,status,created_by)
		VALUES(?,?,?,?,1,'a.txt','text/plain',10,repeat('c',64),?,?,?)`,
		revisionID, documentID, projectID, versionID, "k/"+revisionID.String(), trangThai, userID).Error)
	return revisionID
}

// Đây là cuộc đua thật: người dùng thấy tài liệu nạp lâu nên bấm xoá, lát sau
// worker nạp xong và ghi 'ready' đè lên nhãn lưu trữ. Nhãn mất thì sha256 bị
// khoá lại và không ai hiểu vì sao.
func TestUnarchivedRevision_KhongGhiDeNhanLuuTru(t *testing.T) {
	db := openTestDB(t)
	revisionID := dungRevision(t, db, revisionStatusArchived)

	err := unarchivedRevision(db, revisionID.String()).
		Updates(map[string]any{"status": "ready"}).Error
	require.NoError(t, err, "câu lệnh vẫn phải chạy trót lọt, chỉ là không chạm dòng nào")

	var trangThai string
	require.NoError(t, db.Raw(`SELECT status FROM document_revisions WHERE id=?`,
		revisionID).Scan(&trangThai).Error)
	require.Equal(t, revisionStatusArchived, trangThai,
		"revision đã lưu trữ không được đổi trạng thái nữa")
}

// Chốt chặn không được làm hỏng đường đi bình thường: tài liệu còn sống thì
// worker vẫn phải cập nhật được như cũ.
func TestUnarchivedRevision_VanGhiDuocKhiChuaLuuTru(t *testing.T) {
	db := openTestDB(t)
	revisionID := dungRevision(t, db, "processing")

	require.NoError(t, unarchivedRevision(db, revisionID.String()).
		Updates(map[string]any{"status": "ready"}).Error)

	var trangThai string
	require.NoError(t, db.Raw(`SELECT status FROM document_revisions WHERE id=?`,
		revisionID).Scan(&trangThai).Error)
	require.Equal(t, "ready", trangThai)
}
