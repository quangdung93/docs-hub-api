package usecase

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

func TestBuildTestcaseWorkbook_DienDungCotVaDonSheetMau(t *testing.T) {
	t.Parallel()
	in := testcaseContentInput{
		ProjectName: "Demo Project",
		Items: []testcaseRAGItem{
			{
				ReqID: "REQ-01", DocSource: "srs.docx", Group: "Functional", Priority: "High",
				Title: "Đăng nhập thành công", Precondition: "Đã có tài khoản",
				Steps: "1. Nhập user/pass\n2. Bấm Đăng nhập", Expected: "Vào được trang chủ",
			},
			{
				ReqID: "REQ-02", DocSource: "srs.docx", Group: "UI", Priority: "Medium",
				Title: "Hiển thị lỗi khi sai mật khẩu", Steps: "Nhập sai mật khẩu",
				Expected: "Hiển thị thông báo lỗi",
			},
		},
	}
	raw, err := buildTestcaseWorkbook(in)
	require.NoError(t, err)
	require.NotEmpty(t, raw)

	f, err := excelize.OpenReader(bytes.NewReader(raw))
	require.NoError(t, err)
	defer f.Close()

	projectName, err := f.GetCellValue(testcaseSheetSummary, "C6")
	require.NoError(t, err)
	require.Equal(t, "Demo Project", projectName)

	funcName, err := f.GetCellValue(testcaseSheetCases, "C3")
	require.NoError(t, err)
	require.Equal(t, "Demo Project", funcName)

	req1, err := f.GetCellValue(testcaseSheetCases, "B7")
	require.NoError(t, err)
	require.Equal(t, "REQ-01", req1)
	title1, err := f.GetCellValue(testcaseSheetCases, "F7")
	require.NoError(t, err)
	require.Equal(t, "Đăng nhập thành công", title1)
	expected2, err := f.GetCellValue(testcaseSheetCases, "I8")
	require.NoError(t, err)
	require.Equal(t, "Hiển thị thông báo lỗi", expected2)

	// Cột A là công thức có sẵn — không được ghi đè, phải còn nguyên "=IF(...".
	formula, err := f.GetCellFormula(testcaseSheetCases, "A7")
	require.NoError(t, err)
	require.Contains(t, formula, "COUNTA")

	// Sheet "Test Case 2" (dữ liệu mẫu gốc của template) phải được dọn sạch để
	// Dashboard không đếm nhầm test case ảo.
	sampleReq, err := f.GetCellValue(testcaseSheetSample2, "B7")
	require.NoError(t, err)
	require.Empty(t, sampleReq)
	sampleTitle, err := f.GetCellValue(testcaseSheetSample2, "F7")
	require.NoError(t, err)
	require.Empty(t, sampleTitle)
}

func TestBuildTestcaseWorkbook_KhongDeCotQAConLai(t *testing.T) {
	t.Parallel()
	in := testcaseContentInput{Items: []testcaseRAGItem{{Title: "TC", Steps: "B", Expected: "C"}}}
	raw, err := buildTestcaseWorkbook(in)
	require.NoError(t, err)

	f, err := excelize.OpenReader(bytes.NewReader(raw))
	require.NoError(t, err)
	defer f.Close()

	// Cột J (Origin) trở đi không bị hệ thống điền — để trống cho QA.
	origin, err := f.GetCellValue(testcaseSheetCases, "J7")
	require.NoError(t, err)
	require.Empty(t, origin)
}

