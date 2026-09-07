package usecase

import (
	"bytes"
	"fmt"

	"github.com/go-pdf/fpdf"

	"github.com/quangdung93/docs-hub-api/internal/module/report/assets"
)

// buildTestcasePDF dựng bản PDF tương đương bản xlsx: tiêu đề + tên dự án rồi
// danh sách test case dạng văn xuôi.
func buildTestcasePDF(in testcaseContentInput) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddUTF8FontFromBytes(uatPDFFontFamily, "", assets.PDFFontRegular)
	pdf.AddUTF8FontFromBytes(uatPDFFontFamily, "B", assets.PDFFontBold)
	pdf.SetMargins(15, 15, 15)
	pdf.SetAutoPageBreak(true, 15)
	pdf.AddPage()

	pdf.SetFont(uatPDFFontFamily, "B", 16)
	pdf.CellFormat(uatPDFWidth, 10, "TESTCASE REPORT", "", 1, "C", false, 0, "")
	pdf.SetFont(uatPDFFontFamily, "", 9)
	pdf.CellFormat(uatPDFWidth, 6, "Mẫu ISC SDLC Testcase Report", "", 1, "C", false, 0, "")
	pdf.Ln(4)

	if in.ProjectName != "" {
		writeUATField(pdf, "Dự án", in.ProjectName)
	}

	pdf.Ln(4)
	pdf.SetFont(uatPDFFontFamily, "B", 12)
	pdf.CellFormat(uatPDFWidth, 8, "DANH SÁCH TEST CASE", "", 1, "L", false, 0, "")
	pdf.SetDrawColor(180, 180, 180)
	pdf.Line(pdf.GetX(), pdf.GetY(), pdf.GetX()+uatPDFWidth, pdf.GetY())
	pdf.Ln(3)

	for i, item := range in.Items {
		pdf.SetFont(uatPDFFontFamily, "B", 10)
		title := item.Title
		if item.Priority != "" {
			title = fmt.Sprintf("%s [%s]", title, item.Priority)
		}
		pdf.MultiCell(uatPDFWidth, 6, fmt.Sprintf("%d. %s", i+1, title), "", "L", false)
		pdf.SetFont(uatPDFFontFamily, "", 9)
		if item.Precondition != "" {
			pdf.MultiCell(uatPDFWidth, 5, "Tiền đề: "+item.Precondition, "", "L", false)
		}
		pdf.MultiCell(uatPDFWidth, 5, "Bước thực hiện: "+item.Steps, "", "L", false)
		pdf.MultiCell(uatPDFWidth, 5, "Kết quả mong đợi: "+item.Expected, "", "L", false)
		pdf.Ln(3)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("ghi file Testcase Report PDF: %w", err)
	}
	return buf.Bytes(), nil
}
