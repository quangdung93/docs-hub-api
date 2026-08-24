// Package usecase chứa service tầng nghiệp vụ của module project: điều phối
// repository dự án + repository thành viên, transaction. Trả BusinessError cho
// lỗi nghiệp vụ (không panic). KHÔNG import gin/gorm (depguard bảo vệ).
package usecase

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/errcode"
	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	"github.com/quangdung93/docs-hub-api/internal/common/port"
	"github.com/quangdung93/docs-hub-api/internal/module/project/domain"
)

// Service là API nghiệp vụ của module project mà tầng delivery gọi.
type Service interface {
	Create(ctx context.Context, in CreateProjectInput) (*domain.Project, error)
	List(ctx context.Context, userID uuid.UUID, page pagination.Query) ([]domain.Project, pagination.Meta, error)
	// GetByID lấy chi tiết một dự án. Không có thì trả ProjectNotFound.
	GetByID(ctx context.Context, projectID uuid.UUID) (*domain.Project, error)
	// Stats đếm tài liệu/thành viên/chunk cho nhiều dự án trong một truy vấn.
	Stats(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.ProjectStats, error)
	Update(ctx context.Context, in UpdateProjectInput) (*domain.Project, error)
	Delete(ctx context.Context, in DeleteProjectInput) error
	CreateVersion(ctx context.Context, in CreateVersionInput) (*domain.ProjectVersion, error)
	ListVersions(ctx context.Context, projectID uuid.UUID, page pagination.Query) ([]domain.ProjectVersion, pagination.Meta, error)

	ListMembers(ctx context.Context, projectID uuid.UUID) ([]domain.MemberWithUser, error)
	InviteMember(ctx context.Context, in InviteMemberInput) (*domain.ProjectMember, error)
	ChangeMemberRole(ctx context.Context, in ChangeMemberRoleInput) (*domain.ProjectMember, error)
	RemoveMember(ctx context.Context, projectID, userID uuid.UUID) error
	AcceptInvite(ctx context.Context, projectID, userID uuid.UUID) (*domain.ProjectMember, error)

	// RequestAvatarUpload validate định dạng/dung lượng ảnh và sinh presigned
	// PUT URL để client tự upload thẳng lên storage.
	RequestAvatarUpload(ctx context.Context, in RequestAvatarUploadInput) (*RequestAvatarUploadResult, error)
	// CompleteAvatarUpload xác nhận ảnh đã thực sự lên storage rồi mới gán
	// vào dự án. Idempotent: gọi lại sau khi đã gán không lỗi.
	CompleteAvatarUpload(ctx context.Context, projectID uuid.UUID) (*domain.Project, error)
	// AvatarURL sinh presigned GET URL cho ảnh đại diện của project (rỗng nếu
	// project chưa có ảnh). Không có I/O mạng — chỉ ký URL cục bộ.
	AvatarURL(ctx context.Context, project *domain.Project) (string, error)
}

// Deps gom phụ thuộc của service (struct thay vì nhiều tham số rời).
type Deps struct {
	ProjectRepo        domain.ProjectRepository
	MemberRepo         domain.ProjectMemberRepository
	VersionRepo        domain.ProjectVersionRepository
	Tx                 port.TxManager
	Clock              port.Clock
	RAG                port.RAGClient
	DatasetPrefix      string
	ObjectStore        port.ObjectStore
	AvatarMaxBytes     int64
	AvatarPresignedTTL time.Duration
}

// service là implementation của Service.
type service struct {
	projectRepo        domain.ProjectRepository
	memberRepo         domain.ProjectMemberRepository
	versionRepo        domain.ProjectVersionRepository
	tx                 port.TxManager
	clock              port.Clock
	rag                port.RAGClient
	datasetPrefix      string
	objectStore        port.ObjectStore
	avatarMaxBytes     int64
	avatarPresignedTTL time.Duration
}

// NewService dựng service từ Deps.
func NewService(d Deps) Service {
	return &service{
		projectRepo:        d.ProjectRepo,
		memberRepo:         d.MemberRepo,
		versionRepo:        d.VersionRepo,
		tx:                 d.Tx,
		clock:              d.Clock,
		rag:                d.RAG,
		datasetPrefix:      d.DatasetPrefix,
		objectStore:        d.ObjectStore,
		avatarMaxBytes:     d.AvatarMaxBytes,
		avatarPresignedTTL: d.AvatarPresignedTTL,
	}
}

