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
	// StatusCancelled: người dùng chủ động bỏ phân tích đang dở. Giữ lại bản
	// ghi thay vì xoá để còn truy được tài liệu đã từng bị phân tích nhầm.
	// Không nằm trong uk_urd_analyses_active nên huỷ xong là phân tích lại
	// được ngay.
	StatusCancelled = "cancelled"
)

// ActiveStatuses là các trạng thái bị uk_urd_analyses_active coi là "đang
// hoạt động" — khớp đúng mệnh đề WHERE của chỉ số trong migration 000014.
// Đổi ở đây mà quên đổi migration (hoặc ngược lại) là khoá tài liệu vĩnh viễn.
func ActiveStatuses() []string { return []string{StatusAnalyzing, StatusAwaitingInput} }

var (
	ErrNotFound         = errors.New("không tìm thấy phân tích edge case")
	ErrNotURD           = errors.New("tài liệu chưa được xác nhận là URD")
	ErrRevisionNotReady = errors.New("phiên bản tài liệu chưa sẵn sàng để phân tích")
	ErrAnalysisActive   = errors.New("tài liệu đang có phân tích edge case chưa hoàn tất")
	ErrCaseUnresolved   = errors.New("còn edge case chưa nhập hướng giải quyết")
	// ErrAnalysisNotActive: phân tích đã completed/failed/cancelled rồi nên
	// không còn gì để huỷ. Tách riêng khỏi ErrNotFound để client nói đúng với
	// người dùng thay vì báo "không tìm thấy".
	ErrAnalysisNotActive = errors.New("phân tích edge case không còn đang dở")
)

// Analysis là một lần AI phân tích edge case cho 1 revision của tài liệu URD.
//
// Tag json BẮT BUỘC: thiếu tag thì encoding/json lấy nguyên tên field Go
// ("ID", "TotalCases"), lệch cả với snake_case của toàn repo lẫn với
// camelCase mà swag sinh ra trong docs/swagger — client đọc theo tài liệu sẽ
// nhận undefined, và đó là cách `analysis_id` bị mất ở lần test production
// 2026-09-15 (mất id là tài liệu bị khoá, không API nào trả lại).
type Analysis struct {
	ID            uuid.UUID `json:"id"`
	DocumentID    uuid.UUID `json:"document_id"`
	RevisionID    uuid.UUID `json:"revision_id"`
	Status        string    `json:"status"`
	TotalCases    int       `json:"total_cases"`
	ResolvedCases int       `json:"resolved_cases"`
	CreatedBy     uuid.UUID `json:"created_by"`
	ErrorCode     string    `json:"error_code,omitempty"`
	ErrorDetail   string    `json:"error_detail,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// EdgeCase là 1 case do AI liệt kê, kèm hướng giải quyết người dùng nhập.
type EdgeCase struct {
	ID             uuid.UUID `json:"id"`
	AnalysisID     uuid.UUID `json:"analysis_id"`
	SequenceNo     int       `json:"sequence_no"`
	Description    string    `json:"description"`
	Resolution     string    `json:"resolution"`
	ImageObjectKey string    `json:"image_object_key,omitempty"`
	Resolved       bool      `json:"resolved"`
	// IncludeInDocument: case có được đưa vào phụ lục của phiên bản URD mới
	// hay không. Không phải mọi case "đã nhập hướng giải quyết" đều có nội
	// dung đáng đưa vào tài liệu — hướng giải quyết kiểu "Không"/"Chưa có
	// chức năng này" là một câu trả lời hợp lệ nhưng không phải nội dung cần
	// bổ sung vào URD. Client (FE) có thể gửi tường minh khi submit; nếu
	// không gửi, usecase tự suy đoán từ nội dung Resolution (xem
	// usecase/no_action_resolution.go) — không cần FE đổi gì vẫn hoạt động.
	IncludeInDocument bool `json:"include_in_document"`
	// TargetHeading là tiêu đề mục (AC/BR/...) mà AI ĐỀ XUẤT bổ sung nội dung
	// xử lý case này vào, copy nguyên văn từ tài liệu. Chỉ là đề xuất:
	// docxmerge còn phải đối chiếu với tiêu đề thật trong word/document.xml
	// mới chèn thẳng vào mục đó, không khớp thì lùi về phụ lục cuối tài liệu.
	// Rỗng = AI không xác định được mục nào.
	TargetHeading string `json:"target_heading,omitempty"`
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
	// Cancel chuyển phân tích đang dở sang cancelled để gỡ khoá tài liệu.
	// Trả ErrAnalysisNotActive nếu phân tích đã rời trạng thái hoạt động.
	Cancel(ctx context.Context, analysisID uuid.UUID) (*Analysis, error)
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
