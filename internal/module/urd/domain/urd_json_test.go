package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Hai test dưới đây canh HỢP ĐỒNG JSON của Analysis/EdgeCase — thứ client đọc
// được. Không có tag json thì encoding/json trả nguyên tên field Go ("ID",
// "TotalCases"), lệch với snake_case của toàn repo; client đọc theo tài liệu
// nhận undefined. Đúng như vậy đã làm mất analysis_id khi test production
// 2026-09-15, mà mất id thì không API nào trả lại được.

func TestAnalysis_TenTruongJSONTheoSnakeCase(t *testing.T) {
	a := Analysis{
		ID: uuid.New(), DocumentID: uuid.New(), RevisionID: uuid.New(),
		Status: StatusAwaitingInput, TotalCases: 11, ResolvedCases: 0,
		CreatedBy: uuid.New(), ErrorCode: "X", ErrorDetail: "y",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	var got map[string]any
	raw, err := json.Marshal(a)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &got))

	mong := []string{
		"id", "document_id", "revision_id", "status", "total_cases",
		"resolved_cases", "created_by", "error_code", "error_detail",
		"created_at", "updated_at",
	}
	require.ElementsMatch(t, mong, khoa(got))
	require.Equal(t, a.ID.String(), got["id"], "client lấy analysis_id từ đây")
	require.Equal(t, float64(11), got["total_cases"])
}

func TestEdgeCase_TenTruongJSONTheoSnakeCase(t *testing.T) {
	c := EdgeCase{
		ID: uuid.New(), AnalysisID: uuid.New(), SequenceNo: 3,
		Description: "mô tả", Resolution: "hướng giải quyết",
		ImageObjectKey: "urd/a/b/anh.png", Resolved: true,
	}
	var got map[string]any
	raw, err := json.Marshal(c)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &got))

	mong := []string{
		"id", "analysis_id", "sequence_no", "description", "resolution",
		"image_object_key", "resolved",
	}
	require.ElementsMatch(t, mong, khoa(got))
	require.Equal(t, c.ID.String(), got["id"], "client lấy case_id từ đây để gửi resolutions")
	require.Equal(t, float64(3), got["sequence_no"])
}

// TestEdgeCase_ChuaCoAnhThiBoQuaTruongAnh giữ omitempty: case chưa đính ảnh thì
// response không kèm khóa rỗng, client phân biệt được "chưa có ảnh".
func TestEdgeCase_ChuaCoAnhThiBoQuaTruongAnh(t *testing.T) {
	raw, err := json.Marshal(EdgeCase{ID: uuid.New(), AnalysisID: uuid.New(), SequenceNo: 1})
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	require.NotContains(t, khoa(got), "image_object_key")
}

func khoa(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
