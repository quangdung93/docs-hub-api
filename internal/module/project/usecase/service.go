// Package usecase điều phối project local và dataset tương ứng trên RAGFlow.
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
	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/common/errcode"
	"github.com/quangdung93/docs-hub-api/internal/common/port"
	"github.com/quangdung93/docs-hub-api/internal/module/project/domain"
)

var (
	projectCodePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{1,63}$`) //nolint:gochecknoglobals
	datasetPartPattern = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)                  //nolint:gochecknoglobals
)

type Service struct {
	repo          domain.Repository
	tx            port.TxManager
	rag           port.RAGClient
	clock         port.Clock
	datasetPrefix string
}

type CreateInput struct {
	Code        string
	Name        string
	Description string
}

func New(repo domain.Repository, tx port.TxManager, rag port.RAGClient, clock port.Clock, datasetPrefix string) *Service {
	return &Service{repo: repo, tx: tx, rag: rag, clock: clock, datasetPrefix: datasetPrefix}
}

// Create tạo dataset RAGFlow trước, rồi atomically lưu project, owner membership
// và audit local. Nếu transaction local thất bại, dataset vừa tạo được bù trừ.
func (s *Service) Create(ctx context.Context, in CreateInput) (*domain.Project, error) {
	actor, ok := contextx.ActorFrom(ctx)
	ownerID, parseErr := uuid.Parse(actor.UserID)
	if !ok || parseErr != nil {
		return nil, apperr.Unauthorized("Thiếu thông tin người dùng xác thực")
	}
	in.Code = strings.TrimSpace(in.Code)
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	if !projectCodePattern.MatchString(in.Code) {
		return nil, apperr.BadRequest("Code phải dài 2-64 ký tự và chỉ gồm chữ, số, _ hoặc -")
	}
	if in.Name == "" || len([]rune(in.Name)) > 255 {
		return nil, apperr.BadRequest("Name là bắt buộc và không vượt quá 255 ký tự")
	}
	exists, err := s.repo.CodeExists(ctx, in.Code)
	if err != nil {
		return nil, apperr.Database("Không thể kiểm tra mã project").WithCause(err)
	}
	if exists {
		return nil, duplicateCodeError(in.Code)
	}
	if s.rag == nil {
		return nil, apperr.External("RAGFlow chưa được cấu hình")
	}

	now := s.clock.Now().UTC()
	project := &domain.Project{
		ID: uuid.New(), Code: in.Code, Name: in.Name, Description: in.Description,
		Status: domain.StatusActive, OwnerID: ownerID, Version: 1,
		RAGFlowSyncStatus: "ready", CreatedAt: now, UpdatedAt: now,
	}
	dataset, err := s.rag.CreateDataset(ctx, s.datasetName(project.ID), s.datasetDescription(project))
	if err != nil {
		return nil, apperr.External("Không thể tạo dataset cho project trên RAGFlow").WithCause(err)
	}
	project.RAGFlowDatasetID = dataset.ID

	err = s.tx.Do(ctx, func(txctx context.Context) error {
		return s.repo.Create(txctx, domain.CreateParams{Project: project, RequestID: contextx.RequestID(ctx)})
	})
	if err == nil {
		return project, nil
	}
	// Request có thể vừa bị hủy do DB timeout; dùng context tách hủy có giới hạn
	// để vẫn cố gắng dọn dataset mồ côi trên RAGFlow.
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	cleanupErr := s.rag.DeleteDatasets(cleanupCtx, []string{dataset.ID})
	if errors.Is(err, domain.ErrDuplicateCode) {
		return nil, duplicateCodeError(in.Code).WithCause(errors.Join(err, cleanupErr))
	}
	return nil, apperr.Database("Không thể lưu project").WithCause(errors.Join(err, cleanupErr))
}

func (s *Service) datasetName(projectID uuid.UUID) string {
	prefix := strings.Trim(datasetPartPattern.ReplaceAllString(strings.TrimSpace(s.datasetPrefix), "_"), "_-")
	if prefix == "" {
		prefix = "docs_hub"
	}
	return prefix + "_" + strings.ReplaceAll(projectID.String(), "-", "")
}

func (*Service) datasetDescription(project *domain.Project) string {
	return fmt.Sprintf("Docs Hub project %s (%s), local_project_id=%s", project.Name, project.Code, project.ID)
}

func duplicateCodeError(code string) *apperr.BusinessError {
	return apperr.NewBusiness(errcode.ProjectCodeExists, "Mã project đã tồn tại", false).
		WithDetails(map[string]string{"field": "code", "value": code})
}
