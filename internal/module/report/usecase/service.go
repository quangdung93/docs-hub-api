// Package usecase điều phối sinh báo cáo dự án (UAT/Planning/Testcase) bằng
// RAGFlow, không phụ thuộc HTTP/GORM.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/common/errcode"
	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	"github.com/quangdung93/docs-hub-api/internal/common/port"
	"github.com/quangdung93/docs-hub-api/internal/module/report/domain"
	retrievaldomain "github.com/quangdung93/docs-hub-api/internal/module/retrieval/domain"
)

// maxUATItems giới hạn số dòng report UAT — template chừa sẵn dòng 12..200 cho
// Round 1 (công thức COUNTIF trong sheet Report 1_Module dựa vào range đó).
// Khác UAT export cũ (module document): ở đây KHÔNG lỗi khi vượt, chỉ cắt bớt,
// vì nội dung do LLM sinh (không phải danh sách tài liệu cố định của người dùng).
const maxUATItems = 188

const downloadURLTTL = 15 * time.Minute

// scopeMetadataKey PHẢI khớp với chat/usecase/service.go (cùng gắn/lọc
// metadata trên CÙNG tập document RAGFlow của project).
const scopeMetadataKey = "docs_hub_scope_id"

// ScopeRepository giải quyết version/change_request thành phạm vi tài liệu cụ
// thể — implement bởi retrieval/repository.New(db) (tái dùng y hệt module chat,
// xem chat/module.go), không viết SQL mới.
type ScopeRepository interface {
	ResolveScope(ctx context.Context, projectID uuid.UUID, scope retrievaldomain.Scope) ([]retrievaldomain.ResolvedScope, error)
	RevisionRefs(ctx context.Context, projectID uuid.UUID, scope retrievaldomain.Scope) ([]retrievaldomain.RevisionRef, error)
}

type Service struct {
	repo             domain.Repository
	scopeRepo        ScopeRepository
	tx               port.TxManager
	rag              port.RAGClient
	store            port.ObjectStore
	clock            port.Clock
	bypassProjectACL bool
}

// Option cấu hình hành vi tùy môi trường cho report service.
type Option func(*Service)

// WithProjectACLBypass chỉ dành cho local development; production không được bật.
func WithProjectACLBypass(enabled bool) Option {
	return func(service *Service) { service.bypassProjectACL = enabled }
}

func New(
	repo domain.Repository, scopeRepo ScopeRepository, tx port.TxManager, rag port.RAGClient,
	store port.ObjectStore, clock port.Clock, options ...Option,
) *Service {
	service := &Service{repo: repo, scopeRepo: scopeRepo, tx: tx, rag: rag, store: store, clock: clock}
	for _, option := range options {
		option(service)
	}
	return service
}

// GenerateInput là tham số xuất báo cáo. VersionID/ChangeRequestID để trống
// (nil cả hai) = lấy toàn bộ tài liệu mới nhất của project; chỉ được chọn
// đúng 1 trong 2, không hỗ trợ chọn nhiều version/CR cùng lúc.
type GenerateInput struct {
	ProjectID       uuid.UUID
	ReportType      string
	Format          string
	VersionID       *uuid.UUID
	ChangeRequestID *uuid.UUID
}

type GenerateResult struct {
	Report      domain.Report `json:"report"`
	DownloadURL string        `json:"download_url"`
}

// Generate sinh báo cáo dự án (UAT Report — tiêu chí nghiệm thu, Project
// Planning, hoặc Testcase Report — SRS v1.1 mục IX/X) bằng cách nhờ RAGFlow
// tổng hợp nội dung tài liệu dự án (toàn bộ hoặc giới hạn theo version/change
// request), điền vào template chuẩn ISC tương ứng, lưu file + lịch sử.
//
// BR SRS: "Chỉ Editor trở lên được phép xuất báo cáo" — authorize(..., write=true).
func (s *Service) Generate(ctx context.Context, in GenerateInput) (*GenerateResult, error) {
	actorID, err := s.authorize(ctx, in.ProjectID, true)
	if err != nil {
		return nil, err
	}
	format, err := normalizeFormat(in.Format)
	if err != nil {
		return nil, err
	}
	scope, err := buildScope(in)
	if err != nil {
		return nil, err
	}
	resolved, refs, err := s.resolveScope(ctx, in.ProjectID, scope)
	if err != nil {
		return nil, err
	}
	if len(refs) == 0 {
		return nil, apperr.BadRequest("Không có tài liệu nào trong phạm vi đã chọn")
	}
	reportID := uuid.New()
	content, contentType, items, err := s.generateContent(
		ctx, in.ProjectID, in.ReportType, format, reportID, scope, refs, resolved)
	if err != nil {
		return nil, err
	}
	return s.persistReport(ctx, in.ProjectID, actorID, reportID, in.ReportType, format, contentType, content, items)
}

