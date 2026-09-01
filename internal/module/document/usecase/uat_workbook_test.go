package usecase

import (
	"bytes"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"github.com/quangdung93/docs-hub-api/internal/module/document/domain"
)

func TestBuildUATWorkbook_FillsSummaryAndModuleRows(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	due := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	content, err := buildUATWorkbook(uatWorkbookInput{
		ProjectName: "Docs Hub", ProjectCode: "DHUB",
		ScopeLabel: "v1.2", ScopeKind: domain.ScopeKindVersion,
		PO: "Nguyễn Văn A", PM: "Trần Thị B",
		AccountTest: "1. huongttt38 - ISC", ScopeTest: "Toàn bộ tài liệu v1.2",
		StartDate: &start, DueDate: &due,
		Items: []domain.UATItem{
			{DocumentID: uuid.New(), Title: "URD v1.0", FileName: "urd.docx", RevisionNo: 2, Status: "ready"},
			{DocumentID: uuid.New(), Title: "SRS v1.0", FileName: "srs.docx", RevisionNo: 1, Status: "ready"},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, content)

	f, err := excelize.OpenReader(bytes.NewReader(content))
	require.NoError(t, err)
	defer f.Close()

	summaryTitle, err := f.GetCellValue(uatSheetSummary, "D3")
	require.NoError(t, err)
	require.Equal(t, "Docs Hub - v1.2", summaryTitle)

	dates, err := f.GetCellValue(uatSheetSummary, "F3")
	require.NoError(t, err)
	require.Equal(t, "01/09/2026 - 08/09/2026", dates)

	po, err := f.GetCellValue(uatSheetSummary, "D4")
	require.NoError(t, err)
	require.Equal(t, "Nguyễn Văn A", po)

	pm, err := f.GetCellValue(uatSheetSummary, "D5")
	require.NoError(t, err)
	require.Equal(t, "Trần Thị B", pm)

	module1, err := f.GetCellValue(uatSheetModule, "C12")
	require.NoError(t, err)
	require.Equal(t, "URD v1.0", module1)

	steps1, err := f.GetCellValue(uatSheetModule, "D12")
	require.NoError(t, err)
	require.Contains(t, steps1, "urd.docx")
	require.Contains(t, steps1, "revision #2")

	module2, err := f.GetCellValue(uatSheetModule, "C13")
	require.NoError(t, err)
	require.Equal(t, "SRS v1.0", module2)

	no2, err := f.GetCellValue(uatSheetModule, "B13")
	require.NoError(t, err)
	require.Equal(t, "2", no2)
}

func TestFormatUATDate_NilIsTBD(t *testing.T) {
	require.Equal(t, "TBD", formatUATDate(nil))
	d := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	require.Equal(t, "05/01/2026", formatUATDate(&d))
}
