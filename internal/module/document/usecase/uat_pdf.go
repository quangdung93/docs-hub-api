package usecase

import (
	"bytes"
	"fmt"

	"github.com/go-pdf/fpdf"

	uattemplate "github.com/quangdung93/docs-hub-api/template"
)

const (
	uatPDFFontFamily = "DejaVu"
	uatPDFWidth      = 180.0
)

// buildUATPDF dựng bản PDF tương đương bản xlsx: khối hành chính (project/
// scope/PO/PM/account test) rồi danh sách test case dạng văn xuôi (PDF không
// có sẵn layout bảng như template xlsx nên trình bày theo khối cho dễ đọc).
func buildUATPDF(in uatWorkbookInput) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	// Font registry nằm trên từng *Fpdf instance, không dùng chung được giữa
	// các request — phải đăng ký lại (không nặng: chỉ parse bytes đã nhúng sẵn).
	pdf.AddUTF8FontFromBytes(uatPDFFontFamily, "", uattemplate.PDFFontRegular)
	pdf.AddUTF8FontFromBytes(uatPDFFontFamily, "B", uattemplate.PDFFontBold)
	pdf.SetMargins(15, 15, 15)
	pdf.SetAutoPageBreak(true, 15)
	pdf.AddPage()

	pdf.SetFont(uatPDFFontFamily, "B", 16)
	pdf.CellFormat(uatPDFWidth, 10, "UAT REPORT", "", 1, "C", false, 0, "")
	pdf.SetFont(uatPDFFontFamily, "", 9)
	pdf.CellFormat(uatPDFWidth, 6, "Mẫu 4.0-BM/PM/HDCV/FTEL, phiên bản 2.0", "", 1, "C", false, 0, "")
	pdf.Ln(4)

	title := in.ProjectName
	if in.ScopeLabel != "" {
		title = fmt.Sprintf("%s - %s", in.ProjectName, in.ScopeLabel)
	}
	writeUATField(pdf, "Dự án - Phạm vi", title)
	writeUATField(pdf, "Thời gian",
		fmt.Sprintf("%s - %s", formatUATDate(in.StartDate), formatUATDate(in.DueDate)))
	if in.PO != "" {
		writeUATField(pdf, "PO - Product Owner", in.PO)
	}
	if in.PM != "" {
		writeUATField(pdf, "PM - Project Manager", in.PM)
	}
	if in.AccountTest != "" {
		writeUATField(pdf, "Account Test", in.AccountTest)
	}
	scopeTest := in.ScopeTest
	if scopeTest == "" {
		scopeTest = in.ScopeLabel
	}
	if scopeTest != "" {
		writeUATField(pdf, "Scope Test/Module", scopeTest)
	}

	pdf.Ln(4)
	pdf.SetFont(uatPDFFontFamily, "B", 12)
	pdf.CellFormat(uatPDFWidth, 8, "DANH SÁCH TEST CASE", "", 1, "L", false, 0, "")
	pdf.SetDrawColor(180, 180, 180)
	pdf.Line(pdf.GetX(), pdf.GetY(), pdf.GetX()+uatPDFWidth, pdf.GetY())
	pdf.Ln(3)

	for i, item := range in.Items {
		pdf.SetFont(uatPDFFontFamily, "B", 10)
		pdf.MultiCell(uatPDFWidth, 6, fmt.Sprintf("%d. %s", i+1, item.Title), "", "L", false)
		pdf.SetFont(uatPDFFontFamily, "", 9)
		pdf.MultiCell(uatPDFWidth, 5, "Bước thực hiện: "+uatStepsText(item), "", "L", false)
		pdf.MultiCell(uatPDFWidth, 5, "Kết quả mong đợi: "+uatExpectedText, "", "L", false)
		pdf.Ln(3)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("ghi file UAT PDF: %w", err)
	}
	return buf.Bytes(), nil
}

func writeUATField(pdf *fpdf.Fpdf, label, value string) {
	pdf.SetFont(uatPDFFontFamily, "B", 9)
	pdf.CellFormat(45, 6, label+":", "", 0, "L", false, 0, "")
	pdf.SetFont(uatPDFFontFamily, "", 9)
	pdf.MultiCell(uatPDFWidth-45, 6, value, "", "L", false)
}
