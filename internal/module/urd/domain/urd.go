// Package domain chứa entity và hợp đồng nghiệp vụ cho tính năng "Nhận diện &
// Phân tích Edge Case cho URD (AI)" (URD v1.2 mục XI). Package này KHÔNG được
// import gin/gorm/redis.
package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Trạng thái của một phân tích edge case — khớp CHECK/giá trị mặc định trong
// migration 000014.
const (
	StatusAnalyzing     = "analyzing"      // đang gọi AI liệt kê edge case
	StatusAwaitingInput = "awaiting_input" // đã có danh sách, chờ người dùng nhập hướng giải quyết
	StatusCompleted     = "completed"      // đã nhập đủ hướng giải quyết, đã tạo phiên bản URD mới
	StatusFailed        = "failed"         // AI phân tích thất bại
)

var (
	ErrNotFound         = errors.New("không tìm thấy phân tích edge case")
	ErrNotURD           = errors.New("tài liệu chưa được xác nhận là URD")
	ErrRevisionNotReady = errors.New("phiên bản tài liệu chưa sẵn sàng để phân tích")
	ErrAnalysisActive   = errors.New("tài liệu đang có phân tích edge case chưa hoàn tất")
	ErrCaseUnresolved   = errors.New("còn edge case chưa nhập hướng giải quyết")
)

// Analysis là một lần AI phân tích edge case cho 1 revision của tài liệu URD.
type Analysis struct {
	ID            uuid.UUID
	DocumentID    uuid.UUID
	RevisionID    uuid.UUID
	Status        string
	TotalCases    int
	ResolvedCases int
	CreatedBy     uuid.UUID
	ErrorCode     string
	ErrorDetail   string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// EdgeCase là 1 case do AI liệt kê, kèm hướng giải quyết người dùng nhập.
type EdgeCase struct {
	ID             uuid.UUID
	AnalysisID     uuid.UUID
	SequenceNo     int
	Description    string
	Resolution     string
	ImageObjectKey string
	Resolved       bool
}

// Repository là cổng ra ngoài (Postgres) mà usecase phụ thuộc.
type Repository interface {
	// CreateAnalysis lưu 1 phân tích mới kèm toàn bộ edge case AI vừa liệt kê.
	CreateAnalysis(ctx context.Context, a Analysis, cases []EdgeCase) error
	// GetActiveAnalysis trả phân tích đang analyzing/awaiting_input của tài
	// liệu (nếu có) — trả (nil, nil, nil) nếu không có.
	GetActiveAnalysis(ctx context.Context, documentID uuid.UUID) (*Analysis, []EdgeCase, error)
	GetAnalysis(ctx context.Context, analysisID uuid.UUID) (*Analysis, []EdgeCase, error)
	// SaveResolutions cập nhật resolution/ảnh cho các case chỉ định, tính lại
	// resolved_cases/status trong 1 transaction, trả Analysis mới nhất.
	SaveResolutions(ctx context.Context, analysisID uuid.UUID, cases []EdgeCase) (*Analysis, error)
	// MarkCompleted đánh dấu phân tích đã tạo xong phiên bản URD mới.
	MarkCompleted(ctx context.Context, analysisID uuid.UUID) (*Analysis, error)
	// Summaries trả phân tích mới nhất (theo created_at) của mỗi document còn
	// sống thuộc project — dùng hiển thị cột "Hoàn thiện" trong bảng Quản lý
	// dự án (chỉ document nào có phân tích mới xuất hiện trong map trả về).
	Summaries(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID]Analysis, error)

	// MemberRole/RAGFlowDatasetID/RAGFlowChatID/SaveRAGFlowChatID: cùng
	// pattern với module report (đọc thẳng bảng projects/project_members,
	// xem report/repository/repository.go) — urd không phụ thuộc module
	// report vì đó là chi tiết hạ tầng, không phải hợp đồng nghiệp vụ chung.
	MemberRole(ctx context.Context, projectID, actorID uuid.UUID) (string, error)
	RAGFlowDatasetID(ctx context.Context, projectID uuid.UUID) (string, error)
	RAGFlowChatID(ctx context.Context, projectID uuid.UUID) (string, error)
	SaveRAGFlowChatID(ctx context.Context, projectID uuid.UUID, proposed string) (string, error)
}
