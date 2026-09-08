package ingestion

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBackoffFor_LuyThuaRoiChanTran(t *testing.T) {
	const backoffCap = 5 * time.Minute
	for _, tc := range []struct {
		attempt int
		mong    time.Duration
	}{
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{6, 64 * time.Second},
		{8, 256 * time.Second},
		{9, backoffCap}, // 512s đã vượt trần
		{20, backoffCap},
	} {
		require.Equal(t, tc.mong, backoffFor(tc.attempt, backoffCap), "attempt=%d", tc.attempt)
	}
}

// attempt lớn bất thường không được làm phép dịch bit tràn thành số âm — chờ âm
// nghĩa là available_at nằm trong quá khứ, job quay lại ngay lập tức và biến
// backoff thành vòng lặp nóng.
func TestBackoffFor_KhongTranKhiAttemptRatLon(t *testing.T) {
	const backoffCap = 5 * time.Minute
	for _, attempt := range []int{31, 63, 64, 1000} {
		wait := backoffFor(attempt, backoffCap)
		require.Equal(t, backoffCap, wait, "attempt=%d", attempt)
		require.Positive(t, wait)
	}
}

func TestBackoffFor_AttemptKhongHopLeThiVanChoMotNhip(t *testing.T) {
	const backoffCap = time.Minute
	require.Equal(t, 2*time.Second, backoffFor(0, backoffCap))
	require.Equal(t, 2*time.Second, backoffFor(-5, backoffCap))
}

// Ngân sách mặc định phải trùm được sự cố đã đo thật: RAGFlow chết 30 phút ngày
// 2026-09-03 và ≥50 phút ngày 2026-09-04. Test này giữ cho ai đó chỉnh hằng số
// mà không nhận ra mình vừa thu ngân sách xuống dưới mức đã cân nhắc.
func TestNganSachMacDinh_TrumDuocSuCoDaGap(t *testing.T) {
	tong := time.Duration(0)
	for attempt := 1; attempt < defaultMaxAttempts; attempt++ {
		tong += backoffFor(attempt, defaultRetryBackoffCap)
	}
	t.Logf("ngân sách chờ với %d lượt, trần %s: %s",
		defaultMaxAttempts, defaultRetryBackoffCap, tong)
	require.Greater(t, tong, 30*time.Minute, "phải trùm được sự cố 30 phút đã đo")
	require.Less(t, tong, 2*time.Hour, "quá dài thì người dùng chờ vô ích khi RAGFlow chết hẳn")
}

func TestRetryBudget_BoTrongThiDungMacDinh(t *testing.T) {
	p := &RAGFlowProcessor{}
	maxAttempts, backoffCap := p.retryBudget()
	require.Equal(t, defaultMaxAttempts, maxAttempts)
	require.Equal(t, defaultRetryBackoffCap, backoffCap)

	p = &RAGFlowProcessor{cfg: RAGFlowProcessorConfig{MaxAttempts: 3, RetryBackoffCap: time.Minute}}
	maxAttempts, backoffCap = p.retryBudget()
	require.Equal(t, 3, maxAttempts)
	require.Equal(t, time.Minute, backoffCap)
}
