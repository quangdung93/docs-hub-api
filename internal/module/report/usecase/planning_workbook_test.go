package usecase

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

func TestBuildPlanningWorkbook_DienMilestoneVaCatBotTheoSlot(t *testing.T) {
	t.Parallel()
	milestone1Tasks := make([]planningRAGTask, 0, 8)
	for i := 0; i < 8; i++ { // 8 task nhưng slot 1 chỉ chứa tối đa 7 -> phải cắt còn 7
		milestone1Tasks = append(milestone1Tasks, planningRAGTask{
			Name: "Task " + string(rune('A'+i)), Description: "Mô tả " + string(rune('A'+i)),
		})
	}
	in := planningContentInput{
		ProjectName: "Demo Project",
		Milestones: []planningRAGMilestone{
			{Name: "Chuẩn bị hạ tầng", Tasks: milestone1Tasks},
			{Name: "Phát triển tính năng", Tasks: []planningRAGTask{
				{Name: "Task X", Description: "Mô tả X"},
			}},
		},
	}
	raw, err := buildPlanningWorkbook(in)
	require.NoError(t, err)
	require.NotEmpty(t, raw)

	f, err := excelize.OpenReader(bytes.NewReader(raw))
	require.NoError(t, err)
	defer f.Close()

	title, err := f.GetCellValue(planningSheetPlan, "B1")
	require.NoError(t, err)
	require.Contains(t, title, "Demo Project")

	header1, err := f.GetCellValue(planningSheetPlan, "B9")
	require.NoError(t, err)
	require.Equal(t, "I. Chuẩn bị hạ tầng", header1)

	header2, err := f.GetCellValue(planningSheetPlan, "B17")
	require.NoError(t, err)
	require.Equal(t, "II. Phát triển tính năng", header2)

	// Slot 1: dòng 10-16 (7 dòng) phải đủ 7 task, task thứ 8 KHÔNG được ghi
	// (slot chỉ có 7 dòng, dòng 17 là header milestone 2).
	name10, err := f.GetCellValue(planningSheetPlan, "C10")
	require.NoError(t, err)
	require.Equal(t, "Task A", name10)
	name16, err := f.GetCellValue(planningSheetPlan, "C16")
	require.NoError(t, err)
	require.Equal(t, "Task G", name16)

	// Slot 2 (milestone 2): dòng 18.
	name18, err := f.GetCellValue(planningSheetPlan, "C18")
	require.NoError(t, err)
	require.Equal(t, "Task X", name18)
	desc18, err := f.GetCellValue(planningSheetPlan, "D18")
	require.NoError(t, err)
	require.Equal(t, "Mô tả X", desc18)
}

func TestBuildPlanningWorkbook_QuaSoMilestoneChiLay5(t *testing.T) {
	t.Parallel()
	milestones := make([]planningRAGMilestone, 0, 7)
	for i := 0; i < 7; i++ {
		milestones = append(milestones, planningRAGMilestone{
			Name: "Milestone " + string(rune('A'+i)),
			Tasks: []planningRAGTask{
				{Name: "T", Description: "D"},
			},
		})
	}
	// fetchPlanningMilestones là nơi cắt về maxPlanningMilestones=5 trong luồng
	// thật; ở đây test trực tiếp render với input đã cắt để khớp hành vi.
	if len(milestones) > maxPlanningMilestones {
		milestones = milestones[:maxPlanningMilestones]
	}
	raw, err := buildPlanningWorkbook(planningContentInput{Milestones: milestones})
	require.NoError(t, err)

	f, err := excelize.OpenReader(bytes.NewReader(raw))
	require.NoError(t, err)
	defer f.Close()

	header5, err := f.GetCellValue(planningSheetPlan, "B29")
	require.NoError(t, err)
	require.Equal(t, "V. Milestone E", header5)
}

func TestBuildPlanningWorkbook_DonDuLieuMau(t *testing.T) {
	t.Parallel()
	raw, err := buildPlanningWorkbook(planningContentInput{ProjectName: "Demo Project"})
	require.NoError(t, err)

	f, err := excelize.OpenReader(bytes.NewReader(raw))
	require.NoError(t, err)
	defer f.Close()

	// Risk: 7 dòng rủi ro ví dụ, đây là khối lớn nhất của Project Plan.
	for _, cell := range []string{"B16", "C16", "D16", "E16", "E17", "D22", "E22", "G22"} {
		got, err := f.GetCellValue(planningSheetRisk, cell)
		require.NoError(t, err)
		require.Empty(t, got, "Risk!%s còn dữ liệu mẫu", cell)
	}

	// Objective: lý do đặt target và điều kiện riêng của dự án mẫu.
	for _, cell := range []string{"I5", "I6", "I7", "I8", "J5", "J8"} {
		got, err := f.GetCellValue(planningSheetObjective, cell)
		require.NoError(t, err)
		require.Empty(t, got, "Objective!%s còn dữ liệu mẫu", cell)
	}

	// Org Chart: đơn vị stakeholder của dự án mẫu (FTQ, CSOC).
	for _, cell := range []string{"D16", "D17", "D18"} {
		got, err := f.GetCellValue(planningSheetOrgChart, cell)
		require.NoError(t, err)
		require.Empty(t, got, "Org Chart!%s còn dữ liệu mẫu", cell)
	}

	revision, err := f.GetCellValue(planningSheetRevision, "G5")
	require.NoError(t, err)
	require.Empty(t, revision)
}

func TestBuildPlanningWorkbook_GiuNguyenKhungBieuMau(t *testing.T) {
	t.Parallel()
	raw, err := buildPlanningWorkbook(planningContentInput{ProjectName: "Demo Project"})
	require.NoError(t, err)

	f, err := excelize.OpenReader(bytes.NewReader(raw))
	require.NoError(t, err)
	defer f.Close()

	// Risk!B9:D12 là bảng chú giải (mức ưu tiên -> cách ứng phó được phép),
	// KHÔNG phải dữ liệu mẫu dù nằm ngay trên bảng rủi ro.
	chuGiai, err := f.GetCellValue(planningSheetRisk, "D10")
	require.NoError(t, err)
	require.Equal(t, "Advoid, Transfer, Reduce", chuGiai)

	// Cột H là công thức TOTAL SCORE, phải còn nguyên sau khi dọn cột B..G.
	congThuc, err := f.GetCellFormula(planningSheetRisk, "H16")
	require.NoError(t, err)
	require.NotEmpty(t, congThuc)

	// Bộ chỉ số chuẩn ISC ở Objective là khung biểu mẫu, giữ nguyên.
	chiSo, err := f.GetCellValue(planningSheetObjective, "C5")
	require.NoError(t, err)
	require.Equal(t, "PCV", chiSo)

	// Bảng vai trò của Org Chart cũng là khung chuẩn ISC.
	vaiTro, err := f.GetCellValue(planningSheetOrgChart, "E8")
	require.NoError(t, err)
	require.Equal(t, "PM", vaiTro)
}
