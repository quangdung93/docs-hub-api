//go:build integration

// Integration test cho lỗi "xoá tài liệu rồi tải lại đúng file đó vẫn bị báo
// trùng". Bắt buộc chạy trên Postgres thật: thứ đang kiểm là hai chỉ số RIÊNG
// PHẦN uk_revisions_*_hash với điều kiện `status <> 'archived'` — repo giả
// không có chỉ số thì mọi unit test vẫn xanh dù bản sửa sai.
package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/module/document/domain"
	"github.com/quangdung93/docs-hub-api/internal/module/document/repository"
)

// Đây là ca người dùng gặp: xoá nhầm tài liệu, tải lại chính file đó lên thì bị
// chặn và không còn đường nào gỡ ngoài sửa tay trong database.
func TestSoftDelete_XoaRoiTaiLaiDungNoiDungDo_KhongBiChan(t *testing.T) {
	db := openTestDB(t)
	repo := repository.New(db)
	ctx := context.Background()
	pid, vA, _, _, actor := duLieuScope(t, db)
	sha := sha64("xoa-roi-tai-lai")

	dau := thamSo(pid, actor, domain.Scope{VersionID: &vA}, sha)
	doc, _, err := repo.CreateRevision(ctx, dau)
	require.NoError(t, err)

	// Còn sống thì vẫn phải chặn — nếu không, test dưới xanh vì lý do sai.
	_, _, err = repo.CreateRevision(ctx, thamSo(pid, actor, domain.Scope{VersionID: &vA}, sha))
	require.ErrorIs(t, err, domain.ErrDuplicateContent,
		"tài liệu còn sống thì trùng nội dung vẫn phải bị chặn")

	require.NoError(t, repo.SoftDelete(ctx, pid, doc.ID, actor))

	_, _, err = repo.CreateRevision(ctx, thamSo(pid, actor, domain.Scope{VersionID: &vA}, sha))
	require.NoError(t, err, "xoá rồi thì nội dung đó phải tải lên lại được")
}

// Nhả sha256 nằm ở chỗ đánh dấu 'archived'. Kiểm thẳng giá trị cột để khi test
// trên đỏ còn biết ngay là do SoftDelete không đánh dấu hay do chỉ số sai.
func TestSoftDelete_LuuTruMoiRevisionCuaTaiLieu(t *testing.T) {
	db := openTestDB(t)
	repo := repository.New(db)
	ctx := context.Background()
	pid, vA, _, _, actor := duLieuScope(t, db)

	dau := thamSo(pid, actor, domain.Scope{VersionID: &vA}, sha64("luu-tru-1"))
	dau.AutoVersion = true
	doc, _, err := repo.CreateRevision(ctx, dau)
	require.NoError(t, err)

	// Revision thứ hai của CÙNG tài liệu: cả hai đều phải được lưu trữ, không
	// riêng bản mới nhất — mỗi revision giữ một sha256 khác nhau.
	sau := thamSo(pid, actor, domain.Scope{VersionID: &vA}, sha64("luu-tru-2"))
	sau.DocumentID, sau.AutoVersion = doc.ID, true
	_, _, err = repo.CreateRevision(ctx, sau)
	require.NoError(t, err)

	require.NoError(t, repo.SoftDelete(ctx, pid, doc.ID, actor))

	var conSot int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM document_revisions
		WHERE document_id=? AND status<>?`, doc.ID, domain.RevisionStatusArchived).
		Scan(&conSot).Error)
	require.Zero(t, conSot, "mọi revision của tài liệu đã xoá phải mang nhãn lưu trữ")
}

// Đánh dấu phải bó đúng trong một tài liệu. Viết sai mệnh đề WHERE thành
// project_id là nhả sha256 của cả dự án, lỗi này im lặng và rất khó thấy.
func TestSoftDelete_KhongLuuTruNhamTaiLieuKhac(t *testing.T) {
	db := openTestDB(t)
	repo := repository.New(db)
	ctx := context.Background()
	pid, vA, _, _, actor := duLieuScope(t, db)

	xoa := thamSo(pid, actor, domain.Scope{VersionID: &vA}, sha64("bi-xoa"))
	docXoa, _, err := repo.CreateRevision(ctx, xoa)
	require.NoError(t, err)

	giuLai := thamSo(pid, actor, domain.Scope{VersionID: &vA}, sha64("giu-lai"))
	docGiu, revGiu, err := repo.CreateRevision(ctx, giuLai)
	require.NoError(t, err)

	require.NoError(t, repo.SoftDelete(ctx, pid, docXoa.ID, actor))

	var trangThai string
	require.NoError(t, db.Raw(`SELECT status FROM document_revisions WHERE id=?`,
		revGiu.ID).Scan(&trangThai).Error)
	require.Equal(t, "queued", trangThai, "tài liệu khác trong cùng dự án phải nguyên vẹn")

	_, _, err = repo.CreateRevision(ctx, thamSo(pid, actor, domain.Scope{VersionID: &vA}, sha64("giu-lai")))
	require.ErrorIs(t, err, domain.ErrDuplicateContent,
		"tài liệu chưa xoá thì nội dung của nó vẫn phải bị khoá")
	require.NotEqual(t, docXoa.ID, docGiu.ID)
}

// Tải lại sau khi xoá sinh một TÀI LIỆU MỚI chứ không hồi sinh bản cũ: bản cũ
// vẫn nằm trong thùng rác cùng lịch sử của nó.
func TestSoftDelete_TaiLaiSinhTaiLieuMoi_BanCuVanLaDaXoa(t *testing.T) {
	db := openTestDB(t)
	repo := repository.New(db)
	ctx := context.Background()
	pid, vA, _, _, actor := duLieuScope(t, db)
	sha := sha64("tai-lieu-moi")

	dau := thamSo(pid, actor, domain.Scope{VersionID: &vA}, sha)
	dau.FileName, dau.AutoVersion = "spec.txt", true
	docCu, _, err := repo.CreateRevision(ctx, dau)
	require.NoError(t, err)
	require.NoError(t, repo.SoftDelete(ctx, pid, docCu.ID, actor))

	sau := thamSo(pid, actor, domain.Scope{VersionID: &vA}, sha)
	sau.FileName, sau.AutoVersion = "spec.txt", true
	docMoi, revMoi, err := repo.CreateRevision(ctx, sau)
	require.NoError(t, err)
	require.NotEqual(t, docCu.ID, docMoi.ID, "phải là tài liệu mới, không dùng lại bản đã xoá")
	require.Equal(t, 1, revMoi.RevisionNo)

	var daXoa bool
	require.NoError(t, db.Raw(`SELECT deleted_at IS NOT NULL FROM documents WHERE id=?`,
		docCu.ID).Scan(&daXoa).Error)
	require.True(t, daXoa, "bản cũ vẫn phải ở trạng thái đã xoá")
}