// buildScope dựng scope từ input — đúng 1 trong VersionID/ChangeRequestID
// hoặc để trống cả hai (toàn bộ project). Dùng retrievaldomain.Scope với mảng
// 1 phần tử để tái dùng nguyên ResolveScope/RevisionRefs của module retrieval,
// dù report chỉ cho chọn 1 giá trị/lần gọi (không multi-select như /search).
func buildScope(in GenerateInput) (retrievaldomain.Scope, error) {
	switch {
	case in.VersionID != nil && in.ChangeRequestID != nil:
		return retrievaldomain.Scope{}, apperr.BadRequest("Chỉ chọn version hoặc change request, không thể cả hai")
	case in.VersionID != nil:
		return retrievaldomain.Scope{Mode: retrievaldomain.ScopeVersions, VersionIDs: []uuid.UUID{*in.VersionID}}, nil
	case in.ChangeRequestID != nil:
		return retrievaldomain.Scope{
			Mode: retrievaldomain.ScopeChangeRequests, ChangeRequestIDs: []uuid.UUID{*in.ChangeRequestID},
		}, nil
	default:
		return retrievaldomain.Scope{Mode: retrievaldomain.ScopeAll}, nil
	}
}

// resolveScope kiểm tra version/change request thuộc đúng project rồi lấy
// danh sách revision khớp scope — copy-adapt từ chat/usecase/service.go
// (resolveScope), lỗi NotFound nếu ID không resolve được.
func (s *Service) resolveScope(
	ctx context.Context, projectID uuid.UUID, scope retrievaldomain.Scope,
) ([]retrievaldomain.ResolvedScope, []retrievaldomain.RevisionRef, error) {
	resolved, err := s.scopeRepo.ResolveScope(ctx, projectID, scope)
	if err != nil {
		return nil, nil, apperr.Database("Không thể kiểm tra scope").WithCause(err)
	}
	expected := len(scope.VersionIDs) + len(scope.ChangeRequestIDs)
	if scope.Mode != retrievaldomain.ScopeAll && len(resolved) != expected {
		return nil, nil, apperr.NotFound(errcode.NotFound, "Không tìm thấy version hoặc change request")
	}
	refs, err := s.scopeRepo.RevisionRefs(ctx, projectID, scope)
	if err != nil {
		return nil, nil, apperr.Database("Không thể đọc revision mapping").WithCause(err)
	}
	return resolved, refs, nil
}

// generateContent điều phối theo report_type: nhờ RAGFlow tổng hợp nội dung
// rồi render ra file — mỗi loại có prompt/schema/template riêng.
func (s *Service) generateContent(
	ctx context.Context, projectID uuid.UUID, reportType, format string, reportID uuid.UUID,
	scope retrievaldomain.Scope, refs []retrievaldomain.RevisionRef, resolved []retrievaldomain.ResolvedScope,
) ([]byte, string, []domain.ReportItem, error) {
	switch reportType {
	case domain.ReportTypeUAT:
		return s.generateUATContent(ctx, projectID, format, reportID, scope, refs, resolved)
	case domain.ReportTypePlanning:
		return s.generatePlanningContent(ctx, projectID, format, reportID, scope, refs, resolved)
	case domain.ReportTypeTestcase:
		return s.generateTestcaseContent(ctx, projectID, format, reportID, scope, refs, resolved)
	default:
		return nil, "", nil, apperr.BadRequest("Loại báo cáo không hợp lệ (hỗ trợ: uat, planning, testcase)")
	}
}

func (s *Service) generateUATContent(
	ctx context.Context, projectID uuid.UUID, format string, reportID uuid.UUID,
	scope retrievaldomain.Scope, refs []retrievaldomain.RevisionRef, resolved []retrievaldomain.ResolvedScope,
) ([]byte, string, []domain.ReportItem, error) {
	title, items, err := s.fetchUATItems(ctx, projectID, scope, refs, resolved)
	if err != nil {
		return nil, "", nil, err
	}
	content, contentType, err := renderUATContent(format, uatContentInput{ProjectName: title, Items: items})
	if err != nil {
		return nil, "", nil, apperr.Internal("Không thể tạo file report").WithCause(err)
	}
	return content, contentType, toUATReportItems(reportID, items), nil
}

