package http

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// urd-summary là API duy nhất FE gọi để dựng cột "Hoàn thiện", nên nó phải trả
// analysis_id — thiếu nó thì id chỉ tồn tại trong response của lần Analyze đầu
// tiên, mất là tài liệu kẹt awaiting_input vĩnh viễn (đo trên production
// 2026-09-16: summary không có id, Analyze lại bị URD_ANALYSIS_ACTIVE chặn).
func TestSummaryItem_CoAnalysisIDVaTenTruongSnakeCase(t *testing.T) {
	item := SummaryItem{
		DocumentID: uuid.New(), AnalysisID: uuid.New(),
		Status: "awaiting_input", TotalCases: 11, ResolvedCases: 0,
	}

	raw, err := json.Marshal(item)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))

	khoa := make([]string, 0, len(got))
	for k := range got {
		khoa = append(khoa, k)
	}
	require.ElementsMatch(t,
		[]string{"document_id", "analysis_id", "status", "total_cases", "resolved_cases"}, khoa)
	require.Equal(t, item.AnalysisID.String(), got["analysis_id"])
}
