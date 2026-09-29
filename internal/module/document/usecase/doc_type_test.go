package usecase

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/module/document/domain"
)

// suggestDocType là cửa duy nhất mở ra luồng phân tích edge case — không gợi
// ý thì FE không hiện popup, người dùng không xác nhận được, tài liệu không
// phân tích được. Trước bản này hàm không có một test nào.
func TestSuggestDocType_TenTaiLieuThat(t *testing.T) {
	tests := []struct {
		name, title, fileName, want string
	}{
		// Tên thật đang có trên production — bộ này chặn hồi quy khi ai đó
		// đổi cách so khớp.
		{"URD viết hoa có khoảng trắng", "ISC_MBX URD v1.0", "ISC_MBX_URD_v1.0.docx", domain.DocTypeURD},
		{"urd viết thường trong tên file", "app_bac_si_urd_v1.1.pdf", "app_bac_si_urd_v1.1.pdf", domain.DocTypeURD},
		{"URD nối bằng gạch ngang", "URD-MBX26-v1.8.docx", "URD-MBX26-v1.8.docx", domain.DocTypeURD},
		{"URD nằm giữa tên dài", "ISC_FPT-DOCS-HUB_v1_0_URD_v1_0_FILLED", "x.docx", domain.DocTypeURD},

		{"PRD viết hoa", "PRD-Quan-ly-kho-v1.0", "PRD-Quan-ly-kho-v1.0.docx", domain.DocTypePRD},
		{"prd viết thường", "tai_lieu_prd_v2.docx", "tai_lieu_prd_v2.docx", domain.DocTypePRD},

		// BRD chỉ khác URD một chữ cái, tuyệt đối không được nhầm.
		{"BRD không phải URD", "BRD-MBX26-v1.0.docx", "BRD-MBX26-v1.0.docx", ""},
		{"báo cáo UAT", "UAT_Report_2026-09-10.xlsx", "UAT_Report_2026-09-10.xlsx", ""},
		{"tài liệu kỹ thuật", "mcp-integration.md", "mcp-integration.md", ""},
		{"biên bản họp", "Bien-ban-hop-giao-ban-tuan-39", "Bien-ban-hop-giao-ban-tuan-39.docx", ""},

		// Tiêu đề và tên file là hai nguồn riêng, khớp ở nguồn nào cũng tính.
		{"chỉ tiêu đề có URD", "Đặc tả URD hệ thống", "tai-lieu-1.docx", domain.DocTypeURD},
		{"chỉ tên file có PRD", "Tài liệu yêu cầu", "prd-v1.docx", domain.DocTypePRD},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, suggestDocType(tc.title, tc.fileName, ""))
		})
	}
}

// Tên chứa cả hai từ khoá: URD thắng. Luồng phân tích edge case sinh ra để
// phục vụ URD (URD v1.2 mục XI), PRD là phần mở rộng sau.
func TestSuggestDocType_CoCaHaiTuKhoaThiUuTienURD(t *testing.T) {
	require.Equal(t, domain.DocTypeURD, suggestDocType("URD-PRD-mapping", "URD-PRD-mapping.docx", ""))
	require.Equal(t, domain.DocTypeURD, suggestDocType("PRD va URD doi chieu", "x.docx", ""))
}

// Đã xác nhận loại rồi thì không gợi ý lại, kể cả khi tải lên revision mới.
func TestSuggestDocType_DaXacNhanThiKhongGoiYLai(t *testing.T) {
	require.Empty(t, suggestDocType("URD-MBX26-v1.8", "URD-MBX26-v1.8.docx", domain.DocTypeURD))
	require.Empty(t, suggestDocType("PRD-kho", "PRD-kho.docx", domain.DocTypePRD))
	// Đổi loại: tài liệu đang là PRD mà tên gợi ý URD cũng không gợi ý lại —
	// người dùng đã chốt rồi, hỏi lại là phiền.
	require.Empty(t, suggestDocType("URD-MBX26", "URD-MBX26.docx", domain.DocTypePRD))
}

// Giới hạn đã biết của cách so khớp chuỗi con: tài liệu URD đặt tên tiếng
// Việt thì bỏ sót, còn từ chứa "urd" ở giữa thì nhận nhầm. Ghi lại thành test
// để đó là hành vi ĐƯỢC BIẾT chứ không phải chỗ hỏng chưa ai thấy — muốn siết
// thì đổi sang so khớp theo từ, nhưng sẽ hỏng với tên viết liền kiểu "URDv1".
func TestSuggestDocType_GioiHanDaBiet(t *testing.T) {
	require.Empty(t, suggestDocType("Đặc tả yêu cầu người dùng", "dac-ta-yeu-cau.docx", ""),
		"URD đặt tên tiếng Việt hiện KHÔNG được nhận diện")
	require.Equal(t, domain.DocTypeURD, suggestDocType("Phân tích absurd", "bao-cao-absurd.docx", ""),
		"từ chứa chuỗi urd hiện BỊ nhận nhầm")
}

func TestIsAnalyzableDocType(t *testing.T) {
	require.True(t, domain.IsAnalyzableDocType(domain.DocTypeURD))
	require.True(t, domain.IsAnalyzableDocType(domain.DocTypePRD))
	require.False(t, domain.IsAnalyzableDocType(""), "chưa xác nhận thì không phân tích được")
	require.False(t, domain.IsAnalyzableDocType("srs"))
}

// Nhãn dùng trong prompt gửi AI — sai tên loại là AI phân tích sai trọng tâm.
func TestDocTypeLabel(t *testing.T) {
	require.Equal(t, "URD (User Requirement Document)", domain.DocTypeLabel(domain.DocTypeURD))
	require.Equal(t, "PRD (Product Requirement Document)", domain.DocTypeLabel(domain.DocTypePRD))
	require.Equal(t, "srs", domain.DocTypeLabel("srs"), "loại lạ thì giữ nguyên, không nuốt mất")
}
