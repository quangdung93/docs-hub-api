package usecase

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/common/errcode"
	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	"github.com/quangdung93/docs-hub-api/internal/module/project/domain"
)

// CreateVersionInput là dữ liệu tạo một draft version mới.
type CreateVersionInput struct {
	ProjectID uuid.UUID
	Label     string
}

// CreateVersion tạo draft version trong PostgreSQL. Version dùng chung dataset
// RAGFlow của project; chỉ document revision mới có remote document reference.
func (s *service) CreateVersion(ctx context.Context, input CreateVersionInput) (*domain.ProjectVersion, error) {
	actorID, err := s.authorizeProject(ctx, input.ProjectID, true)
	if err != nil {
		return nil, err
	}
	input.Label = strings.TrimSpace(input.Label)
	if input.Label == "" || len([]rune(input.Label)) > 100 {
		return nil, apperr.BadRequest("Label version là bắt buộc và không vượt quá 100 ký tự")
	}
	now := s.clock.Now().UTC()
	version := &domain.ProjectVersion{
		ID: uuid.New(), ProjectID: input.ProjectID, Label: input.Label,
		Status: domain.StatusDraft, CreatedBy: actorID, CreatedAt: now, UpdatedAt: now,
	}
	if s.versionRepo == nil {
		return nil, apperr.Database("Project version repository chưa được cấu hình")
	}
	err = s.tx.Do(ctx, func(txctx context.Context) error {
		return s.versionRepo.Create(txctx, version, contextx.RequestID(ctx))
	})
	if errors.Is(err, domain.ErrDuplicateVersionLabel) {
		return nil, apperr.NewBusiness(errcode.VersionLabelExists, "Label version đã tồn tại", false).
			WithDetails(map[string]string{"field": "label", "value": input.Label}).WithCause(err)
	}
	if errors.Is(err, domain.ErrNotFound) {
		return nil, apperr.NotFound(errcode.NotFound, "Project không tồn tại").WithCause(err)
	}
	if err != nil {
		return nil, apperr.Database("Không thể tạo project version").WithCause(err)
	}
	return version, nil
}

// ListVersions trả timeline theo sequence_no giảm dần và chỉ cho thành viên project.
func (s *service) ListVersions(
	ctx context.Context, projectID uuid.UUID, page pagination.Query,
) ([]domain.ProjectVersion, pagination.Meta, error) {
	if _, err := s.authorizeProject(ctx, projectID, false); err != nil {
		return nil, pagination.Meta{}, err
	}
	page = page.Normalize()
	if s.versionRepo == nil {
		return nil, pagination.Meta{}, apperr.Database("Project version repository chưa được cấu hình")
	}
	versions, total, err := s.versionRepo.List(ctx, projectID, page)
	if err != nil {
		return nil, pagination.Meta{}, apperr.Database("Không thể đọc danh sách project version").WithCause(err)
	}
	return versions, pagination.NewMeta(page.Page, page.Limit, total), nil
}

func (s *service) authorizeProject(ctx context.Context, projectID uuid.UUID, write bool) (uuid.UUID, error) {
	actor, ok := contextx.ActorFrom(ctx)
	if !ok {
		return uuid.Nil, apperr.Unauthorized("Thiếu thông tin người dùng xác thực")
	}
	actorID, err := uuid.Parse(actor.UserID)
	if err != nil {
		return uuid.Nil, apperr.Unauthorized("Thiếu thông tin người dùng xác thực")
	}
	member, err := s.memberRepo.FindByProjectAndUser(ctx, projectID, actorID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return uuid.Nil, apperr.Forbidden("Bạn không có quyền thực hiện thao tác này trong project")
		}
		return uuid.Nil, apperr.Database("Không thể kiểm tra quyền project").WithCause(err)
	}
	if member.Status != domain.MemberStatusActive ||
		(write && member.Role != domain.RoleOwner && member.Role != domain.RoleEditor) {
		return uuid.Nil, apperr.Forbidden("Bạn không có quyền thực hiện thao tác này trong project")
	}
	return actorID, nil
}
