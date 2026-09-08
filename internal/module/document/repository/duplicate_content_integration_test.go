//go:build integration

// Integration test cho mục #5: upload trùng nội dung phải thành lỗi NGHIỆP VỤ
// chứ không lọt thành SYS_500.
//
// Bắt buộc chạy trên Postgres thật. Bản sửa nhận diện lỗi bằng TÊN ràng buộc
// (uk_revisions_version_hash / uk_revisions_change_hash); gõ sai tên thì mọi
// unit test với repo giả vẫn xanh, chỉ có database thật mới bắt được.
package repository_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/internal/module/document/domain"
	"github.com/quangdung93/docs-hub-api/internal/module/document/repository"
)

// duLieuScope dựng project cùng HAI version và MỘT change request, đủ để kiểm
// cả trường hợp trùng lẫn trường hợp khác phạm vi thì không trùng.
func duLieuScope(t *testing.T, db *gorm.DB) (projectID, versionA, versionB, changeReq, actorID uuid.UUID) {
	t.Helper()
	projectID, actorID = uuid.New(), uuid.New()
	versionA, versionB, changeReq = uuid.New(), uuid.New(), uuid.New()

	require.NoError(t, db.Exec(`INSERT INTO users (id,email,password_hash,full_name)
		VALUES (?,?,?,?)`,
		actorID, "dup-"+actorID.String()+"@test.local", "x", "Dup Actor").Error)
	require.NoError(t, db.Exec(`INSERT INTO projects (id,owner_id,name,code)
		VALUES (?,?,?,?)`, projectID, actorID, "Dup Project",
		"DUP-"+projectID.String()[:8]).Error)
	for i, vid := range []uuid.UUID{versionA, versionB} {
		require.NoError(t, db.Exec(`INSERT INTO project_versions
			(id,project_id,label,sequence_no,status,created_by)
			VALUES (?,?,?,?,'draft',?)`,
			vid, projectID, "v1."+string(rune('0'+i)), i+1, actorID).Error)
	}
	require.NoError(t, db.Exec(`INSERT INTO change_requests
		(id,project_id,code,title,status,sequence_no,created_by)
		VALUES (?,?,?,?,'draft',1,?)`,
		changeReq, projectID, "CR-"+changeReq.String()[:8], "CR thử", actorID).Error)
	return projectID, versionA, versionB, changeReq, actorID
}

// sha64 sinh chuỗi đúng 64 ký tự — sha256 là CHAR(64), thiếu thì Postgres đệm
// khoảng trắng và phép so trùng lệch đi.
func sha64(seed string) string {
	return (seed + strings.Repeat("0", 64))[:64]
}

func thamSo(projectID, actorID uuid.UUID, scope domain.Scope, sha string) domain.CreateRevisionParams {
	rid := uuid.New()
	return domain.CreateRevisionParams{
		DocumentID: uuid.New(), RevisionID: rid, ProjectID: projectID, ActorID: actorID,
		Scope: scope, Title: "Tài liệu " + rid.String()[:8], FileName: "a.txt",
		MediaType: "text/plain", SHA256: sha, ObjectKey: "obj-" + rid.String(), SizeBytes: 10,
	}
}

// Đây là bug #5: cùng nội dung, cùng version -> chỉ số uk_revisions_version_hash
// chặn, và lỗi 23505 phải được dịch thành lỗi nghiệp vụ chứ không lọt lên 500.
func TestCreateRevision_TrungNoiDungTrongCungVersion(t *testing.T) {
	db := openTestDB(t)
	repo := repository.New(db)
	ctx := context.Background()
	pid, vA, _, _, actor := duLieuScope(t, db)
	sha := sha64("dup-version")

	_, _, err := repo.CreateRevision(ctx, thamSo(pid, actor, domain.Scope{VersionID: &vA}, sha))
	require.NoError(t, err, "lần nạp đầu phải thành công")

	_, _, err = repo.CreateRevision(ctx, thamSo(pid, actor, domain.Scope{VersionID: &vA}, sha))
	require.ErrorIs(t, err, domain.ErrDuplicateContent,
		"nạp lại đúng nội dung đó phải ra lỗi nghiệp vụ, không phải lỗi thô")
}

// Cùng cơ chế nhưng qua chỉ số kia — uk_revisions_change_hash. Kiểm riêng vì
// bản sửa liệt kê hai tên ràng buộc, sót một cái thì nửa số đường vẫn ra 500.
func TestCreateRevision_TrungNoiDungTrongCungChangeRequest(t *testing.T) {
	db := openTestDB(t)
	repo := repository.New(db)
	ctx := context.Background()
	pid, _, _, cr, actor := duLieuScope(t, db)
	sha := sha64("dup-change-request")

	_, _, err := repo.CreateRevision(ctx, thamSo(pid, actor, domain.Scope{ChangeRequestID: &cr}, sha))
	require.NoError(t, err)

	_, _, err = repo.CreateRevision(ctx, thamSo(pid, actor, domain.Scope{ChangeRequestID: &cr}, sha))
	require.ErrorIs(t, err, domain.ErrDuplicateContent)
}

// Chỉ số là RIÊNG PHẦN theo scope: cùng nội dung nhưng khác version thì hợp lệ.
// Nếu bản sửa bắt trùng rộng tay hơn ràng buộc thì test này đỏ.
func TestCreateRevision_KhacVersionThiKhongTinhLaTrung(t *testing.T) {
	db := openTestDB(t)
	repo := repository.New(db)
	ctx := context.Background()
	pid, vA, vB, _, actor := duLieuScope(t, db)
	sha := sha64("khac-version")

	_, _, err := repo.CreateRevision(ctx, thamSo(pid, actor, domain.Scope{VersionID: &vA}, sha))
	require.NoError(t, err)

	_, _, err = repo.CreateRevision(ctx, thamSo(pid, actor, domain.Scope{VersionID: &vB}, sha))
	require.NoError(t, err, "cùng nội dung ở version khác là hợp lệ, không được chặn")
}

// Chốt chặn quan trọng nhất của bản sửa: KHÔNG được bắt bừa mọi lỗi 23505.
// object_key trùng là lỗi sinh khoá của chính hệ thống — người dùng không làm gì
// được, phải giữ nguyên là lỗi kỹ thuật chứ đừng báo họ "nội dung trùng".
func TestCreateRevision_TrungObjectKeyVanLaLoiKyThuat(t *testing.T) {
	db := openTestDB(t)
	repo := repository.New(db)
	ctx := context.Background()
	pid, vA, _, _, actor := duLieuScope(t, db)

	dau := thamSo(pid, actor, domain.Scope{VersionID: &vA}, sha64("obj-key-1"))
	_, _, err := repo.CreateRevision(ctx, dau)
	require.NoError(t, err)

	// Nội dung KHÁC (sha khác) nhưng object_key lặp lại.
	sau := thamSo(pid, actor, domain.Scope{VersionID: &vA}, sha64("obj-key-2"))
	sau.ObjectKey = dau.ObjectKey
	_, _, err = repo.CreateRevision(ctx, sau)
	require.Error(t, err)
	require.NotErrorIs(t, err, domain.ErrDuplicateContent,
		"trùng object_key là lỗi nội bộ, không được đội lốt lỗi nghiệp vụ")
}