// --- Input types (không phải DTO HTTP; DTO nằm ở tầng delivery) ---

// CreateProjectInput là tham số tạo dự án.
type CreateProjectInput struct {
	OwnerID     uuid.UUID
	Code        string
	Name        string
	Description string
	Settings    domain.ProjectSettings
}

var (
	projectCodePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{1,63}$`) //nolint:gochecknoglobals
	datasetPartPattern = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)                  //nolint:gochecknoglobals
)

// UpdateProjectInput là tham số cập nhật dự án. Con trỏ nil = giữ nguyên (PATCH).
type UpdateProjectInput struct {
	ID          uuid.UUID
	Name        *string
	Description *string
	Settings    *domain.ProjectSettings
}

// DeleteProjectInput là tham số xóa dự án. ConfirmName phải khớp CHÍNH XÁC tên
// hiện tại của dự án (BR SRS: xóa dự án không thể hoàn tác, cần xác nhận).
type DeleteProjectInput struct {
	ID          uuid.UUID
	ConfirmName string
}

// InviteMemberInput là tham số mời thành viên mới.
type InviteMemberInput struct {
	ProjectID uuid.UUID
	UserID    uuid.UUID
	Role      domain.Role
}

// RequestAvatarUploadInput là tham số xin phép upload ảnh đại diện dự án.
type RequestAvatarUploadInput struct {
	ProjectID uuid.UUID
	MimeType  string
	SizeBytes int64
}

// RequestAvatarUploadResult là kết quả xin phép upload: URL để client tự PUT
// ảnh lên storage.
type RequestAvatarUploadResult struct {
	UploadURL string
	ExpiresAt time.Time
}

// ChangeMemberRoleInput là tham số đổi vai trò thành viên.
type ChangeMemberRoleInput struct {
	ProjectID uuid.UUID
	UserID    uuid.UUID
	Role      domain.Role
}

// isRepoErr kiểm tra err có khớp sentinel của repository không (dùng errors.Is
// để xuyên qua lỗi bọc).
func isRepoErr(err, target error) bool {
	return errors.Is(err, target)
}

// Create tạo dự án mới và tự động thêm owner làm thành viên active đầu tiên,
// trong cùng một transaction.
func (s *service) Create(ctx context.Context, in CreateProjectInput) (*domain.Project, error) {
	project := domain.NewProject(in.OwnerID, in.Name, in.Description, in.Settings)
	if code := strings.TrimSpace(in.Code); code != "" {
		if !projectCodePattern.MatchString(code) {
			return nil, apperr.BadRequest("Code phải dài 2-64 ký tự và chỉ gồm chữ, số, _ hoặc -")
		}
		project.Code = code
	}
	project.RAGFlowSyncStatus = "disabled"
	if s.rag != nil {
		dataset, ragErr := s.rag.CreateDataset(ctx, s.datasetName(project.ID), s.datasetDescription(project))
		if ragErr != nil {
			return nil, apperr.External("Không thể tạo dataset cho project trên RAGFlow").WithCause(ragErr)
		}
		project.RAGFlowDatasetID = dataset.ID
		project.RAGFlowSyncStatus = "ready"
	}
	owner := domain.NewInvite(project.ID, in.OwnerID, domain.RoleOwner)
	now := s.clock.Now()
	project.CreatedAt = now
	project.UpdatedAt = now
	owner.Status = domain.MemberStatusActive
	owner.JoinedAt = &now

	err := s.tx.Do(ctx, func(txCtx context.Context) error {
		if createErr := s.projectRepo.Create(txCtx, project); createErr != nil {
			return fmt.Errorf("tạo dự án: %w", createErr)
		}
		if createErr := s.memberRepo.Create(txCtx, owner); createErr != nil {
			return fmt.Errorf("thêm owner làm thành viên: %w", createErr)
		}
		return nil
	})
	if err != nil {
		if s.rag != nil && project.RAGFlowDatasetID != "" {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			cleanupErr := s.rag.DeleteDatasets(cleanupCtx, []string{project.RAGFlowDatasetID})
			err = errors.Join(err, cleanupErr)
		}
		if errors.Is(err, domain.ErrDuplicate) {
			return nil, apperr.NewBusiness(errcode.ProjectCodeExists, "Mã project đã tồn tại", false).
				WithDetails(map[string]string{"field": "code", "value": project.Code}).WithCause(err)
		}
		return nil, apperr.Database("Lỗi tạo dự án").WithCause(err)
	}
	return project, nil
}

func (s *service) datasetName(projectID uuid.UUID) string {
	prefix := strings.Trim(datasetPartPattern.ReplaceAllString(strings.TrimSpace(s.datasetPrefix), "_"), "_-")
	if prefix == "" {
		prefix = "docs_hub"
	}
	return prefix + "_" + strings.ReplaceAll(projectID.String(), "-", "")
}

func (*service) datasetDescription(project *domain.Project) string {
	return fmt.Sprintf("Docs Hub project %s (%s), local_project_id=%s", project.Name, project.Code, project.ID)
}

// List liệt kê dự án mà user là owner hoặc thành viên active.
func (s *service) List(
	ctx context.Context, userID uuid.UUID, page pagination.Query,
) ([]domain.Project, pagination.Meta, error) {
	p := page.Normalize()
	projects, total, err := s.projectRepo.ListForUser(ctx, userID, p)
	if err != nil {
		return nil, pagination.Meta{}, apperr.Database("Lỗi truy vấn danh sách dự án").WithCause(err)
	}
	meta := pagination.NewMeta(p.Page, p.Limit, total)
	return projects, meta, nil
}

// Update cập nhật thông tin chung/cấu hình dự án.
func (s *service) Update(ctx context.Context, in UpdateProjectInput) (*domain.Project, error) {
	project, err := s.projectRepo.FindByID(ctx, in.ID)
	if err != nil {
		if isRepoErr(err, domain.ErrNotFound) {
			return nil, domain.ErrProjectNotFound()
		}
		return nil, apperr.Database("Lỗi truy vấn dự án").WithCause(err)
	}

	project.ChangeProfile(in.Name, in.Description, in.Settings)

	if updErr := s.projectRepo.Update(ctx, project); updErr != nil {
		if isRepoErr(updErr, domain.ErrNotFound) {
			return nil, domain.ErrProjectNotFound()
		}
		return nil, apperr.Database("Lỗi cập nhật dự án").WithCause(updErr)
	}
	return project, nil
}

// avatarStorageKey dựng object key CỐ ĐỊNH cho ảnh đại diện của 1 dự án (1
// slot duy nhất/project) — upload lại chỉ ghi đè, không để lại object rác.
func avatarStorageKey(projectID uuid.UUID) string {
	return fmt.Sprintf("projects/%s/avatar", projectID)
}

// RequestAvatarUpload validate rồi sinh presigned PUT URL cho ảnh đại diện.
// Dự án phải đã tồn tại (được tạo trước qua Create) — ảnh đại diện luôn là
// bước SAU khi có project ID.
func (s *service) RequestAvatarUpload(
	ctx context.Context, in RequestAvatarUploadInput,
) (*RequestAvatarUploadResult, error) {
	if !domain.IsAvatarFormatAllowed(in.MimeType) {
		return nil, domain.ErrInvalidAvatarFormat
	}
	if in.SizeBytes > s.avatarMaxBytes {
		return nil, domain.ErrAvatarTooLarge
	}

	if _, err := s.projectRepo.FindByID(ctx, in.ProjectID); err != nil {
		if isRepoErr(err, domain.ErrNotFound) {
			return nil, domain.ErrProjectNotFound()
		}
		return nil, apperr.Database("Lỗi truy vấn dự án").WithCause(err)
	}

	uploadURL, err := s.objectStore.PresignedPutURL(ctx, avatarStorageKey(in.ProjectID), s.avatarPresignedTTL)
	if err != nil {
		return nil, apperr.External("Lỗi tạo URL upload ảnh đại diện").WithCause(err)
	}

	return &RequestAvatarUploadResult{
		UploadURL: uploadURL,
		ExpiresAt: s.clock.Now().Add(s.avatarPresignedTTL),
	}, nil
}

// CompleteAvatarUpload xác nhận ảnh đã thực sự tồn tại trong storage rồi mới
// gán vào dự án — không tin tưởng mù quáng lời gọi từ client.
func (s *service) CompleteAvatarUpload(ctx context.Context, projectID uuid.UUID) (*domain.Project, error) {
	project, err := s.projectRepo.FindByID(ctx, projectID)
	if err != nil {
		if isRepoErr(err, domain.ErrNotFound) {
			return nil, domain.ErrProjectNotFound()
		}
		return nil, apperr.Database("Lỗi truy vấn dự án").WithCause(err)
	}

	key := avatarStorageKey(projectID)
	if _, statErr := s.objectStore.Stat(ctx, key); statErr != nil {
		if errors.Is(statErr, port.ErrObjectNotFound) {
			return nil, domain.ErrAvatarNotUploaded
		}
		return nil, apperr.External("Lỗi kiểm tra ảnh trên storage").WithCause(statErr)
	}

	project.SetAvatar(key)
	if updErr := s.projectRepo.Update(ctx, project); updErr != nil {
		if isRepoErr(updErr, domain.ErrNotFound) {
			return nil, domain.ErrProjectNotFound()
		}
		return nil, apperr.Database("Lỗi cập nhật ảnh đại diện").WithCause(updErr)
	}
	return project, nil
}

// AvatarURL sinh presigned GET URL cho ảnh đại diện (rỗng nếu chưa có ảnh).
func (s *service) AvatarURL(ctx context.Context, project *domain.Project) (string, error) {
	if project.AvatarKey == "" {
		return "", nil
	}
	url, err := s.objectStore.PresignedGetURL(ctx, project.AvatarKey, s.avatarPresignedTTL)
	if err != nil {
		return "", apperr.External("Lỗi tạo URL ảnh đại diện").WithCause(err)
	}
	return url, nil
}

// Delete xóa CỨNG dự án (DB tự cascade sang project_members). RBAC (chỉ owner)
// đã được middleware chặn trước khi vào tới đây. BR SRS: xóa dự án không thể
// hoàn tác -> bắt buộc ConfirmName khớp CHÍNH XÁC tên hiện tại của dự án,
// không khớp -> ErrConfirmNameMismatch (nghiệp vụ), không xóa.
func (s *service) Delete(ctx context.Context, in DeleteProjectInput) error {
	project, err := s.projectRepo.FindByID(ctx, in.ID)
	if err != nil {
		if isRepoErr(err, domain.ErrNotFound) {
			return domain.ErrProjectNotFound()
		}
		return apperr.Database("Lỗi truy vấn dự án").WithCause(err)
	}
	if project.Name != in.ConfirmName {
		return domain.ErrConfirmNameMismatch
	}

	if delErr := s.projectRepo.Delete(ctx, in.ID); delErr != nil {
		if isRepoErr(delErr, domain.ErrNotFound) {
			return domain.ErrProjectNotFound()
		}
		return apperr.Database("Lỗi xóa dự án").WithCause(delErr)
	}
	return nil
}

// GetByID lấy chi tiết một dự án.
func (s *service) GetByID(ctx context.Context, projectID uuid.UUID) (*domain.Project, error) {
	p, err := s.projectRepo.FindByID(ctx, projectID)
	if err != nil {
		if isRepoErr(err, domain.ErrNotFound) {
			return nil, domain.ErrProjectNotFound()
		}
		return nil, apperr.Database("Lỗi truy vấn dự án").WithCause(err)
	}
	return p, nil
}

// Stats trả về các con số tổng hợp của dự án.
func (s *service) Stats(
	ctx context.Context, ids []uuid.UUID,
) (map[uuid.UUID]domain.ProjectStats, error) {
	stats, err := s.projectRepo.Stats(ctx, ids)
	if err != nil {
		return nil, apperr.Database("Lỗi đếm dữ liệu dự án").WithCause(err)
	}
	return stats, nil
}

// ListMembers liệt kê thành viên (mọi trạng thái) của dự án.
func (s *service) ListMembers(ctx context.Context, projectID uuid.UUID) ([]domain.MemberWithUser, error) {
	members, err := s.memberRepo.ListByProject(ctx, projectID)
	if err != nil {
		return nil, apperr.Database("Lỗi truy vấn danh sách thành viên").WithCause(err)
	}
	return members, nil
}

// InviteMember mời một user vào dự án (status = pending). User đã là thành
// viên (hoặc đang có lời mời) -> ErrAlreadyMember (nghiệp vụ).
func (s *service) InviteMember(ctx context.Context, in InviteMemberInput) (*domain.ProjectMember, error) {
	_, err := s.memberRepo.FindByProjectAndUser(ctx, in.ProjectID, in.UserID)
	if err == nil {
		return nil, domain.ErrAlreadyMember
	}
	if !isRepoErr(err, domain.ErrNotFound) {
		return nil, apperr.Database("Lỗi kiểm tra thành viên").WithCause(err)
	}

	member := domain.NewInvite(in.ProjectID, in.UserID, in.Role)
	if createErr := s.memberRepo.Create(ctx, member); createErr != nil {
		if isRepoErr(createErr, domain.ErrDuplicate) {
			return nil, domain.ErrAlreadyMember
		}
		return nil, apperr.Database("Lỗi mời thành viên").WithCause(createErr)
	}
	return member, nil
}

// ChangeMemberRole đổi vai trò của một thành viên đã tồn tại. Không cho đổi
// role của owner qua API này (xem ensureNotOwner).
func (s *service) ChangeMemberRole(ctx context.Context, in ChangeMemberRoleInput) (*domain.ProjectMember, error) {
	if err := s.ensureNotOwner(ctx, in.ProjectID, in.UserID); err != nil {
		return nil, err
	}

	if updErr := s.memberRepo.UpdateRole(ctx, in.ProjectID, in.UserID, in.Role); updErr != nil {
		if isRepoErr(updErr, domain.ErrNoRowsAffected) || isRepoErr(updErr, domain.ErrNotFound) {
			return nil, domain.ErrMemberNotFound()
		}
		return nil, apperr.Database("Lỗi đổi vai trò thành viên").WithCause(updErr)
	}

	member, err := s.memberRepo.FindByProjectAndUser(ctx, in.ProjectID, in.UserID)
	if err != nil {
		return nil, apperr.Database("Lỗi tải lại thành viên").WithCause(err)
	}
	return member, nil
}

// RemoveMember gỡ một thành viên khỏi dự án. Không cho gỡ owner qua API này
// (xem ensureNotOwner).
func (s *service) RemoveMember(ctx context.Context, projectID, userID uuid.UUID) error {
	if err := s.ensureNotOwner(ctx, projectID, userID); err != nil {
		return err
	}

	if err := s.memberRepo.Delete(ctx, projectID, userID); err != nil {
		if isRepoErr(err, domain.ErrNotFound) {
			return domain.ErrMemberNotFound()
		}
		return apperr.Database("Lỗi gỡ thành viên").WithCause(err)
	}
	return nil
}

// ensureNotOwner chặn đổi role/gỡ owner qua API quản lý thành viên — owner
// luôn phải có đúng 1 hàng role=owner trong project_members (khớp
// projects.owner_id), nếu không sẽ tự khóa quyền quản lý dự án của owner
// (RequireProjectRole tra role qua project_members, không qua projects.owner_id).
// Muốn đổi chủ dự án cần API transfer ownership riêng (chưa hiện thực).
func (s *service) ensureNotOwner(ctx context.Context, projectID, userID uuid.UUID) error {
	member, err := s.memberRepo.FindByProjectAndUser(ctx, projectID, userID)
	if err != nil {
		if isRepoErr(err, domain.ErrNotFound) {
			return domain.ErrMemberNotFound()
		}
		return apperr.Database("Lỗi kiểm tra thành viên").WithCause(err)
	}
	if member.Role == domain.RoleOwner {
		return domain.ErrCannotModifyOwner
	}
	return nil
}

// AcceptInvite xác nhận lời mời của chính user hiện tại: pending -> active.
func (s *service) AcceptInvite(ctx context.Context, projectID, userID uuid.UUID) (*domain.ProjectMember, error) {
	member, err := s.memberRepo.FindByProjectAndUser(ctx, projectID, userID)
	if err != nil {
		if isRepoErr(err, domain.ErrNotFound) {
			return nil, domain.ErrMemberNotFound()
		}
		return nil, apperr.Database("Lỗi truy vấn lời mời").WithCause(err)
	}

	if acceptErr := member.Accept(s.clock.Now()); acceptErr != nil {
		return nil, acceptErr
	}

	if updErr := s.memberRepo.UpdateStatus(ctx, member); updErr != nil {
		return nil, apperr.Database("Lỗi xác nhận lời mời").WithCause(updErr)
	}
	return member, nil
}
