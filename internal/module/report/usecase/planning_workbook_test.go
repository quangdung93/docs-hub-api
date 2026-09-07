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
