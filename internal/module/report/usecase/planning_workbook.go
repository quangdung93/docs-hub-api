package usecase

import (
	"bytes"
	"fmt"

	"github.com/xuri/excelize/v2"

	"github.com/quangdung93/docs-hub-api/internal/module/report/assets"
	"github.com/quangdung93/docs-hub-api/internal/module/report/domain"
)

const (
	planningSheetPlan      = "Plan"
	planningSheetRisk      = "Risk"
	planningSheetObjective = "Objective (opt)"
	planningSheetOrgChart  = "Org Chart & Communication (opt)"
	planningSheetRevision  = "Revision history"
)

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

// buildPlanningWorkbook mở template Project Plan chuẩn ISC đã nhúng sẵn, dọn dữ
// liệu của dự án mẫu, điền tên dự án và tối đa 5 milestone (mỗi milestone tối đa
// 7/3/3/3/3 task — số dòng BỊ KHÓA cứng bởi công thức MIN/MAX/SUM có sẵn trong
// sheet "Plan", xem planningMilestoneSlots). Milestone/task vượt slot bị cắt bớt.
func buildPlanningWorkbook(in planningContentInput) ([]byte, error) {
	f, err := excelize.OpenReader(bytes.NewReader(assets.ProjectPlanXLSX))
	if err != nil {
		return nil, fmt.Errorf("mở template Project Plan: %w", err)
	}
	defer f.Close()

	if err := clearPlanningSampleData(f); err != nil {
		return nil, err
	}
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

// clearPlanningSampleData dọn dữ liệu của dự án mẫu trong template Project Plan.
// Code chỉ điền sheet "Plan", bốn sheet còn lại giữ nguyên nội dung template nên
// đây là loại báo cáo lộ nhiều nhất trong ba loại.
func clearPlanningSampleData(f *excelize.File) error {
	// Risk: 7 dòng rủi ro ví dụ — chính template ghi ở ô B14 rằng đây là ví dụ
	// tham khảo. Cột H (TOTAL SCORE = LS*IS) là công thức nên tự được bỏ qua.
	if err := clearSampleBlock(f, planningSheetRisk, "B", "H", 16, 22); err != nil {
		return err
	}
	// Objective: cột I/J là lý do đặt target và điều kiện riêng của dự án mẫu.
	// Cột B..H là bộ chỉ số chuẩn ISC (PCV/CSAT/SPI/CPI) — khung biểu mẫu, giữ.
	if err := clearSampleBlock(f, planningSheetObjective, "I", "J", 5, 8); err != nil {
		return err
	}
	// Org Chart: cột D của khối Stakeholders là đơn vị của dự án mẫu (FTQ, CSOC).
	// Bảng nhân sự phía trên là khung vai trò chuẩn ISC, giữ nguyên.
	if err := clearSampleBlock(f, planningSheetOrgChart, "D", "D", 16, 19); err != nil {
		return err
	}
	// Revision history: dòng lịch sử phiên bản của dự án mẫu.
	return clearSampleCells(f, planningSheetRevision, "G5")
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
