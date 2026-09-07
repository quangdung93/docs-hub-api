package usecase

import (
	"bytes"
	"fmt"

	"github.com/xuri/excelize/v2"

	"github.com/quangdung93/docs-hub-api/internal/module/report/assets"
	"github.com/quangdung93/docs-hub-api/internal/module/report/domain"
)

const (
	testcaseSheetSummary  = "Summary"
	testcaseSheetCases    = "Test Cases"
	testcaseSheetSample2  = "Test Case 2" // sheet mẫu có sẵn dữ liệu ví dụ, cần dọn để Dashboard không đếm nhầm
	testcaseFirstDataRow  = 7
	testcaseSampleLastRow = 9 // dòng ví dụ có sẵn trong template gốc ở "Test Case 2"
)

type testcaseContentInput struct {
	ProjectName string
	Items       []testcaseRAGItem
}

// renderTestcaseContent sinh nội dung file theo định dạng đã chuẩn hoá — items
// là test case do RAGFlow tổng hợp từ User Story/AC trong tài liệu dự án.
func renderTestcaseContent(format string, in testcaseContentInput) ([]byte, string, error) {
	if format == domain.FormatPDF {
		content, err := buildTestcasePDF(in)
		return content, contentTypePDF, err
	}
	content, err := buildTestcaseWorkbook(in)
	return content, contentTypeXLSX, err
}

// buildTestcaseWorkbook mở template Testcase Report chuẩn ISC (SDLC) đã nhúng
// sẵn, điền tên dự án (Summary) và danh sách test case (sheet "Test Cases").
// Cột A (Testcase ID) là công thức có sẵn trong template (tự sinh từ cột C
// DOC Source) — không đụng vào. Sheet "Test Case 2" là dữ liệu MẪU có sẵn
// trong template gốc, dọn sạch để Dashboard (đếm qua INDIRECT theo tên sheet)
// không cộng nhầm test case ảo vào báo cáo thật.
func buildTestcaseWorkbook(in testcaseContentInput) ([]byte, error) {
	f, err := excelize.OpenReader(bytes.NewReader(assets.TestcaseReportXLSX))
	if err != nil {
		return nil, fmt.Errorf("mở template Testcase Report: %w", err)
	}
	defer f.Close()

	if in.ProjectName != "" {
		if err := f.SetCellValue(testcaseSheetSummary, "C6", in.ProjectName); err != nil {
			return nil, fmt.Errorf("điền sheet Summary ô C6: %w", err)
		}
		if err := f.SetCellValue(testcaseSheetCases, "C3", in.ProjectName); err != nil {
			return nil, fmt.Errorf("điền sheet %s ô C3: %w", testcaseSheetCases, err)
		}
	}
	// Cả "Test Cases" lẫn "Test Case 2" đều có sẵn dữ liệu MẪU ở dòng 7-9 trong
	// template gốc — phải dọn cả hai trước khi điền, nếu không phần dư (cột J+
	// hoặc các dòng vượt quá số item thật) sẽ lẫn dữ liệu mẫu vào báo cáo thật.
	if err := clearTestcaseSampleRows(f, testcaseSheetCases); err != nil {
		return nil, err
	}
	if err := clearTestcaseSampleRows(f, testcaseSheetSample2); err != nil {
		return nil, err
	}
	if err := fillTestcaseRows(f, in.Items); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("ghi file Testcase Report: %w", err)
	}
	return buf.Bytes(), nil
}

// fillTestcaseRows điền mỗi test case thành 1 dòng bắt đầu từ dòng 7 (B..I) —
// nội dung THẬT do RAGFlow sinh. Cột J trở đi (Origin/Review/Automated/Script/
// Vibe-test/KQ Script/Kết quả/Executed By/ID Bugs) bỏ trống — QA điền tay sau
// khi test thật, giống UAT report.
func fillTestcaseRows(f *excelize.File, items []testcaseRAGItem) error {
	for i, item := range items {
		row := testcaseFirstDataRow + i
		cells := map[string]string{
			fmt.Sprintf("B%d", row): item.ReqID,
			fmt.Sprintf("C%d", row): item.DocSource,
			fmt.Sprintf("D%d", row): item.Group,
			fmt.Sprintf("E%d", row): item.Priority,
			fmt.Sprintf("F%d", row): item.Title,
			fmt.Sprintf("G%d", row): item.Precondition,
			fmt.Sprintf("H%d", row): item.Steps,
			fmt.Sprintf("I%d", row): item.Expected,
		}
		for _, col := range []string{"B", "C", "D", "E", "F", "G", "H", "I"} {
			cell := fmt.Sprintf("%s%d", col, row)
			if err := f.SetCellValue(testcaseSheetCases, cell, cells[cell]); err != nil {
				return fmt.Errorf("điền sheet %s dòng %d: %w", testcaseSheetCases, row, err)
			}
		}
	}
	return nil
}

// clearTestcaseSampleRows xóa dữ liệu ví dụ có sẵn (B..T, dòng 7-9) trong
// template gốc của sheet đã cho — không thể xóa hẳn sheet "Test Case 2" (vẫn
// được Dashboard tham chiếu qua INDIRECT theo tên sheet, xóa sheet sẽ làm
// hỏng công thức #REF!). Cột A (Testcase ID) là công thức, không đụng vào.
func clearTestcaseSampleRows(f *excelize.File, sheet string) error {
	cols := []string{"B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M", "N", "O", "P", "Q", "R", "S", "T"}
	for row := testcaseFirstDataRow; row <= testcaseSampleLastRow; row++ {
		for _, col := range cols {
			if err := f.SetCellValue(sheet, fmt.Sprintf("%s%d", col, row), ""); err != nil {
				return fmt.Errorf("dọn dữ liệu mẫu sheet %s dòng %d: %w", sheet, row, err)
			}
		}
	}
	return nil
}