func (s *Service) generatePlanningContent(
	ctx context.Context, projectID uuid.UUID, format string, reportID uuid.UUID,
	scope retrievaldomain.Scope, refs []retrievaldomain.RevisionRef, resolved []retrievaldomain.ResolvedScope,
) ([]byte, string, []domain.ReportItem, error) {
	title, milestones, err := s.fetchPlanningMilestones(ctx, projectID, scope, refs, resolved)
	if err != nil {
		return nil, "", nil, err
	}
	content, contentType, err := renderPlanningContent(format,
		planningContentInput{ProjectName: title, Milestones: milestones})
	if err != nil {
		return nil, "", nil, apperr.Internal("Không thể tạo file report").WithCause(err)
	}
	return content, contentType, toPlanningReportItems(reportID, milestones), nil
}

func (s *Service) generateTestcaseContent(
	ctx context.Context, projectID uuid.UUID, format string, reportID uuid.UUID,
	scope retrievaldomain.Scope, refs []retrievaldomain.RevisionRef, resolved []retrievaldomain.ResolvedScope,
) ([]byte, string, []domain.ReportItem, error) {
	title, items, err := s.fetchTestcaseItems(ctx, projectID, scope, refs, resolved)
	if err != nil {
		return nil, "", nil, err
	}
	content, contentType, err := renderTestcaseContent(format,
		testcaseContentInput{ProjectName: title, Items: items})
	if err != nil {
		return nil, "", nil, apperr.Internal("Không thể tạo file report").WithCause(err)
	}
	return content, contentType, toTestcaseReportItems(reportID, items), nil
}

// fetchUATItems nhờ RAGFlow tổng hợp User Story/Acceptance Criteria trong tài
// liệu dự án (giới hạn theo scope nếu có) thành danh sách test case UAT.
func (s *Service) fetchUATItems(
	ctx context.Context, projectID uuid.UUID, scope retrievaldomain.Scope,
	refs []retrievaldomain.RevisionRef, resolved []retrievaldomain.ResolvedScope,
) (string, []uatRAGItem, error) {
	title, raw, err := s.completeChat(ctx, projectID, scope, refs, resolved, func(name string) string {
		return uatPrompt(name, maxUATItems)
	})
	if err != nil {
		return "", nil, err
	}
	if noDataAnswer(raw) {
		return "", nil, noDataError(raw)
	}
	items, err := parseUATItems(raw)
	if err != nil {
		return "", nil, apperr.External("RAGFlow trả nội dung không đúng định dạng JSON mong đợi").WithCause(err)
	}
	if len(items) == 0 {
		return "", nil, apperr.BadRequest("Không tìm thấy User Story/Acceptance Criteria nào trong tài liệu dự án")
	}
	if len(items) > maxUATItems {
		items = items[:maxUATItems]
	}
	return title, items, nil
}

// fetchPlanningMilestones nhờ RAGFlow tổng hợp tài liệu dự án thành kế hoạch
// triển khai theo milestone (Project Planning).
func (s *Service) fetchPlanningMilestones(
	ctx context.Context, projectID uuid.UUID, scope retrievaldomain.Scope,
	refs []retrievaldomain.RevisionRef, resolved []retrievaldomain.ResolvedScope,
) (string, []planningRAGMilestone, error) {
	title, raw, err := s.completeChat(ctx, projectID, scope, refs, resolved, planningPrompt)
	if err != nil {
		return "", nil, err
	}
	if noDataAnswer(raw) {
		return "", nil, noDataError(raw)
	}
	milestones, err := parsePlanningMilestones(raw)
	if err != nil {
		return "", nil, apperr.External("RAGFlow trả nội dung không đúng định dạng JSON mong đợi").WithCause(err)
	}
	if len(milestones) == 0 {
		return "", nil, apperr.BadRequest("Không tìm thấy thông tin kế hoạch nào trong tài liệu dự án")
	}
	if len(milestones) > maxPlanningMilestones {
		milestones = milestones[:maxPlanningMilestones]
	}
	return title, milestones, nil
}

