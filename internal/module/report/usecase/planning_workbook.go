package usecase

import (
	"bytes"
	"fmt"

	"github.com/xuri/excelize/v2"

	"github.com/quangdung93/docs-hub-api/internal/module/report/assets"
	"github.com/quangdung93/docs-hub-api/internal/module/report/domain"
)

const planningSheetPlan = "Plan"

type planningContentInput struct {
	ProjectName string
	Milestones  []planningRAGMilestone
}

// renderPlanningContent sinh nội dung file theo định dạng đã chuẩn hoá —
// milestones là kế hoạch do RAGFlow tổng hợp từ tài liệu dự án.
func renderPlanningContent(format string, in planningContentInput) ([]byte, string, error) {
	if format == domain.FormatPDF {
		content, err := buildPlanningPDF(in)
		return content, contentTypePDF, err
	}
	content, err := buildPlanningWorkbook(in)
	return content, contentTypeXLSX, err
}

// buildPlanningWorkbook mở template Project Plan chuẩn ISC đã nhúng sẵn, điền
// tên dự án và tối đa 5 milestone (mỗi milestone tối đa 7/3/3/3/3 task — số
// dòng BỊ KHÓA cứng bởi công thức MIN/MAX/SUM có sẵn trong sheet "Plan", xem
// planningMilestoneSlots). Milestone/task vượt giới hạn slot bị cắt bớt.
func buildPlanningWorkbook(in planningContentInput) ([]byte, error) {
	f, err := excelize.OpenReader(bytes.NewReader(assets.ProjectPlanXLSX))
	if err != nil {
		return nil, fmt.Errorf("mở template Project Plan: %w", err)
	}
	defer f.Close()

	if in.ProjectName != "" {
		if err := f.SetCellValue(planningSheetPlan, "B1",
			fmt.Sprintf("[%s] Project Planning", in.ProjectName)); err != nil {
			return nil, fmt.Errorf("điền sheet Plan ô B1: %w", err)
		}
	}
	if err := fillPlanningMilestones(f, in.Milestones); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("ghi file Project Plan: %w", err)
	}
	return buf.Bytes(), nil
}

// fillPlanningMilestones điền tên milestone (cột B, dòng header) và task con
// (cột C=tên, D=mô tả). Cột PIC/%DONE/START/DUE/EFFORT/NOTE để trống — PM điền
// tay sau, giống cách UAT/Testcase report để trống cột QA điền tay.
func fillPlanningMilestones(f *excelize.File, milestones []planningRAGMilestone) error {
	roman := []string{"I", "II", "III", "IV", "V"}
	for i, slot := range planningMilestoneSlots {
		if i >= len(milestones) {
			break
		}
		milestone := milestones[i]
		header := fmt.Sprintf("B%d", slot.headerRow)
		label := fmt.Sprintf("%s. %s", roman[i], milestone.Name)
		if err := f.SetCellValue(planningSheetPlan, header, label); err != nil {
			return fmt.Errorf("điền milestone header %s: %w", header, err)
		}
		tasks := milestone.Tasks
		if len(tasks) > slot.maxTasks {
			tasks = tasks[:slot.maxTasks]
		}
		for j, task := range tasks {
			row := slot.firstTaskRow + j
			if err := f.SetCellValue(planningSheetPlan, fmt.Sprintf("C%d", row), task.Name); err != nil {
				return fmt.Errorf("điền task tên dòng %d: %w", row, err)
			}
			if err := f.SetCellValue(planningSheetPlan, fmt.Sprintf("D%d", row), task.Description); err != nil {
				return fmt.Errorf("điền task mô tả dòng %d: %w", row, err)
			}
		}
	}
	return nil
}
