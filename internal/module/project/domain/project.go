// Package domain chứa entity, quy tắc nghiệp vụ, port repository và lỗi nghiệp vụ
// của module project.
//
// Đây là tầng TRONG CÙNG của Clean Architecture: chỉ phụ thuộc stdlib, uuid và
// common/apperr. TUYỆT ĐỐI không import gin/gorm/redis (được golangci-lint
// depguard bảo vệ). database/sql và database/sql/driver là stdlib nên được phép
// dùng ở đây — chỉ để ProjectSettings tự mô tả cách (de)serialize JSONB, không
// hề biết tới GORM.
package domain

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
)

// Role là vai trò của một thành viên trong dự án.
type Role string

const (
	RoleOwner  Role = "owner"  // toàn quyền: quản lý dự án + thành viên
	RoleEditor Role = "editor" // upload tài liệu + truy vấn
	RoleViewer Role = "viewer" // chỉ truy vấn
)

const StatusDraft = "draft"

// Valid kiểm tra role có nằm trong tập hợp lệ không.
func (r Role) Valid() bool {
	switch r {
	case RoleOwner, RoleEditor, RoleViewer:
		return true
	default:
		return false
	}
}

// MemberStatus là trạng thái thành viên trong dự án.
type MemberStatus string

const (
	MemberStatusPending MemberStatus = "pending" // vừa được mời, chưa xác nhận
	MemberStatusActive  MemberStatus = "active"  // đã accept lời mời
)

// DefaultProjectStatus là trạng thái mặc định của dự án mới tạo.
const DefaultProjectStatus = "active"

// ProjectSettings là cấu hình RAG của dự án, lưu dạng JSONB ở cột `settings`.
// Implement driver.Valuer/sql.Scanner để tầng repository (GORM) tự động
// marshal/unmarshal mà domain không cần biết chi tiết ORM.
type ProjectSettings struct {
	Model          string   `json:"model"`
	TopK           int      `json:"top_k"`
	ChunkSize      int      `json:"chunk_size"`
	AllowedFormats []string `json:"allowed_formats"`
}

// Value implement driver.Valuer — chuyển settings thành JSON khi ghi DB.
func (s ProjectSettings) Value() (driver.Value, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("marshal project settings: %w", err)
	}
	return b, nil
}

// Scan implement sql.Scanner — đọc JSONB từ DB vào struct.
func (s *ProjectSettings) Scan(value any) error {
	if value == nil {
		*s = ProjectSettings{}
		return nil
	}
	var raw []byte
	switch v := value.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("project settings: kiểu dữ liệu không hỗ trợ %T", value)
	}
	if len(raw) == 0 {
		*s = ProjectSettings{}
		return nil
	}
	if err := json.Unmarshal(raw, s); err != nil {
		return fmt.Errorf("unmarshal project settings: %w", err)
	}
	return nil
}

// allowedAvatarFormats là danh sách mime_type CỐ ĐỊNH được phép làm ảnh đại
// diện dự án.
var allowedAvatarFormats = map[string]bool{ //nolint:gochecknoglobals // bảng tra cứu bất biến
	"image/png":  true,
	"image/jpeg": true,
	"image/webp": true,
}

// IsAvatarFormatAllowed kiểm tra mime_type có thuộc danh sách cho phép làm
// ảnh đại diện dự án không.
func IsAvatarFormatAllowed(mimeType string) bool {
	return allowedAvatarFormats[mimeType]
}

