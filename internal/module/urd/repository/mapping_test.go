package repository

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/module/urd/domain"
)

// fromAnalysis phải chép CẢ hai mốc thời gian: usecase gán sẵn rồi trả bản ghi
// đó cho client, nên DB phải lưu đúng giá trị ấy. Bỏ trống thì GORM tự điền giờ
// của nó và response lệch với dữ liệu đã lưu.
func TestFromAnalysis_ChepMocThoiGianXuongBanGhi(t *testing.T) {
	luc := time.Date(2026, 9, 16, 9, 53, 43, 0, time.UTC)
	a := domain.Analysis{
		ID: uuid.New(), DocumentID: uuid.New(), RevisionID: uuid.New(),
		Status: domain.StatusAwaitingInput, TotalCases: 2, CreatedBy: uuid.New(),
		CreatedAt: luc, UpdatedAt: luc,
	}

	m := fromAnalysis(a)

	require.Equal(t, luc, m.CreatedAt)
	require.Equal(t, luc, m.UpdatedAt)
	require.Equal(t, a.ID.String(), m.ID)
	require.Equal(t, a.RevisionID.String(), m.DocumentRevisionID)
}

// Không gán thời gian thì vẫn để zero-value, cho GORM tự điền như trước.
func TestFromAnalysis_KhongGanThiDeGORMTuDien(t *testing.T) {
	m := fromAnalysis(domain.Analysis{
		ID: uuid.New(), DocumentID: uuid.New(), RevisionID: uuid.New(), CreatedBy: uuid.New(),
	})

	require.True(t, m.CreatedAt.IsZero())
	require.True(t, m.UpdatedAt.IsZero())
}

// toAnalysis là chiều ngược lại — đọc từ DB lên phải giữ nguyên mốc thời gian.
func TestToAnalysis_GiuMocThoiGianDocTuDB(t *testing.T) {
	luc := time.Date(2026, 9, 16, 9, 53, 43, 0, time.UTC)
	m := analysisModel{
		ID: uuid.New().String(), DocumentID: uuid.New().String(),
		DocumentRevisionID: uuid.New().String(), CreatedBy: uuid.New().String(),
		Status: domain.StatusCompleted, CreatedAt: luc, UpdatedAt: luc,
	}

	a := toAnalysis(m)

	require.Equal(t, luc, a.CreatedAt)
	require.Equal(t, luc, a.UpdatedAt)
}
