package usecase

import (
	"bytes"
	"fmt"

	"github.com/go-pdf/fpdf"

	"github.com/quangdung93/docs-hub-api/internal/module/report/assets"
)

// buildPlanningPDF dựng bản PDF tương đương bản xlsx: tiêu đề + tên dự án rồi
// từng milestone kèm danh sách task (PDF không có sẵn layout bảng như template
// xlsx nên trình bày theo khối cho dễ đọc).
func buildPlanningPDF(in planningContentInput) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddUTF8FontFromBytes(uatPDFFontFamily, "", assets.PDFFontRegular)
	pdf.AddUTF8FontFromBytes(uatPDFFontFamily, "B", assets.PDFFontBold)
	pdf.SetMargins(15, 15, 15)
	pdf.SetAutoPageBreak(true, 15)
	pdf.AddPage()

	pdf.SetFont(uatPDFFontFamily, "B", 16)
	pdf.CellFormat(uatPDFWidth, 10, "PROJECT PLANNING", "", 1, "C", false, 0, "")
	pdf.SetFont(uatPDFFontFamily, "", 9)
	pdf.CellFormat(uatPDFWidth, 6, "Mẫu 3.0-BM/PM/HDCV/FTEL, phiên bản 3.0", "", 1, "C", false, 0, "")
	pdf.Ln(4)

	if in.ProjectName != "" {
		writeUATField(pdf, "Dự án", in.ProjectName)
	}

	roman := []string{"I", "II", "III", "IV", "V"}
	for i, milestone := range in.Milestones {
		if i >= maxPlanningMilestones {
			break
		}
		pdf.Ln(3)
		pdf.SetFont(uatPDFFontFamily, "B", 12)
		label := milestone.Name
		if i < len(roman) {
			label = fmt.Sprintf("%s. %s", roman[i], milestone.Name)
		}
		pdf.CellFormat(uatPDFWidth, 8, label, "", 1, "L", false, 0, "")
		pdf.SetDrawColor(180, 180, 180)
		pdf.Line(pdf.GetX(), pdf.GetY(), pdf.GetX()+uatPDFWidth, pdf.GetY())
		pdf.Ln(2)

		maxTasks := planningMilestoneSlots[min(i, len(planningMilestoneSlots)-1)].maxTasks
		tasks := milestone.Tasks
		if len(tasks) > maxTasks {
			tasks = tasks[:maxTasks]
		}
		for j, task := range tasks {
			pdf.SetFont(uatPDFFontFamily, "B", 10)
			pdf.MultiCell(uatPDFWidth, 6, fmt.Sprintf("%d. %s", j+1, task.Name), "", "L", false)
			pdf.SetFont(uatPDFFontFamily, "", 9)
			pdf.MultiCell(uatPDFWidth, 5, task.Description, "", "L", false)
			pdf.Ln(2)
		}
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("ghi file Project Plan PDF: %w", err)
	}
	return buf.Bytes(), nil
}
