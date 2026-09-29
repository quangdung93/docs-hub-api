// Package domain chứa entity và hợp đồng nghiệp vụ quản lý tài liệu.
package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
)

var (
	ErrNotFound = errors.New("không tìm thấy tài liệu")
	ErrConflict = errors.New("xung đột phiên bản tài liệu")
	// ErrDuplicateContent: trong cùng một scope (version hoặc change request) đã
	// có revision khác mang đúng nội dung này — chỉ số uk_revisions_*_hash chặn.
	// Đây là chuyện người dùng gây ra và tự sửa được, nên là lỗi NGHIỆP VỤ.
	ErrDuplicateContent = errors.New("nội dung tài liệu đã tồn tại trong phạm vi này")
)

type Scope struct {
	VersionID       *uuid.UUID `json:"project_version_id,omitempty"`
	ChangeRequestID *uuid.UUID `json:"change_request_id,omitempty"`
}

func (s Scope) Valid() bool { return (s.VersionID == nil) != (s.ChangeRequestID == nil) }

// ScopeKindVersion và ScopeKindChangeRequest là giá trị "kind" mà ScopeMeta
// trả về — dùng chung giữa repository và usecase để tránh lặp magic string.
const (
	ScopeKindVersion       = "version"
	ScopeKindChangeRequest = "change_request"
)

// Giá trị Document.DocType khi người dùng xác nhận loại tài liệu — mở khoá
// luồng AI phân tích edge case (xem URD v1.2 mục XI).
const (
	DocTypeURD = "urd" // User Requirement Document
	DocTypePRD = "prd" // Product Requirement Document
)

// AnalyzableDocTypes là các loại tài liệu được phân tích edge case. Một danh
// sách duy nhất cho toàn repo: module urd hỏi "tài liệu này phân tích được
// không", còn ConfirmDocType hỏi "giá trị này hợp lệ không" — trước đây hai
// câu hỏi đó được viết rời thành hai điều kiện so sánh thẳng với hằng, thêm
// loại mới là phải nhớ sửa cả hai chỗ.
func AnalyzableDocTypes() []string { return []string{DocTypeURD, DocTypePRD} }

// IsAnalyzableDocType báo doc_type có thuộc nhóm phân tích được hay không.
// Chuỗi rỗng (chưa xác nhận) trả false.
func IsAnalyzableDocType(docType string) bool {
	for _, t := range AnalyzableDocTypes() {
		if docType == t {
			return true
		}
	}
	return false
}

// DocTypeLabel trả tên đầy đủ dùng trong prompt gửi AI và thông báo cho người
// dùng. Loại lạ trả về chính giá trị đó để không nuốt mất thông tin.
func DocTypeLabel(docType string) string {
	switch docType {
	case DocTypeURD:
		return "URD (User Requirement Document)"
	case DocTypePRD:
		return "PRD (Product Requirement Document)"
	default:
		return docType
	}
}

type Document struct {
	ID          uuid.UUID `json:"id"`
	ProjectID   uuid.UUID `json:"project_id"`
	CreatedBy   uuid.UUID `json:"created_by"`
	Title       string    `json:"title"`
	Key         string    `json:"document_key"`
	Description string    `json:"description"`
	// DocType rỗng nghĩa là chưa xác định/chưa xác nhận; giá trị hợp lệ là
	// "urd" hoặc "prd", do người dùng xác nhận qua ConfirmDocType.
	DocType string `json:"doc_type,omitempty"`
	Version int    `json:"version"`
	// DocumentVersion và UploadedAt là metadata của revision được upload gần
	// nhất, dùng cho danh sách tài liệu. Lịch sử đầy đủ nằm trong Revision.
	DocumentVersion string     `json:"document_version"`
	UploadedAt      *time.Time `json:"uploaded_at,omitempty"`
	IsDeleted       bool       `json:"is_deleted"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type Revision struct {
	ID                uuid.UUID  `json:"id"`
	DocumentID        uuid.UUID  `json:"document_id"`
	ProjectID         uuid.UUID  `json:"project_id"`
	CreatedBy         uuid.UUID  `json:"created_by"`
	Scope             Scope      `json:"scope"`
	RevisionNo        int        `json:"revision_no"`
	DocumentVersion   string     `json:"document_version"`
	FileName          string     `json:"file_name"`
	MediaType         string     `json:"media_type"`
	SHA256            string     `json:"sha256"`
	ObjectKey         string     `json:"-"`
	CanonicalTextKey  string     `json:"-"`
	Status            string     `json:"status"`
	SizeBytes         int64      `json:"size_bytes"`
	ErrorCode         string     `json:"error_code,omitempty"`
	ErrorDetail       string     `json:"error_detail,omitempty"`
	RAGFlowDocumentID string     `json:"-"`
	RAGFlowSyncStatus string     `json:"ragflow_sync_status,omitempty"`
	RAGFlowLastError  string     `json:"ragflow_last_error,omitempty"`
	RAGFlowSyncedAt   *time.Time `json:"ragflow_synced_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type Upload struct {
	ID, ProjectID, DocumentID, RevisionID, CreatedBy                                    uuid.UUID
	Scope                                                                               Scope
	Title, Description, DocumentVersion, FileName, MediaType, SHA256, ObjectKey, Status string
	SizeBytes                                                                           int64
	ExpiresAt                                                                           time.Time
}