func TestBuildTestcaseWorkbook_DonDuLieuMauCacSheetConLai(t *testing.T) {
	t.Parallel()
	raw, err := buildTestcaseWorkbook(testcaseContentInput{ProjectName: "Demo Project"})
	require.NoError(t, err)

	f, err := excelize.OpenReader(bytes.NewReader(raw))
	require.NoError(t, err)
	defer f.Close()

	// Bug Data: 13 bug giả IP-101..IP-113, nguồn của mọi thống kê trong báo cáo.
	for _, cell := range []string{"A2", "C2", "F2", "N2", "V2", "A14", "C14"} {
		got, err := f.GetCellValue(testcaseSheetBugData, cell)
		require.NoError(t, err)
		require.Empty(t, got, "Bug Data!%s còn dữ liệu mẫu", cell)
	}

	// RTM: 5 yêu cầu giả REQ-01..REQ-05 chẳng liên quan tới dự án thật.
	for _, cell := range []string{"B6", "C6", "D6", "B10", "C10", "D10"} {
		got, err := f.GetCellValue(testcaseSheetRTM, cell)
		require.NoError(t, err)
		require.Empty(t, got, "RTM!%s còn dữ liệu mẫu", cell)
	}

	// Summary: version/sprint/ngày test/PIC và khối môi trường của dự án mẫu.
	for _, cell := range []string{"C8", "C9", "C10", "C11", "C13", "C57", "C59", "C62"} {
		got, err := f.GetCellValue(testcaseSheetSummary, cell)
		require.NoError(t, err)
		require.Empty(t, got, "Summary!%s còn dữ liệu mẫu", cell)
	}

	for _, cell := range []string{"B7", "C7", "D7", "E7", "F7", "G7"} {
		got, err := f.GetCellValue(testcaseSheetRevision, cell)
		require.NoError(t, err)
		require.Empty(t, got, "Revision History!%s còn dữ liệu mẫu", cell)
	}

	for _, cell := range []string{"C42", "C43", "C44", "L218", "L223"} {
		got, err := f.GetCellValue(testcaseSheetReport, cell)
		require.NoError(t, err)
		require.Empty(t, got, "Report Test!%s còn dữ liệu mẫu", cell)
	}
}

func TestBuildTestcaseWorkbook_DienTenDuAnVaoTrangBia(t *testing.T) {
	t.Parallel()
	raw, err := buildTestcaseWorkbook(testcaseContentInput{ProjectName: "Demo Project"})
	require.NoError(t, err)

	f, err := excelize.OpenReader(bytes.NewReader(raw))
	require.NoError(t, err)
	defer f.Close()

	// Cover!B6 là chỗ trống chờ điền ("[ĐIỀN TÊN DỰ ÁN]"), phải GHI ĐÈ chứ không
	// xoá trắng — nếu không thì trang bìa in nguyên placeholder.
	cover, err := f.GetCellValue(testcaseSheetCover, "B6")
	require.NoError(t, err)
	require.Equal(t, "Demo Project", cover)
}

func TestBuildTestcaseWorkbook_GiuNguyenKhungThongKe(t *testing.T) {
	t.Parallel()
	raw, err := buildTestcaseWorkbook(testcaseContentInput{ProjectName: "Demo Project"})
	require.NoError(t, err)

	f, err := excelize.OpenReader(bytes.NewReader(raw))
	require.NoError(t, err)
	defer f.Close()

	// Tiêu đề cột của Bug Data nằm ở dòng 1, ngoài vùng dọn.
	header, err := f.GetCellValue(testcaseSheetBugData, "A1")
	require.NoError(t, err)
	require.Equal(t, "Key", header)

	// Mã chức năng F01..F10 ở Report Test là khung bảng, chỉ TÊN chức năng bị dọn.
	ma, err := f.GetCellValue(testcaseSheetReport, "B42")
	require.NoError(t, err)
	require.Equal(t, "F01", ma)
}

// RTM được dựng lại từ chính test case vừa sinh, thay cho 5 yêu cầu ví dụ của
// template. Dòng có yêu cầu thì GIỮ công thức đếm; dòng không có thì phải rỗng
// HOÀN TOÀN, vì Req ID rỗng làm COUNTIF("*"&B&"*") thành "**" — khớp mọi ô có
// chữ, khiến dòng trống báo là phủ toàn bộ test case.
func TestBuildTestcaseWorkbook_RTMDungLaiTuMaYeuCauThat(t *testing.T) {
	t.Parallel()
	in := testcaseContentInput{
		ProjectName: "Demo Project",
		Items: []testcaseRAGItem{
			{ReqID: "REQ-01", DocSource: "srs.docx", Title: "A"},
			{ReqID: "REQ-02", DocSource: "urd.docx", Title: "B"},
			{ReqID: "REQ-01", DocSource: "srs.docx", Title: "C"}, // trùng, không thêm dòng
		},
	}
	raw, err := buildTestcaseWorkbook(in)
	require.NoError(t, err)

	f, err := excelize.OpenReader(bytes.NewReader(raw))
	require.NoError(t, err)
	defer f.Close()

	req1, err := f.GetCellValue(testcaseSheetRTM, "B6")
	require.NoError(t, err)
	require.Equal(t, "REQ-01", req1)
	nguon1, err := f.GetCellValue(testcaseSheetRTM, "D6")
	require.NoError(t, err)
	require.Equal(t, "srs.docx", nguon1)
	req2, err := f.GetCellValue(testcaseSheetRTM, "B7")
	require.NoError(t, err)
	require.Equal(t, "REQ-02", req2)

	// Mô tả yêu cầu để trống: app không có nội dung yêu cầu, tự sinh là bịa.
	moTa, err := f.GetCellValue(testcaseSheetRTM, "C6")
	require.NoError(t, err)
	require.Empty(t, moTa)

	// Dòng CÓ yêu cầu phải còn công thức đếm.
	congThuc, err := f.GetCellFormula(testcaseSheetRTM, "E6")
	require.NoError(t, err)
	require.Contains(t, congThuc, "COUNTIF")

	// Dòng KHÔNG có yêu cầu phải sạch cả công thức.
	for _, cell := range []string{"B8", "C8", "D8", "E8", "F8", "I10", "J10"} {
		giaTri, err := f.GetCellValue(testcaseSheetRTM, cell)
		require.NoError(t, err)
		require.Empty(t, giaTri, "RTM!%s còn giá trị", cell)
		fm, err := f.GetCellFormula(testcaseSheetRTM, cell)
		require.NoError(t, err)
		require.Empty(t, fm, "RTM!%s còn công thức", cell)
	}
}

