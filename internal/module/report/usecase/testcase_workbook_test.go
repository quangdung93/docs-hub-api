package usecase

import (
	"bytes"
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