type Filter struct {
	Query, Status, MediaType, DocumentVersion string
	VersionID, ChangeRequestID                *uuid.UUID
	IncludeDeleted                            bool
}

// UATItem là 1 dòng trong sheet Report 1_Module của UAT Report — tài liệu
// (document) cùng revision mới nhất khớp scope được chọn khi export.
type UATItem struct {
	DocumentID uuid.UUID
	Title      string
	FileName   string
	RevisionNo int
	Status     string
	UpdatedAt  time.Time
}

type CreateRevisionParams struct {
	DocumentID, RevisionID, ProjectID, ActorID                                  uuid.UUID
	Scope                                                                       Scope
	Title, Description, DocumentVersion, FileName, MediaType, SHA256, ObjectKey string
	SizeBytes                                                                   int64
	AutoVersion                                                                 bool
}

type Repository interface {
	MemberRole(ctx context.Context, projectID, actorID uuid.UUID) (string, error)
	ScopeExists(ctx context.Context, projectID uuid.UUID, scope Scope) (bool, error)
	CreateRevision(ctx context.Context, in CreateRevisionParams) (*Document, *Revision, error)
	CreateUpload(ctx context.Context, upload *Upload) error
	FindUpload(ctx context.Context, projectID, uploadID uuid.UUID) (*Upload, error)
	CompleteUpload(ctx context.Context, upload *Upload) (*Document, *Revision, error)
	List(ctx context.Context, projectID uuid.UUID, filter Filter, page pagination.Query) ([]Document, int64, error)
	FindDocument(ctx context.Context, projectID, documentID uuid.UUID) (*Document, []Revision, error)
	FindRevision(ctx context.Context, projectID, documentID, revisionID uuid.UUID) (*Revision, error)
	Update(ctx context.Context, projectID, documentID uuid.UUID, title, description string, version int) (*Document, error)
	// SetDocType xác nhận/đổi loại tài liệu (optimistic lock qua version, cùng
	// cơ chế với Update) — dùng cho luồng xác nhận URD (URD v1.2 mục XI).
	//
	// Nhận actor để ghi audit log: đây là thao tác mở khoá cả luồng phân tích
	// AI và có thể gán sai loại cho tài liệu của người khác, nhưng trước đây
	// KHÔNG để lại dấu vết nào (Retry và SoftDelete đều có ghi). Ngày 25/09 có
	// ba tài liệu bị gán nhãn URD nhầm mà không tra được ai gán lúc nào, phải
	// suy ngược từ cột version và updated_at.
	SetDocType(
		ctx context.Context, projectID, documentID uuid.UUID, docType string, version int, actor uuid.UUID,
	) (*Document, error)
	Retry(ctx context.Context, projectID, documentID, revisionID, actorID uuid.UUID) error
	SoftDelete(ctx context.Context, projectID, documentID, actorID uuid.UUID) error

	// ProjectMeta trả tên và code của project — dùng điền sheet Summary của UAT Report.
	ProjectMeta(ctx context.Context, projectID uuid.UUID) (name, code string, err error)
	// ScopeMeta trả nhãn hiển thị và loại scope ("version"/"change_request"/"" nếu là toàn dự án).
	ScopeMeta(ctx context.Context, projectID uuid.UUID, scope Scope) (label, kind string, err error)
	// UATItems liệt kê tài liệu (kèm revision mới nhất) khớp scope, dùng điền sheet Report 1_Module.
	UATItems(ctx context.Context, projectID uuid.UUID, scope Scope) ([]UATItem, error)
}