// I11 (dòng "Tổng") nằm chung nhóm shared formula với I6:I10 nên bị xoá lây khi
// dọn — phải được khôi phục.
func TestBuildTestcaseWorkbook_RTMGiuNguyenDongTong(t *testing.T) {
	t.Parallel()
	raw, err := buildTestcaseWorkbook(testcaseContentInput{ProjectName: "Demo Project"})
	require.NoError(t, err)

	f, err := excelize.OpenReader(bytes.NewReader(raw))
	require.NoError(t, err)
	defer f.Close()

	tong, err := f.GetCellFormula(testcaseSheetRTM, "I11")
	require.NoError(t, err)
	require.Equal(t, "IFERROR(F11/E11,0)", tong)

	// E11:H11 là SUM(E6:E10) — vẫn còn, và cộng trên vùng đã dọn nên ra 0.
	for _, cell := range []string{"E11", "F11", "G11", "H11"} {
		fm, err := f.GetCellFormula(testcaseSheetRTM, cell)
		require.NoError(t, err)
		require.Contains(t, fm, "SUM(")
	}
}

// Không test case nào có mã yêu cầu (prompt yêu cầu để trống thay vì tự đặt)
// thì RTM phải rỗng hoàn toàn, không còn yêu cầu giả nào.
func TestBuildTestcaseWorkbook_RTMRongKhiKhongCoMaYeuCau(t *testing.T) {
	t.Parallel()
	in := testcaseContentInput{
		ProjectName: "Demo Project",
		Items:       []testcaseRAGItem{{ReqID: "", DocSource: "srs.docx", Title: "A"}},
	}
	raw, err := buildTestcaseWorkbook(in)
	require.NoError(t, err)

	f, err := excelize.OpenReader(bytes.NewReader(raw))
	require.NoError(t, err)
	defer f.Close()

	for row := testcaseRTMFirstRow; row <= testcaseRTMLastRow; row++ {
		for _, col := range []string{"B", "C", "D", "E", "J"} {
			cell := fmt.Sprintf("%s%d", col, row)
			giaTri, err := f.GetCellValue(testcaseSheetRTM, cell)
			require.NoError(t, err)
			require.Empty(t, giaTri, "RTM!%s còn dữ liệu mẫu", cell)
		}
	}
}

func TestDistinctRequirements_BoTrungBoRongVaChanTheoSoDong(t *testing.T) {
	t.Parallel()
	require.Empty(t, distinctRequirements(nil))

	got := distinctRequirements([]testcaseRAGItem{
		{ReqID: " REQ-01 ", DocSource: "a.docx"},
		{ReqID: "REQ-01", DocSource: "b.docx"}, // trùng sau khi trim
		{ReqID: "", DocSource: "c.docx"},       // rỗng: bỏ qua
		{ReqID: "REQ-02", DocSource: "d.docx"},
	})
	require.Equal(t, []rtmRequirement{
		{reqID: "REQ-01", docSource: "a.docx"},
		{reqID: "REQ-02", docSource: "d.docx"},
	}, got)

	nhieu := make([]testcaseRAGItem, 0, 20)
	for i := 0; i < 20; i++ {
		nhieu = append(nhieu, testcaseRAGItem{ReqID: fmt.Sprintf("REQ-%02d", i)})
	}
	require.Len(t, distinctRequirements(nhieu), testcaseRTMLastRow-testcaseRTMFirstRow+1)
}
