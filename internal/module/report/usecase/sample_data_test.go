package usecase

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"github.com/quangdung93/docs-hub-api/internal/module/report/assets"
)

func TestClearSampleCells_BoQuaOCongThuc(t *testing.T) {
	t.Parallel()
	f, err := excelize.OpenReader(bytes.NewReader(assets.ProjectPlanXLSX))
	require.NoError(t, err)
	defer f.Close()

	// Risk!H16 = TOTAL SCORE (LS*IS) — công thức gom số liệu của template.
	truoc, err := f.GetCellFormula(planningSheetRisk, "H16")
	require.NoError(t, err)
	require.NotEmpty(t, truoc, "template phải có sẵn công thức ở Risk!H16")

	require.NoError(t, clearSampleCells(f, planningSheetRisk, "H16", "D16"))

	sau, err := f.GetCellFormula(planningSheetRisk, "H16")
	require.NoError(t, err)
	require.Equal(t, truoc, sau, "ô công thức không được ghi đè")

	// Ô giá trị thường trong cùng lời gọi vẫn phải bị dọn.
	giaTri, err := f.GetCellValue(planningSheetRisk, "D16")
	require.NoError(t, err)
	require.Empty(t, giaTri)
}

func TestClearSampleBlock_DonHetVungChuNhat(t *testing.T) {
	t.Parallel()
	f, err := excelize.OpenReader(bytes.NewReader(assets.TestcaseReportXLSX))
	require.NoError(t, err)
	defer f.Close()

	require.NoError(t, clearSampleBlock(f, testcaseSheetBugData, "A", "V", 2, 14))

	for _, cell := range []string{"A2", "C2", "F2", "V2", "A14", "C14"} {
		got, err := f.GetCellValue(testcaseSheetBugData, cell)
		require.NoError(t, err)
		require.Empty(t, got, "Bug Data!%s chưa được dọn", cell)
	}

	// Dòng 1 là tiêu đề cột, nằm ngoài vùng nên phải còn nguyên.
	header, err := f.GetCellValue(testcaseSheetBugData, "A1")
	require.NoError(t, err)
	require.Equal(t, "Key", header)
}

func TestClearSampleBlock_CotKhongHopLe(t *testing.T) {
	t.Parallel()
	f, err := excelize.OpenReader(bytes.NewReader(assets.ProjectPlanXLSX))
	require.NoError(t, err)
	defer f.Close()

	err = clearSampleBlock(f, planningSheetRisk, "1", "H", 16, 22)
	require.Error(t, err)
}