// fetchTestcaseItems nhờ RAGFlow sinh danh sách test case chi tiết từ User
// Story/Acceptance Criteria trong tài liệu dự án.
func (s *Service) fetchTestcaseItems(
	ctx context.Context, projectID uuid.UUID, scope retrievaldomain.Scope,
	refs []retrievaldomain.RevisionRef, resolved []retrievaldomain.ResolvedScope,
) (string, []testcaseRAGItem, error) {
	title, raw, err := s.completeChat(ctx, projectID, scope, refs, resolved, func(name string) string {
		return testcasePrompt(name, maxTestcaseItems)
	})
	if err != nil {
		return "", nil, err
	}
	if noDataAnswer(raw) {
		return "", nil, noDataError(raw)
	}
	items, err := parseTestcaseItems(raw)
	if err != nil {
		return "", nil, apperr.External("RAGFlow trả nội dung không đúng định dạng JSON mong đợi").WithCause(err)
	}
	if len(items) == 0 {
		return "", nil, apperr.BadRequest("Không tìm thấy User Story/Acceptance Criteria nào trong tài liệu dự án")
	}
	if len(items) > maxTestcaseItems {
		items = items[:maxTestcaseItems]
	}
	return title, items, nil
}

// completeChat gói chung phần lấy project metadata + đồng bộ scope metadata +
// RAGFlow dataset/chat + gọi CompleteChat — 3 loại report chỉ khác nhau ở
// prompt (buildPrompt nhận TÊN DỰ ÁN THÔ, không phải title đã ghép scope —
// tránh làm rối câu hỏi gửi cho LLM). Trả về title (đã ghép nhãn scope, dùng
// để hiển thị trong file report) + nội dung LLM trả lời.
func (s *Service) completeChat(
	ctx context.Context, projectID uuid.UUID, scope retrievaldomain.Scope,
	refs []retrievaldomain.RevisionRef, resolved []retrievaldomain.ResolvedScope,
	buildPrompt func(projectName string) string,
) (string, string, error) {
	projectName, _, err := s.repo.ProjectMeta(ctx, projectID)
	if err != nil {
		return "", "", s.mapErr(err)
	}
	datasetID, err := s.repo.RAGFlowDatasetID(ctx, projectID)
	if err != nil {
		return "", "", apperr.Database("Không thể đọc RAGFlow dataset mapping").WithCause(err)
	}
	if datasetID == "" {
		return "", "", apperr.External("Project chưa được đồng bộ sang RAGFlow")
	}
	if scope.Mode != retrievaldomain.ScopeAll {
		if err = s.syncScopeMetadata(ctx, datasetID, refs); err != nil {
			return "", "", apperr.External("Không thể đồng bộ scope metadata sang RAGFlow").WithCause(err)
		}
	}
	chatID, err := s.ensureChat(ctx, projectID, datasetID)
	if err != nil {
		return "", "", apperr.External("Không thể chuẩn bị RAGFlow chat assistant").WithCause(err)
	}
	result, err := s.rag.CompleteChat(ctx, port.RAGChatCompletionRequest{
		ChatID: chatID, Messages: []port.RAGChatMessage{{Role: "user", Content: buildPrompt(projectName)}},
		MetadataLogic: "or", MetadataConditions: scopeConditions(scope),
		// Báo cáo KHÔNG cần trích dẫn: luồng này vứt hẳn result.References và chỉ
		// parse result.Content thành JSON, nên "[ID:n]" chỉ là rác làm hỏng parse.
		WantReference: false,
	})
	if err != nil {
		return "", "", apperr.External("RAGFlow chat không khả dụng").WithCause(err)
	}
	return reportTitle(projectName, resolved), result.Content, nil
}

// syncScopeMetadata gắn docs_hub_scope_id/docs_hub_scope_type lên document
// RAGFlow khớp scope — copy-adapt từ chat/usecase/service.go (syncScopeMetadata),
// CHUNG cơ chế với tính năng hỏi-đáp trên cùng tập document.
func (s *Service) syncScopeMetadata(
	ctx context.Context, datasetID string, refs []retrievaldomain.RevisionRef,
) error {
	type group struct {
		scope retrievaldomain.ResolvedScope
		ids   []string
	}
	groups := make(map[uuid.UUID]*group)
	for _, ref := range refs {
		item := groups[ref.Scope.ID]
		if item == nil {
			item = &group{scope: ref.Scope}
			groups[ref.Scope.ID] = item
		}
		item.ids = append(item.ids, ref.RAGFlowDocumentID)
	}
	for _, item := range groups {
		if err := s.rag.UpdateDocumentMetadata(ctx, datasetID, item.ids, map[string]string{
			scopeMetadataKey: item.scope.ID.String(), "docs_hub_scope_type": item.scope.Type,
		}); err != nil {
			return err
		}
	}
	return nil
}

