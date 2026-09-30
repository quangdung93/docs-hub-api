package domain

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

// RevisionStatusArchived phải khớp ĐÚNG chuỗi trong mệnh đề WHERE của hai chỉ
// số nội dung.
//
// Lệch nhau là hỏng câm theo cả hai chiều: đổi hằng Go mà quên đổi chỉ số thì
// SoftDelete ghi một giá trị chỉ số không biết tới, sha256 vẫn khoá và người
// dùng vẫn không tải lại được file đã xoá; đổi chỉ số mà quên đổi hằng thì y
// hệt. Không phép thử nào khác bắt được, vì cả hai phía đều chạy trót lọt —
// chỉ có kết quả là sai.
func TestRevisionStatusArchived_KhopVoiChiSoTrongMigration(t *testing.T) {
	duongDan := filepath.Join("..", "..", "..", "..",
		"migrations", "000007_create_documents.up.sql")
	noiDung, err := os.ReadFile(duongDan) //nolint:gosec // đường dẫn cố định trong repo
	require.NoError(t, err, "không đọc được migration tạo uk_revisions_*_hash")

	for _, chiSo := range []string{"uk_revisions_version_hash", "uk_revisions_change_hash"} {
		mau := regexp.MustCompile(`(?is)` + chiSo + `.*?status\s*<>\s*'([^']*)'`)
		khop := mau.FindSubmatch(noiDung)
		require.Len(t, khop, 2, "không tìm thấy điều kiện status <> '...' của "+chiSo)
		require.Equal(t, RevisionStatusArchived, string(khop[1]),
			chiSo+" loại trừ một trạng thái khác với RevisionStatusArchived")
	}
}

// Migration vá dữ liệu cũ phải ghi đúng giá trị đó. Ghi lệch thì nó chạy trót
// lọt, báo thành công, mà không tài liệu cũ nào được nhả ra.
func TestRevisionStatusArchived_KhopVoiMigrationVaDuLieuCu(t *testing.T) {
	duongDan := filepath.Join("..", "..", "..", "..",
		"migrations", "000018_archive_revisions_of_deleted_documents.up.sql")
	noiDung, err := os.ReadFile(duongDan) //nolint:gosec // đường dẫn cố định trong repo
	require.NoError(t, err)

	mau := regexp.MustCompile(`(?is)SET\s+status\s*=\s*'([^']*)'`)
	khop := mau.FindSubmatch(noiDung)
	require.Len(t, khop, 2, "không tìm thấy câu SET status = '...' trong migration")
	require.Equal(t, RevisionStatusArchived, string(khop[1]))
}
