package usecase

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Prompt nói với LLM số task tối đa mỗi milestone, còn planningMilestoneSlots
// mới là chỗ chứa thật. Hai con số lệch nhau thì LLM sinh thừa rồi bị cắt âm
// thầm — không log, không cảnh báo. Test này khoá hai bên lại với nhau.
func TestPlanningPrompt_KhopVoiSoSlotThat(t *testing.T) {
	t.Parallel()
	require.Len(t, planningMilestoneSlots, maxPlanningMilestones)
	require.Equal(t, 7, planningMilestoneSlots[0].maxTasks)
	for _, slot := range planningMilestoneSlots[1:] {
		require.Equal(t, 3, slot.maxTasks,
			"nếu template đổi số slot thì phải sửa cả planningPromptTemplate")
	}

	prompt := planningPrompt("Demo Project")
	require.Contains(t, prompt, "tối đa 7 task")
	require.Contains(t, prompt, "tối đa 3 task")
}

// group chỉ được nhận đúng bốn nhóm mà sheet Guideline định nghĩa và Dashboard
// đếm bằng COUNTIF khớp chính xác. "Non-functional" từng lọt vào prompt và làm
// mọi test case rơi vào đó biến mất khỏi thống kê.
func TestTestcasePrompt_ChiDungNhomCoTrongTemplate(t *testing.T) {
	t.Parallel()
	prompt := testcasePrompt("Demo Project", maxTestcaseItems)
	for _, group := range []string{"Functional", "UI", "Integration", "Database"} {
		require.Contains(t, prompt, group)
	}
	require.NotContains(t, prompt, "Non-functional")
	require.Contains(t, prompt, "req_id KHÔNG được tự đặt")
}

// Ba prompt đều phải ràng buộc ngôn ngữ: chat assistant của RAGFlow đặt
// language="English", template và người đọc đều tiếng Việt.
func TestBaPrompt_DeuRangBuocTiengViet(t *testing.T) {
	t.Parallel()
	for ten, prompt := range map[string]string{
		"uat":      uatPrompt("Demo Project", 10),
		"planning": planningPrompt("Demo Project"),
		"testcase": testcasePrompt("Demo Project", 10),
	} {
		require.True(t, strings.Contains(prompt, "Trả lời bằng tiếng Việt"),
			"prompt %s thiếu ràng buộc ngôn ngữ", ten)
	}
}
