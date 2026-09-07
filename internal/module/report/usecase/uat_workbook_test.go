package usecase

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

func TestBuildUATWorkbook_DienNoiDungThat(t *testing.T) {
	t.Parallel()
	in := uatContentInput{
		ProjectName: "Demo Project",
		Items: []uatRAGItem{
			{Title: "Đăng nhập", Steps: "1. Nhập user/pass", Expected: "Vào được trang chủ"},
			{Title: "Đăng xuất", Steps: "Bấm Đăng xuất", Expected: "Về màn hình đăng nhập"},
		},
	}
	raw, err := buildUATWorkbook(in)
	require.NoError(t, err)
	require.NotEmpty(t, raw)

	f, err := excelize.OpenReader(bytes.NewReader(raw))
	require.NoError(t, err)
	defer f.Close()

	projectName, err := f.GetCellValue(uatSheetSummary, "D3")
	require.NoError(t, err)
	require.Equal(t, "Demo Project", projectName)

	title, err := f.GetCellValue(uatSheetModule, "C12")
	require.NoError(t, err)
	require.Equal(t, "Đăng nhập", title)
	expected, err := f.GetCellValue(uatSheetModule, "G13")
	require.NoError(t, err)
	require.Equal(t, "Về màn hình đăng nhập", expected)
}

func TestBuildUATWorkbook_DonDuLieuMau(t *testing.T) {
	t.Parallel()
	raw, err := buildUATWorkbook(uatContentInput{ProjectName: "Demo Project"})
	require.NoError(t, err)

	f, err := excelize.OpenReader(bytes.NewReader(raw))
	require.NoError(t, err)
	defer f.Close()

	// Summary: người test và kết quả nghiệm thu của dự án mẫu.
	// D10 nguy hiểm nhất — template chọn sẵn "ACCEPT UAT" nên báo cáo tự nhận PO
	// đã nghiệm thu dù chưa ai chạy test.
	for _, cell := range []string{"D6", "D10"} {
		got, err := f.GetCellValue(uatSheetSummary, cell)
		require.NoError(t, err)
		require.Empty(t, got, "Summary!%s còn dữ liệu mẫu", cell)
	}

	// Report 1_Module: ngày Round 1 của dự án mẫu (16/10/2025 và 20/10/2025).
	for _, cell := range []string{"K4", "K5"} {
		got, err := f.GetCellValue(uatSheetModule, cell)
		require.NoError(t, err)
		require.Empty(t, got, "Report 1_Module!%s còn dữ liệu mẫu", cell)
	}

	// Report2 _Process: quy trình nghiệp vụ dự án mẫu, gồm cả kết quả test.
	for _, cell := range []string{"A2", "B2", "B3", "C3", "D3", "E3", "F3", "G3", "G4"} {
		got, err := f.GetCellValue(uatSheetProcess, cell)
		require.NoError(t, err)
		require.Empty(t, got, "Report2 _Process!%s còn dữ liệu mẫu", cell)
	}
}

func TestBuildUATWorkbook_GiuNguyenNhanBieuMau(t *testing.T) {
	t.Parallel()
	raw, err := buildUATWorkbook(uatContentInput{ProjectName: "Demo Project"})
	require.NoError(t, err)

	f, err := excelize.OpenReader(bytes.NewReader(raw))
	require.NoError(t, err)
	defer f.Close()

	// "ISC FEEDBACK" là nhãn nhóm cột của từng round, KHÔNG phải dữ liệu mẫu.
	feedback, err := f.GetCellValue(uatSheetModule, "L10")
	require.NoError(t, err)
	require.Equal(t, "ISC FEEDBACK", feedback)

	// Tiêu đề cột và nhãn Start/End phải còn: chỉ giá trị ngày bị dọn.
	start, err := f.GetCellValue(uatSheetModule, "J4")
	require.NoError(t, err)
	require.Equal(t, "Start", start)

	// Dòng tiêu đề của Report2 _Process nằm ở dòng 1, không được đụng tới.
	header, err := f.GetCellValue(uatSheetProcess, "B1")
	require.NoError(t, err)
	require.Contains(t, header, "STEPS")
}
