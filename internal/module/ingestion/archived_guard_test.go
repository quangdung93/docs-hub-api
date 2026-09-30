package ingestion

import (
	"testing"

	"github.com/stretchr/testify/require"

	documentdomain "github.com/quangdung93/docs-hub-api/internal/module/document/domain"
)

// Hằng khai lại trong gói này phải khớp bản gốc bên module document. Lệch một
// ký tự là chốt chặn thành vô nghĩa: mệnh đề `status <> 'archivedd'` luôn đúng,
// worker vẫn ghi đè nhãn lưu trữ, sha256 bị khoá lại — mà mọi thứ vẫn chạy
// bình thường nên không ai biết.
func TestRevisionStatusArchived_KhopVoiModuleDocument(t *testing.T) {
	require.Equal(t, documentdomain.RevisionStatusArchived, revisionStatusArchived)
}