func scopeConditions(scope retrievaldomain.Scope) []port.RAGMetadataCondition {
	if scope.Mode == retrievaldomain.ScopeAll {
		return nil
	}
	ids := append([]uuid.UUID(nil), scope.VersionIDs...)
	ids = append(ids, scope.ChangeRequestIDs...)
	out := make([]port.RAGMetadataCondition, len(ids))
	for i, id := range ids {
		out[i] = port.RAGMetadataCondition{Name: scopeMetadataKey, Operator: "is", Value: id.String()}
	}
	return out
}

// reportTitle ghép tên project với nhãn scope (vd "Demo Project - v1.0.0") để
// hiển thị trong file report — giữ hành vi rõ ràng như UAT export cũ của
// module document. Không có scope (resolved rỗng) → chỉ tên project.
func reportTitle(projectName string, resolved []retrievaldomain.ResolvedScope) string {
	if len(resolved) == 0 {
		return projectName
	}
	return fmt.Sprintf("%s - %s", projectName, resolved[0].Label)
}

// persistReport lưu file đã render lên ObjectStore, ghi lịch sử (transaction)
// rồi trả presigned URL để tải file.
func (s *Service) persistReport(
	ctx context.Context, projectID, actorID, reportID uuid.UUID, reportType, format, contentType string,
	content []byte, items []domain.ReportItem,
) (*GenerateResult, error) {
	now := s.clock.Now().UTC()
	key := fmt.Sprintf("reports/%s/%s.%s", projectID, reportID, format)
	if _, err := s.store.Put(ctx, key, content, contentType); err != nil {
		return nil, apperr.External("Không thể lưu file report").WithCause(err)
	}

	report := domain.Report{
		ID: reportID, ProjectID: projectID, ReportType: reportType,
		Format: format, FileKey: key, GeneratedBy: actorID, CreatedAt: now,
	}
	err := s.tx.Do(ctx, func(txctx context.Context) error {
		return s.repo.Create(txctx, report, items)
	})
	if err != nil {
		return nil, apperr.Database("Không thể lưu report").WithCause(err)
	}

	downloadURL, err := s.store.PresignedGetURL(ctx, key, downloadURLTTL)
	if err != nil {
		return nil, apperr.External("Không thể tạo URL tải file report").WithCause(err)
	}
	return &GenerateResult{Report: report, DownloadURL: downloadURL}, nil
}

type HistoryItem struct {
	Report      domain.Report `json:"report"`
	DownloadURL string        `json:"download_url"`
}

// ListHistory trả lịch sử các lần xuất báo cáo của project (đọc — viewer+ được xem).
func (s *Service) ListHistory(
	ctx context.Context, projectID uuid.UUID, page pagination.Query,
) ([]HistoryItem, pagination.Meta, error) {
	if _, err := s.authorize(ctx, projectID, false); err != nil {
		return nil, pagination.Meta{}, err
	}
	page = page.Normalize()
	reports, total, err := s.repo.ListHistory(ctx, projectID, page.Page, page.Limit)
	if err != nil {
		return nil, pagination.Meta{}, apperr.Database("Không thể đọc lịch sử report").WithCause(err)
	}
	items := make([]HistoryItem, len(reports))
	for i, report := range reports {
		url, urlErr := s.store.PresignedGetURL(ctx, report.FileKey, downloadURLTTL)
		if urlErr != nil {
			return nil, pagination.Meta{}, apperr.External("Không thể tạo URL tải file report").WithCause(urlErr)
		}
		items[i] = HistoryItem{Report: report, DownloadURL: url}
	}
	return items, pagination.NewMeta(page.Page, page.Limit, total), nil
}

