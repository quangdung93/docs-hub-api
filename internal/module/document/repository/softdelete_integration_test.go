//go:build integration

// Integration test cho lớp lỗi "GORM không áp soft-delete khi truy vấn không đi
// qua model có gorm.DeletedAt". Chạy: make test-integration (cần Docker).
package repository_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	"github.com/quangdung93/docs-hub-api/internal/module/document/domain"
	"github.com/quangdung93/docs-hub-api/internal/module/document/repository"
)

// dungDuLieu tạo một project + document + revision thật trong schema đã migrate,
// trả về id của từng thứ.
func dungDuLieu(t *testing.T, db *gorm.DB) (projectID, documentID, revisionID, actorID uuid.UUID) {
	t.Helper()
	projectID, documentID, revisionID, actorID = uuid.New(), uuid.New(), uuid.New(), uuid.New()

	require.NoError(t, db.Exec(`INSERT INTO users (id,email,password_hash,full_name)
		VALUES (?,?,?,?)`,
		actorID, "it-"+actorID.String()+"@test.local", "x", "IT Actor").Error)
	require.NoError(t, db.Exec(`INSERT INTO projects (id,owner_id,name,code)
		VALUES (?,?,?,?)`, projectID, actorID, "IT Project",
		"IT-"+projectID.String()[:8]).Error)
	require.NoError(t, db.Exec(`INSERT INTO documents (id,project_id,title,document_key,created_by)
		VALUES (?,?,?,?,?)`, documentID, projectID, "Tài liệu IT",
		"key-"+documentID.String(), actorID).Error)
	// document_revisions có CHECK bắt buộc ĐÚNG MỘT trong project_version_id /
	// change_request_id khác NULL, nên phải dựng sẵn một version để trỏ vào.
	versionID := uuid.New()
	require.NoError(t, db.Exec(`INSERT INTO project_versions
		(id,project_id,label,sequence_no,status,created_by)
		VALUES (?,?,'v1.0',1,'draft',?)`, versionID, projectID, actorID).Error)

	// sha256 là CHAR(64) — phải đủ 64 ký tự, không thì Postgres đệm rồi so lệch.
	sha := strings.Repeat("a", 64-len(revisionID.String())) + revisionID.String()
	require.NoError(t, db.Exec(`INSERT INTO document_revisions
		(id,document_id,project_id,project_version_id,revision_no,file_name,
		 media_type,size_bytes,sha256,object_key,status,created_by)
		VALUES (?,?,?,?,1,'a.txt','text/plain',10,?,?,'failed',?)`,
		revisionID, documentID, projectID, versionID,
		sha, "obj-"+revisionID.String(), actorID).Error)
	return projectID, documentID, revisionID, actorID
}

// Sau khi tài liệu bị xoá mềm, revision của nó KHÔNG được đọc nữa.
//
// document_revisions không có cột deleted_at, nên nếu truy vấn thẳng theo id thì
// vẫn trả về đủ dữ liệu — đúng lỗi đã báo ở mục #6.
func TestFindRevision_KhongDocDuocSauKhiXoaTaiLieu(t *testing.T) {
	db := openTestDB(t)
	repo := repository.New(db)
	ctx := context.Background()
	pid, did, rid, actor := dungDuLieu(t, db)

	// Trước khi xoá: đọc được bình thường.
	rev, err := repo.FindRevision(ctx, pid, did, rid)
	require.NoError(t, err)
	require.Equal(t, rid.String(), rev.ID.String())

	require.NoError(t, repo.SoftDelete(ctx, pid, did, actor))

	// Hàng revision vẫn còn nguyên trong bảng — chứng minh test đang kiểm đúng
	// chỗ: không phải revision bị xoá theo, mà là truy vấn phải tự loại nó ra.
	var con int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM document_revisions WHERE id=?`,
		rid).Scan(&con).Error)
	require.Equal(t, int64(1), con, "revision vẫn nằm trong bảng, chỉ documents bị đánh dấu xoá")

	_, err = repo.FindRevision(ctx, pid, did, rid)
	require.Error(t, err, "revision của tài liệu đã xoá KHÔNG được đọc")
}

// Nặng hơn việc đọc: retry một revision của tài liệu đã xoá sẽ đẩy job ingestion
// và nạp ngược tài liệu đó lên RAGFlow.
func TestRetry_KhongChayLaiChoTaiLieuDaXoa(t *testing.T) {
	db := openTestDB(t)
	repo := repository.New(db)
	ctx := context.Background()
	pid, did, rid, actor := dungDuLieu(t, db)

	require.NoError(t, repo.SoftDelete(ctx, pid, did, actor))

	// SoftDelete sinh 1 event cleanup. Đếm mốc để biết retry có đẩy thêm không.
	var truoc int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM outbox_events WHERE aggregate_id=?`,
		did).Scan(&truoc).Error)

	err := repo.Retry(ctx, pid, did, rid, actor)
	require.Error(t, err, "không được retry revision của tài liệu đã xoá")

	var sau int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM outbox_events WHERE aggregate_id=?`,
		did).Scan(&sau).Error)
	require.Equal(t, truoc, sau, "không được đẩy thêm job ingestion nào")

	// Trạng thái revision cũng phải giữ nguyên 'failed', không bị đổi sang 'queued'.
	var trangThai string
	require.NoError(t, db.Raw(`SELECT status FROM document_revisions WHERE id=?`,
		rid).Scan(&trangThai).Error)
	require.Equal(t, "failed", trangThai)
}

// Chốt chặn mới không được làm hỏng đường đi bình thường.
func TestFindRevision_TaiLieuConSongVanDocDuoc(t *testing.T) {
	db := openTestDB(t)
	repo := repository.New(db)
	ctx := context.Background()
	pid, did, rid, _ := dungDuLieu(t, db)

	rev, err := repo.FindRevision(ctx, pid, did, rid)
	require.NoError(t, err)
	require.Equal(t, "failed", rev.Status)
	require.Equal(t, 1, rev.RevisionNo)
}

func TestList_ChiTraTaiLieuDaXoaKhiDuocYeuCau(t *testing.T) {
	db := openTestDB(t)
	repo := repository.New(db)
	ctx := context.Background()
	pid, did, _, actor := dungDuLieu(t, db)

	require.NoError(t, repo.SoftDelete(ctx, pid, did, actor))

	items, total, err := repo.List(ctx, pid, domain.Filter{}, pagination.Query{Page: 1, Limit: 20})
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, items)

	items, total, err = repo.List(ctx, pid, domain.Filter{IncludeDeleted: true}, pagination.Query{Page: 1, Limit: 20})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	require.Equal(t, did, items[0].ID)
	require.True(t, items[0].IsDeleted)
	require.NotNil(t, items[0].DeletedAt)
}