// Project là entity dự án — mô hình nghiệp vụ thuần, KHÔNG có tag gorm.
type Project struct {
	ID                uuid.UUID
	OwnerID           uuid.UUID
	Code              string
	Name              string
	Description       string
	Status            string
	Settings          ProjectSettings
	Version           int
	RAGFlowDatasetID  string
	RAGFlowSyncStatus string
	RAGFlowLastError  string
	// AvatarKey là object key trong MinIO chứa ảnh đại diện dự án. Rỗng nghĩa
	// là chưa có ảnh. Chỉ được set qua SetAvatar SAU KHI usecase xác nhận
	// object đã thực sự tồn tại trong storage (luồng presigned URL).
	AvatarKey string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewProject tạo dự án mới ở trạng thái active.
func NewProject(ownerID uuid.UUID, name, description string, settings ProjectSettings) *Project {
	p := &Project{
		ID:          uuid.New(),
		OwnerID:     ownerID,
		Name:        name,
		Description: description,
		Status:      DefaultProjectStatus,
		Settings:    settings,
		Version:     1,
	}
	p.Code = "project_" + p.ID.String()[:12]
	return p
}

// ProjectVersion là một mốc tài liệu trong timeline của project. Version mới
// luôn bắt đầu ở draft và dùng chung dataset RAGFlow của project.
type ProjectVersion struct {
	ID         uuid.UUID  `json:"id"`
	ProjectID  uuid.UUID  `json:"project_id"`
	Label      string     `json:"label"`
	SequenceNo int64      `json:"sequence_no"`
	Status     string     `json:"status"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
	CreatedBy  uuid.UUID  `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// ChangeProfile cập nhật thông tin chung/cấu hình. Con trỏ nil = giữ nguyên
// giá trị hiện tại (cập nhật từng phần theo PATCH).
func (p *Project) ChangeProfile(name, description *string, settings *ProjectSettings) {
	if name != nil {
		p.Name = *name
	}
	if description != nil {
		p.Description = *description
	}
	if settings != nil {
		p.Settings = *settings
	}
}

// IsOwner kiểm tra user có phải chủ dự án không.
func (p *Project) IsOwner(userID uuid.UUID) bool {
	return p.OwnerID == userID
}

// SetAvatar gán object key ảnh đại diện. Usecase gọi hàm này SAU KHI đã xác
// minh (qua port.ObjectStore.Stat) ảnh thực sự tồn tại — domain không tự
// kiểm tra vì đó là chi tiết hạ tầng.
func (p *Project) SetAvatar(key string) {
	p.AvatarKey = key
}

// ProjectMember là entity thành viên dự án.
type ProjectMember struct {
	ID        uuid.UUID
	ProjectID uuid.UUID
	UserID    uuid.UUID
	Role      Role
	Status    MemberStatus
	InvitedAt time.Time
	JoinedAt  *time.Time
}

// ProjectStats là các con số tổng hợp của một dự án, để client hiển thị mà
// không phải gọi thêm nhiều endpoint rồi tự đếm (mục #10 báo cáo API).
type ProjectStats struct {
	DocumentCount int64
	MemberCount   int64
	ChunkCount    int64
}

// MemberWithUser là read model cho danh sách thành viên: kèm sẵn tên và email
// để client khỏi phải gọi GET /users/{id} cho từng người (mục #11 báo cáo API).
type MemberWithUser struct {
	ProjectMember
	FullName string
	Email    string
}

// NewInvite tạo một lời mời thành viên mới (trạng thái pending).
func NewInvite(projectID, userID uuid.UUID, role Role) *ProjectMember {
	return &ProjectMember{
		ID:        uuid.New(),
		ProjectID: projectID,
		UserID:    userID,
		Role:      role,
		Status:    MemberStatusPending,
	}
}

// Accept xác nhận lời mời: pending -> active, set joined_at = now.
// Lời mời không ở trạng thái pending -> ErrInviteNotPending (nghiệp vụ).
func (m *ProjectMember) Accept(now time.Time) error {
	if m.Status != MemberStatusPending {
		return ErrInviteNotPending
	}
	m.Status = MemberStatusActive
	m.JoinedAt = &now
	return nil
}

// ProjectRepository là PORT (interface) mà usecase phụ thuộc để thao tác dự án.
//
// Quy ước lỗi trả về:
//   - ErrNotFound  : không có bản ghi khớp.
//   - ErrDuplicate : vi phạm ràng buộc duy nhất.
type ProjectRepository interface {
	Create(ctx context.Context, p *Project) error
	// Update cập nhật name/description/status/settings theo id (không optimistic
	// lock — dự án không có cột version).
	Update(ctx context.Context, p *Project) error
	FindByID(ctx context.Context, id uuid.UUID) (*Project, error)
	// ListForUser liệt kê dự án mà user là owner hoặc thành viên active.
	ListForUser(ctx context.Context, userID uuid.UUID, page pagination.Query) ([]Project, int64, error)
	// Delete xóa CỨNG dự án (DB tự cascade sang project_members).
	Delete(ctx context.Context, id uuid.UUID) error
	// Stats đếm tài liệu/thành viên/chunk cho NHIỀU dự án trong một truy vấn.
	// Nhận danh sách id để tránh N+1 khi liệt kê. Dự án không có bản ghi nào
	// vẫn xuất hiện trong map với giá trị 0.
	Stats(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]ProjectStats, error)
}

// ProjectMemberRepository là PORT (interface) mà usecase phụ thuộc để thao tác
// thành viên dự án.
type ProjectMemberRepository interface {
	Create(ctx context.Context, m *ProjectMember) error
	FindByProjectAndUser(ctx context.Context, projectID, userID uuid.UUID) (*ProjectMember, error)
	ListByProject(ctx context.Context, projectID uuid.UUID) ([]MemberWithUser, error)
	UpdateRole(ctx context.Context, projectID, userID uuid.UUID, role Role) error
	// UpdateStatus ghi lại status + joined_at (dùng cho Accept).
	UpdateStatus(ctx context.Context, m *ProjectMember) error
	Delete(ctx context.Context, projectID, userID uuid.UUID) error
}

// ProjectVersionRepository quản lý timeline version và audit tương ứng.
type ProjectVersionRepository interface {
	Create(ctx context.Context, version *ProjectVersion, requestID string) error
	List(ctx context.Context, projectID uuid.UUID, page pagination.Query) ([]ProjectVersion, int64, error)
}

// Lỗi hợp đồng của repository (độc lập với chi tiết driver/ORM).
var (
	ErrNotFound              = newRepoError("không tìm thấy bản ghi")
	ErrDuplicate             = newRepoError("vi phạm ràng buộc duy nhất")
	ErrNoRowsAffected        = newRepoError("không có dòng nào bị tác động")
	ErrDuplicateVersionLabel = newRepoError("label version đã tồn tại")
)

type repoError struct{ msg string }

func newRepoError(msg string) *repoError { return &repoError{msg: msg} }
func (e *repoError) Error() string       { return "project repository: " + e.msg }