// ensureChat lấy hoặc tạo RAGFlow chat assistant cho project — CHUNG chat với
// tính năng hỏi-đáp (module chat, cùng cột projects.ragflow_chat_id). An toàn vì
// CompleteChat không giữ state phía server: mỗi lần gọi ta tự truyền toàn bộ
// messages, không có lịch sử hội thoại nào bị rò rỉ giữa 2 tính năng.
func (s *Service) ensureChat(ctx context.Context, projectID uuid.UUID, datasetID string) (string, error) {
	chatID, err := s.repo.RAGFlowChatID(ctx, projectID)
	if err != nil {
		return "", err
	}
	if chatID != "" {
		return chatID, nil
	}
	name := "docs_hub_" + strings.ReplaceAll(projectID.String(), "-", "")
	chat, err := s.rag.FindChatByName(ctx, name)
	if err != nil {
		return "", err
	}
	if chat == nil {
		created, createErr := s.rag.CreateChat(ctx, name, []string{datasetID})
		if createErr != nil {
			return "", createErr
		}
		chat = &created
	} else if !contains(chat.DatasetIDs, datasetID) {
		if err = s.rag.UpdateChatDatasets(ctx, chat.ID, []string{datasetID}); err != nil {
			return "", err
		}
	}
	return s.repo.SaveRAGFlowChatID(ctx, projectID, chat.ID)
}

func (s *Service) authorize(ctx context.Context, projectID uuid.UUID, write bool) (uuid.UUID, error) {
	actor, ok := contextx.ActorFrom(ctx)
	if !ok {
		return uuid.Nil, apperr.Unauthorized("Chưa xác thực")
	}
	actorID, err := uuid.Parse(actor.UserID)
	if err != nil {
		return uuid.Nil, apperr.Unauthorized("Actor không hợp lệ")
	}
	if s.bypassProjectACL {
		return actorID, nil
	}
	role, err := s.repo.MemberRole(ctx, projectID, actorID)
	if err != nil {
		return uuid.Nil, apperr.Database("Không thể kiểm tra quyền project").WithCause(err)
	}
	if role == "" || (write && role == "viewer") {
		return uuid.Nil, apperr.Forbidden("Không có quyền thao tác project")
	}
	return actorID, nil
}

func (s *Service) mapErr(err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return apperr.NotFound(errcode.NotFound, "Không tìm thấy project")
	}
	return apperr.Database("Lỗi đọc dữ liệu project").WithCause(err)
}

func normalizeFormat(format string) (string, error) {
	switch format {
	case "", domain.FormatXLSX:
		return domain.FormatXLSX, nil
	case domain.FormatPDF:
		return domain.FormatPDF, nil
	default:
		return "", apperr.BadRequest("Định dạng chỉ hỗ trợ xlsx hoặc pdf")
	}
}

func toUATReportItems(reportID uuid.UUID, items []uatRAGItem) []domain.ReportItem {
	out := make([]domain.ReportItem, len(items))
	for i, item := range items {
		out[i] = domain.ReportItem{
			ID: uuid.New(), ReportID: reportID, SourceRef: item.Source,
			Title: item.Title, Detail: formatUATDetail(item), SequenceNo: i + 1,
		}
	}
	return out
}

func formatUATDetail(item uatRAGItem) string {
	return fmt.Sprintf("Bước thực hiện: %s\nKết quả mong đợi: %s", item.Steps, item.Expected)
}

// toPlanningReportItems làm phẳng milestone/task thành danh sách ReportItem —
// SourceRef giữ tên milestone để biết task thuộc giai đoạn nào.
func toPlanningReportItems(reportID uuid.UUID, milestones []planningRAGMilestone) []domain.ReportItem {
	var out []domain.ReportItem
	seq := 1
	for _, milestone := range milestones {
		for _, task := range milestone.Tasks {
			out = append(out, domain.ReportItem{
				ID: uuid.New(), ReportID: reportID, SourceRef: milestone.Name,
				Title: task.Name, Detail: task.Description, SequenceNo: seq,
			})
			seq++
		}
	}
	return out
}

func toTestcaseReportItems(reportID uuid.UUID, items []testcaseRAGItem) []domain.ReportItem {
	out := make([]domain.ReportItem, len(items))
	for i, item := range items {
		out[i] = domain.ReportItem{
			ID: uuid.New(), ReportID: reportID, SourceRef: item.DocSource,
			Title: item.Title, Detail: formatTestcaseDetail(item), SequenceNo: i + 1,
		}
	}
	return out
}

func formatTestcaseDetail(item testcaseRAGItem) string {
	return fmt.Sprintf("Tiền đề: %s\nBước thực hiện: %s\nKết quả mong đợi: %s",
		item.Precondition, item.Steps, item.Expected)
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
