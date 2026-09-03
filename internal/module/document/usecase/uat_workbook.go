package usecase

import (
	"bytes"
	"fmt"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/quangdung93/docs-hub-api/internal/module/document/assets"
	"github.com/quangdung93/docs-hub-api/internal/module/document/domain"
)

const (
	uatSheetSummary = "Summary"
	uatSheetModule  = "Report 1_Module"
	uatSheetProcess = "Report2 _Process" // tên có khoảng trắng lệch — đúng như template
	uatFirstDataRow = 12                 // dòng đầu tiên dành cho test case trong Report 1_Module
	// uatProcessFirstDataRow: dòng 1 của Report2 _Process là tiêu đề cột, từ
	// dòng 2 trở đi là test case mẫu của template.
	uatProcessFirstDataRow = 2
)

type uatWorkbookInput struct {
	ProjectName, ProjectCode string
	ScopeLabel, ScopeKind    string
	PO, PM                   string
	AccountTest, ScopeTest   string
	StartDate, DueDate       *time.Time
	Items                    []domain.UATItem
}

// buildUATWorkbook mở template UAT Report chuẩn ISC đã nhúng sẵn, dọn dữ liệu
// mẫu, điền phần hành chính (Summary) và danh sách tài liệu (Report 1_Module)
// rồi trả bytes xlsx hoàn chỉnh. Cover và Guideline giữ nguyên như template.
func buildUATWorkbook(in uatWorkbookInput) ([]byte, error) {
	f, err := excelize.OpenReader(bytes.NewReader(assets.UATReportXLSX))
	if err != nil {
		return nil, fmt.Errorf("mở template UAT: %w", err)
	}
	defer f.Close()

	// Dọn TRƯỚC khi điền: dọn sau sẽ xoá luôn dữ liệu vừa ghi.
	if err := clearUATSampleData(f); err != nil {
		return nil, err
	}
	if err := fillUATSummary(f, in); err != nil {
		return nil, err
	}
	if err := fillUATModuleRows(f, in.Items); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("ghi file UAT: %w", err)
	}
	return buf.Bytes(), nil
}

// fillUATSummary điền các ô nhập liệu ở sheet Summary: D3 (project - scope),
// F3 (khoảng ngày), D4 (PO), F4 (scope test/module, merge F4:F6), D5 (PM),
// D6 (account test) — theo đúng layout label(C/E)/value(D/F) của template.
func fillUATSummary(f *excelize.File, in uatWorkbookInput) error {
	title := in.ProjectName
	if in.ScopeLabel != "" {
		title = fmt.Sprintf("%s - %s", in.ProjectName, in.ScopeLabel)
	}
	scopeTest := in.ScopeTest
	if scopeTest == "" {
		scopeTest = in.ScopeLabel
	}
	values := map[string]string{
		"D3": title,
		"F3": fmt.Sprintf("%s - %s", formatUATDate(in.StartDate), formatUATDate(in.DueDate)),
		"D4": in.PO,
		"F4": scopeTest,
		"D5": in.PM,
		"D6": in.AccountTest,
	}
	// Ghi CẢ khi giá trị rỗng. Trước đây bỏ qua ô rỗng nên D6 giữ nguyên dữ
	// liệu mẫu của template ("1. HuongTTT38 - ISC") mỗi khi người dùng không
	// truyền account_test — báo cáo xuất ra mang tên người của dự án khác.
	// D4/D5 không lộ chuyện này chỉ vì template vốn để trống hai ô đó.
	for _, cell := range []string{"D3", "F3", "D4", "F4", "D5", "D6"} {
		if err := f.SetCellValue(uatSheetSummary, cell, values[cell]); err != nil {
			return fmt.Errorf("điền sheet Summary ô %s: %w", cell, err)
		}
	}
	return nil
}

// clearUATSampleData xoá dữ liệu mẫu mà template chuẩn ISC mang sẵn, để báo
// cáo xuất ra không lẫn thông tin của dự án khác. Chỉ xoá phần DỮ LIỆU — nhãn
// cột, tiêu đề, công thức thống kê và sheet Guideline giữ nguyên.
//
// Sheet Report2 _Process vẫn được giữ lại (Guideline mục 3.4 cho phép
// Hide/Delete sheet Report không dùng, nhưng đó là bước tinh gọn tính sau).
func clearUATSampleData(f *excelize.File) error {
	// K4/K5 là ngày test Round 1 của dự án mẫu (16/10/2025 - 20/10/2025), lưu
	// dạng số serial nên nhìn qua tưởng mã số — dễ bị đọc nhầm là ngày thật.
	for _, cell := range []string{"K4", "K5"} {
		if err := f.SetCellValue(uatSheetModule, cell, ""); err != nil {
			return fmt.Errorf("dọn dữ liệu mẫu %s!%s: %w", uatSheetModule, cell, err)
		}
	}
	rows, err := f.GetRows(uatSheetProcess)
	if err != nil {
		return fmt.Errorf("đọc sheet %s: %w", uatSheetProcess, err)
	}
	for i, row := range rows {
		if i+1 < uatProcessFirstDataRow {
			continue // dòng tiêu đề cột
		}
		for j := range row {
			cell, nameErr := excelize.CoordinatesToCellName(j+1, i+1)
			if nameErr != nil {
				return fmt.Errorf("tính ô sheet %s: %w", uatSheetProcess, nameErr)
			}
			if err := f.SetCellValue(uatSheetProcess, cell, ""); err != nil {
				return fmt.Errorf("dọn dữ liệu mẫu %s!%s: %w", uatSheetProcess, cell, err)
			}
		}
	}
	return nil
}

// fillUATModuleRows điền mỗi tài liệu thành 1 dòng test case: NO./MODULE/
// STEPS TO EXECUTE/EXPECTED RESULT. Cột STATUS/DESCRIPTION/ACTION/NOTE của
// từng round bỏ trống — đây là phần QA điền tay sau khi test thật.
func fillUATModuleRows(f *excelize.File, items []domain.UATItem) error {
	for i, item := range items {
		row := uatFirstDataRow + i
		cells := map[string]any{
			fmt.Sprintf("B%d", row): i + 1,
			fmt.Sprintf("C%d", row): item.Title,
			fmt.Sprintf("D%d", row): uatStepsText(item),
			fmt.Sprintf("G%d", row): uatExpectedText,
		}
		for _, col := range []string{"B", "C", "D", "G"} {
			cell := fmt.Sprintf("%s%d", col, row)
			if err := f.SetCellValue(uatSheetModule, cell, cells[cell]); err != nil {
				return fmt.Errorf("điền sheet %s dòng %d: %w", uatSheetModule, row, err)
			}
		}
	}
	return nil
}

// uatStepsText và uatExpectedText là nội dung test case dùng chung giữa bản
// xlsx (sheet Report 1_Module) và bản PDF, để hai định dạng không lệch chữ.
func uatStepsText(item domain.UATItem) string {
	return fmt.Sprintf(
		"Mở tài liệu \"%s\" (revision #%d, file %s) trong Document Hub và đối chiếu nội dung với bản đã duyệt.",
		item.Title, item.RevisionNo, item.FileName,
	)
}

const uatExpectedText = "Nội dung khớp với bản ghi trong hệ thống, ingest thành công (status=ready), " +
	"không có sai lệch nghiệp vụ so với bản đã duyệt."

func formatUATDate(t *time.Time) string {
	if t == nil {
		return "TBD"
	}
	return t.Format("02/01/2006")
}
