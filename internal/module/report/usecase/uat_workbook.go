package usecase

import (
	"bytes"
	"fmt"

	"github.com/xuri/excelize/v2"

	"github.com/quangdung93/docs-hub-api/internal/module/report/assets"
	"github.com/quangdung93/docs-hub-api/internal/module/report/domain"
)

const (
	uatSheetSummary = "Summary"
	uatSheetModule  = "Report 1_Module"
	uatSheetProcess = "Report2 _Process"
	uatFirstDataRow = 12 // dòng đầu tiên dành cho test case trong Report 1_Module
)

type uatContentInput struct {
	ProjectName string
	Items       []uatRAGItem
}

// renderUATContent sinh nội dung file theo định dạng đã chuẩn hoá — items là
// test case do RAGFlow tổng hợp từ User Story/AC trong tài liệu dự án.
func renderUATContent(format string, in uatContentInput) ([]byte, string, error) {
	if format == domain.FormatPDF {
		content, err := buildUATPDF(in)
		return content, contentTypePDF, err
	}
	content, err := buildUATWorkbook(in)
	return content, contentTypeXLSX, err
}

// buildUATWorkbook mở template UAT Report chuẩn ISC đã nhúng sẵn, dọn dữ liệu
// của dự án mẫu, điền tên dự án (Summary) và danh sách test case
// (Report 1_Module) rồi trả bytes xlsx hoàn chỉnh.
func buildUATWorkbook(in uatContentInput) ([]byte, error) {
	f, err := excelize.OpenReader(bytes.NewReader(assets.UATReportXLSX))
	if err != nil {
		return nil, fmt.Errorf("mở template UAT: %w", err)
	}
	defer f.Close()

	if err := clearUATSampleData(f); err != nil {
		return nil, err
	}
	if in.ProjectName != "" {
		if err := f.SetCellValue(uatSheetSummary, "D3", in.ProjectName); err != nil {
			return nil, fmt.Errorf("điền sheet Summary ô D3: %w", err)
		}
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

// clearUATSampleData dọn dữ liệu của dự án mẫu còn sót trong template UAT.
//
// Summary!D10 là ô đáng chú ý nhất: đó là danh sách chọn kết quả nghiệm thu
// (ACCEPT UAT / ACCEPT WITH CONDITIONS / NOT ACCEPT UAT) và template chọn sẵn
// "ACCEPT UAT". Để nguyên thì mọi báo cáo xuất ra đều tự nhận PO đã nghiệm thu,
// trong khi ô tên PO ngay bên cạnh còn trống và chưa ai chạy test.
func clearUATSampleData(f *excelize.File) error {
	// Summary: D6 người test của dự án mẫu, D10 kết quả nghiệm thu chọn sẵn.
	if err := clearSampleCells(f, uatSheetSummary, "D6", "D10"); err != nil {
		return err
	}
	// Report 1_Module: K4/K5 là ngày bắt đầu/kết thúc Round 1 của dự án mẫu.
	if err := clearSampleCells(f, uatSheetModule, "K4", "K5"); err != nil {
		return err
	}
	// Report2 _Process: 3 dòng quy trình nghiệp vụ của dự án mẫu, gồm cả kết quả
	// test PENDING/FAILED. Code không ghi gì vào sheet này nên phải dọn cả khối.
	return clearSampleBlock(f, uatSheetProcess, "A", "J", 2, 4)
}

// fillUATModuleRows điền mỗi test case thành 1 dòng: NO./MODULE/STEPS TO
// EXECUTE/EXPECTED RESULT — nội dung THẬT do RAGFlow sinh (khác UAT export cũ
// của module document, vốn điền text khung cố định vì chỉ có metadata document).
// Cột STATUS/DESCRIPTION/ACTION/NOTE của từng round bỏ trống — QA điền tay sau
// khi test thật.
func fillUATModuleRows(f *excelize.File, items []uatRAGItem) error {
	for i, item := range items {
		row := uatFirstDataRow + i
		cells := map[string]any{
			fmt.Sprintf("B%d", row): i + 1,
			fmt.Sprintf("C%d", row): item.Title,
			fmt.Sprintf("D%d", row): item.Steps,
			fmt.Sprintf("G%d", row): item.Expected,
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
