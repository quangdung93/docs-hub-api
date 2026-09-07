// Package domain định nghĩa entity thuần cho vertical slice xuất báo cáo dự án
// (UAT Report / Project Planning / Testcase — SRS v1.1 mục IX/X). Package này
// KHÔNG được import gin/gorm/redis.
package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound báo project không tồn tại (hoặc đã xóa) khi đọc metadata.
var ErrNotFound = errors.New("không tìm thấy project")

// 3 loại báo cáo theo SRS v1.1 mục IX/X — khớp CHECK constraint report_type
// trong migration 000013.
const (
	ReportTypeUAT      = "uat"
	ReportTypePlanning = "planning"
	ReportTypeTestcase = "testcase"
)

// ValidReportType báo report_type có được hỗ trợ hay không.
func ValidReportType(reportType string) bool {
	switch reportType {
	case ReportTypeUAT, ReportTypePlanning, ReportTypeTestcase:
		return true
	default:
		return false
	}
}

const (
	FormatXLSX = "xlsx"
	FormatPDF  = "pdf"
)

// Report là 1 lần xuất báo cáo đã lưu lịch sử (khác UAT export cũ của module
// document — stateless, không lưu).
type Report struct {
	ID          uuid.UUID `json:"id"`
	ProjectID   uuid.UUID `json:"project_id"`
	ReportType  string    `json:"report_type"`
	Format      string    `json:"format"`
	FileKey     string    `json:"-"`
	GeneratedBy uuid.UUID `json:"generated_by"`
	CreatedAt   time.Time `json:"created_at"`
}

// ReportItem là 1 dòng nội dung report (test case UAT sinh từ RAGFlow). Status
// để trống có chủ đích — QA điền tay sau khi test thật, hệ thống chỉ dựng khung.
type ReportItem struct {
	ID         uuid.UUID `json:"id"`
	ReportID   uuid.UUID `json:"report_id"`
	SourceRef  string    `json:"source_ref"`
	Title      string    `json:"title"`
	Detail     string    `json:"detail"`
	Status     string    `json:"status"`
	SequenceNo int       `json:"sequence_no"`
}

// Repository là cổng ra ngoài (Postgres) mà usecase phụ thuộc.
type Repository interface {
	MemberRole(ctx context.Context, projectID, actorID uuid.UUID) (string, error)
	ProjectMeta(ctx context.Context, projectID uuid.UUID) (name, code string, err error)
	// RAGFlowDatasetID đọc projects.ragflow_dataset_id (cột đã có sẵn từ module ingestion).
	RAGFlowDatasetID(ctx context.Context, projectID uuid.UUID) (string, error)
	// RAGFlowChatID/SaveRAGFlowChatID đọc/ghi projects.ragflow_chat_id — CHUNG cột với
	// tính năng hỏi-đáp (module chat), an toàn vì CompleteChat không giữ state phía server.
	RAGFlowChatID(ctx context.Context, projectID uuid.UUID) (string, error)
	SaveRAGFlowChatID(ctx context.Context, projectID uuid.UUID, proposed string) (string, error)
	// Create lưu report + toàn bộ item trong 1 transaction (usecase tự bọc qua TxManager).
	Create(ctx context.Context, report Report, items []ReportItem) error
	ListHistory(ctx context.Context, projectID uuid.UUID, page, limit int) ([]Report, int64, error)
}
