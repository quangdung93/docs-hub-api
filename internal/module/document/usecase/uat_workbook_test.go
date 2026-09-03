package usecase

import (
	"bytes"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"github.com/quangdung93/docs-hub-api/internal/module/document/domain"
)

func TestBuildUATWorkbook_FillsSummaryAndModuleRows(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	due := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	content, err := buildUATWorkbook(uatWorkbookInput{
		ProjectName: "Docs Hub", ProjectCode: "DHUB",
		ScopeLabel: "v1.2", ScopeKind: domain.ScopeKindVersion,
		PO: "Nguyễn Văn A", PM: "Trần Thị B",
		AccountTest: "1. huongttt38 - ISC", ScopeTest: "Toàn bộ tài liệu v1.2",
		StartDate: &start, DueDate: &due,
		Items: []domain.UATItem{
			{DocumentID: uuid.New(), Title: "URD v1.0", FileName: "urd.docx", RevisionNo: 2, Status: "ready"},
			{DocumentID: uuid.New(), Title: "SRS v1.0", FileName: "srs.docx", RevisionNo: 1, Status: "ready"},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, content)

	f, err := excelize.OpenReader(bytes.NewReader(content))
	require.NoError(t, err)
	defer f.Close()

	summaryTitle, err := f.GetCellValue(uatSheetSummary, "D3")
	require.NoError(t, err)
	require.Equal(t, "Docs Hub - v1.2", summaryTitle)

	dates, err := f.GetCellValue(uatSheetSummary, "F3")
	require.NoError(t, err)
	require.Equal(t, "01/09/2026 - 08/09/2026", dates)

	po, err := f.GetCellValue(uatSheetSummary, "D4")
	require.NoError(t, err)
	require.Equal(t, "Nguyễn Văn A", po)

	pm, err := f.GetCellValue(uatSheetSummary, "D5")
	require.NoError(t, err)
	require.Equal(t, "Trần Thị B", pm)

	module1, err := f.GetCellValue(uatSheetModule, "C12")
	require.NoError(t, err)
	require.Equal(t, "URD v1.0", module1)

	steps1, err := f.GetCellValue(uatSheetModule, "D12")
	require.NoError(t, err)
	require.Contains(t, steps1, "urd.docx")
	require.Contains(t, steps1, "revision #2")

	module2, err := f.GetCellValue(uatSheetModule, "C13")
	require.NoError(t, err)
	require.Equal(t, "SRS v1.0", module2)

	no2, err := f.GetCellValue(uatSheetModule, "B13")
	require.NoError(t, err)
	require.Equal(t, "2", no2)
}

// Template chuẩn ISC mang sẵn dữ liệu mẫu của một dự án khác. Không dọn thì
// báo cáo xuất ra lẫn thông tin người khác — mentor đã phản ánh chuyện này.
func TestBuildUATWorkbook_DonDuLieuMauCuaTemplate(t *testing.T) {
	f := moWorkbook(t, uatWorkbookInput{
		ProjectName: "Docs Hub",
		Items:       []domain.UATItem{{DocumentID: uuid.New(), Title: "URD", FileName: "urd.docx", RevisionNo: 1}},
	})
	defer f.Close()

	// D6 (Account Test): template có sẵn "1. HuongTTT38 - ISC". Không truyền
	// account_test thì phải TRỐNG, không được để lộ tên người của dự án khác.
	account, err := f.GetCellValue(uatSheetSummary, "D6")
	require.NoError(t, err)
	require.Empty(t, account, "D6 phải trống khi không truyền account_test")

	// K4/K5: ngày test Round 1 của dự án mẫu (16/10/2025 - 20/10/2025), lưu
	// dạng số serial nên nhìn qua tưởng mã số — dễ bị đọc nhầm là ngày thật.
	for _, cell := range []string{"K4", "K5"} {
		v, cellErr := f.GetCellValue(uatSheetModule, cell)
		require.NoError(t, cellErr)
		require.Empty(t, v, "%s phải trống, đây là ngày mẫu của template", cell)
	}

	// Report2 _Process: dọn test case mẫu nhưng GIỮ tiêu đề cột, vì sheet này
	// vẫn được giữ lại cho người dùng tự điền.
	tieuDe, err := f.GetCellValue(uatSheetProcess, "A1")
	require.NoError(t, err)
	require.Equal(t, "STT", tieuDe, "phải giữ nguyên tiêu đề cột")
	for _, cell := range []string{"B2", "B3", "C3", "D3", "E3", "F3", "G3", "G4"} {
		v, cellErr := f.GetCellValue(uatSheetProcess, cell)
		require.NoError(t, cellErr)
		require.Empty(t, v, "%s là test case mẫu, phải dọn", cell)
	}
}

// Dọn dữ liệu mẫu KHÔNG được đụng tới phần khung của biểu mẫu.
func TestBuildUATWorkbook_GiuNguyenKhungBieuMau(t *testing.T) {
	f := moWorkbook(t, uatWorkbookInput{
		ProjectName: "Docs Hub", AccountTest: "1. QA-ISC",
		Items: []domain.UATItem{{DocumentID: uuid.New(), Title: "URD", FileName: "urd.docx", RevisionNo: 1}},
	})
	defer f.Close()

	require.Equal(t, []string{"Cover", "Summary", "Report 1_Module", "Report2 _Process", "Guideline"},
		f.GetSheetList(), "không được thêm hay bớt sheet nào")

	// Công thức thống kê phải còn nguyên — dọn ô mà xoá nhầm là hỏng báo cáo.
	congThuc, err := f.GetCellFormula(uatSheetModule, "E4")
	require.NoError(t, err)
	require.Contains(t, congThuc, "COUNTIF")

	// Truyền account_test thì D6 mang giá trị người dùng, không phải mẫu.
	account, err := f.GetCellValue(uatSheetSummary, "D6")
	require.NoError(t, err)
	require.Equal(t, "1. QA-ISC", account)

	// Guideline là hướng dẫn chính thức của ISC, không được dọn.
	huongDan, err := f.GetCellValue("Guideline", "C2")
	require.NoError(t, err)
	require.Contains(t, huongDan, "Hướng dẫn sử dụng Template UAT")
}

func moWorkbook(t *testing.T, in uatWorkbookInput) *excelize.File {
	t.Helper()
	content, err := buildUATWorkbook(in)
	require.NoError(t, err)
	f, err := excelize.OpenReader(bytes.NewReader(content))
	require.NoError(t, err)
	return f
}

func TestFormatUATDate_NilIsTBD(t *testing.T) {
	require.Equal(t, "TBD", formatUATDate(nil))
	d := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	require.Equal(t, "05/01/2026", formatUATDate(&d))
}
