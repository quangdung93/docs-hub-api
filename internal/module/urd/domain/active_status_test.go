package domain

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ActiveStatuses phải khớp ĐÚNG mệnh đề WHERE của uk_urd_analyses_active.
//
// Lệch nhau là hỏng câm: thêm một trạng thái vào chỉ số mà quên thêm vào Go
// thì Cancel tưởng đã gỡ khoá nhưng chỉ số vẫn giữ, tài liệu kẹt vĩnh viễn;
// bỏ bớt ở chỉ số mà quên bỏ ở Go thì hai phân tích cùng chạy trên một tài
// liệu. Cả hai đều không có test nào khác bắt được, nên đối chiếu thẳng với
// file migration.
func TestActiveStatuses_KhopVoiChiSoTrongMigration(t *testing.T) {
	duongDan := filepath.Join("..", "..", "..", "..",
		"migrations", "000014_urd_edge_case_analysis.up.sql")
	noiDung, err := os.ReadFile(duongDan) //nolint:gosec // đường dẫn cố định trong repo
	require.NoError(t, err, "không đọc được migration tạo uk_urd_analyses_active")

	// Bắt phần trong ngoặc của "WHERE status IN (...)" thuộc chỉ số này.
	mau := regexp.MustCompile(`(?is)uk_urd_analyses_active.*?WHERE\s+status\s+IN\s*\(([^)]*)\)`)
	khop := mau.FindSubmatch(noiDung)
	require.Len(t, khop, 2, "không tìm thấy mệnh đề WHERE status IN của uk_urd_analyses_active")

	trongMigration := make([]string, 0, 2)
	for _, phan := range strings.Split(string(khop[1]), ",") {
		trongMigration = append(trongMigration, strings.Trim(strings.TrimSpace(phan), "'"))
	}

	require.ElementsMatch(t, ActiveStatuses(), trongMigration,
		"ActiveStatuses() và uk_urd_analyses_active phải liệt kê cùng một tập trạng thái")
}

// Trạng thái kết thúc tuyệt đối không được nằm trong tập hoạt động — nếu có,
// tài liệu hoàn tất rồi vẫn bị chỉ số coi là đang khoá.
func TestActiveStatuses_KhongChuaTrangThaiKetThuc(t *testing.T) {
	for _, ketThuc := range []string{StatusCompleted, StatusFailed, StatusCancelled} {
		require.NotContains(t, ActiveStatuses(), ketThuc)
	}
}
