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
)

// maxUATItems giới hạn số dòng report UAT — template chừa sẵn dòng 12..200 cho
// Round 1 (công thức COUNTIF trong sheet Report 1_Module dựa vào range đó).
// Khác UAT export cũ (module document): ở đây KHÔNG lỗi khi vượt, chỉ cắt bớt,
// vì nội dung do LLM sinh (không phải danh sách tài liệu cố định của người dùng).
const maxUATItems = 188

const downloadURLTTL = 15 * time.Minute

type Service struct {
	repo             domain.Repository
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
	repo domain.Repository, tx port.TxManager, rag port.RAGClient, store port.ObjectStore,
	clock port.Clock, options ...Option,
) *Service {
	service := &Service{repo: repo, tx: tx, rag: rag, store: store, clock: clock}
	for _, option := range options {
		option(service)
	}
	return service
}

// GenerateInput là tham số xuất báo cáo. Theo SRS v1.1: không còn chọn version/
// change request — luôn lấy nội dung tài liệu mới nhất tại thời điểm xuất.
type GenerateInput struct {
	ProjectID  uuid.UUID
	ReportType string
	Format     string
}

type GenerateResult struct {
	Report      domain.Report `json:"report"`
	DownloadURL string        `json:"download_url"`
}

// Generate sinh báo cáo dự án (UAT Report — tiêu chí nghiệm thu, Project
// Planning, hoặc Testcase Report — SRS v1.1 mục IX/X) bằng cách nhờ RAGFlow
// tổng hợp nội dung tài liệu dự án, điền vào template chuẩn ISC tương ứng,
// lưu file + lịch sử.
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
	reportID := uuid.New()
	content, contentType, items, err := s.generateContent(ctx, in.ProjectID, in.ReportType, format, reportID)
	if err != nil {
		return nil, err
	}
	return s.persistReport(ctx, in.ProjectID, actorID, reportID, in.ReportType, format, contentType, content, items)
}

// generateContent điều phối theo report_type: nhờ RAGFlow tổng hợp nội dung
// rồi render ra file — mỗi loại có prompt/schema/template riêng.
func (s *Service) generateContent(
	ctx context.Context, projectID uuid.UUID, reportType, format string, reportID uuid.UUID,
) ([]byte, string, []domain.ReportItem, error) {
	switch reportType {
	case domain.ReportTypeUAT:
		return s.generateUATContent(ctx, projectID, format, reportID)
	case domain.ReportTypePlanning:
		return s.generatePlanningContent(ctx, projectID, format, reportID)
	case domain.ReportTypeTestcase:
		return s.generateTestcaseContent(ctx, projectID, format, reportID)
	default:
		return nil, "", nil, apperr.BadRequest("Loại báo cáo không hợp lệ (hỗ trợ: uat, planning, testcase)")
	}
}

func (s *Service) generateUATContent(
	ctx context.Context, projectID uuid.UUID, format string, reportID uuid.UUID,
) ([]byte, string, []domain.ReportItem, error) {
	projectName, items, err := s.fetchUATItems(ctx, projectID)
	if err != nil {
		return nil, "", nil, err
	}
	content, contentType, err := renderUATContent(format, uatContentInput{ProjectName: projectName, Items: items})
	if err != nil {
		return nil, "", nil, apperr.Internal("Không thể tạo file report").WithCause(err)
	}
	return content, contentType, toUATReportItems(reportID, items), nil
}

func (s *Service) generatePlanningContent(
	ctx context.Context, projectID uuid.UUID, format string, reportID uuid.UUID,
) ([]byte, string, []domain.ReportItem, error) {
	projectName, milestones, err := s.fetchPlanningMilestones(ctx, projectID)
	if err != nil {
		return nil, "", nil, err
	}
	content, contentType, err := renderPlanningContent(format,
		planningContentInput{ProjectName: projectName, Milestones: milestones})
	if err != nil {
		return nil, "", nil, apperr.Internal("Không thể tạo file report").WithCause(err)
	}
	return content, contentType, toPlanningReportItems(reportID, milestones), nil
}

func (s *Service) generateTestcaseContent(
	ctx context.Context, projectID uuid.UUID, format string, reportID uuid.UUID,
) ([]byte, string, []domain.ReportItem, error) {
	projectName, items, err := s.fetchTestcaseItems(ctx, projectID)
	if err != nil {
		return nil, "", nil, err
	}
	content, contentType, err := renderTestcaseContent(format,
		testcaseContentInput{ProjectName: projectName, Items: items})
	if err != nil {
		return nil, "", nil, apperr.Internal("Không thể tạo file report").WithCause(err)
	}
	return content, contentType, toTestcaseReportItems(reportID, items), nil
}

// fetchUATItems nhờ RAGFlow tổng hợp User Story/Acceptance Criteria trong tài
// liệu dự án thành danh sách test case UAT.
func (s *Service) fetchUATItems(ctx context.Context, projectID uuid.UUID) (string, []uatRAGItem, error) {
	projectName, raw, err := s.completeChat(ctx, projectID, func(name string) string {
		return uatPrompt(name, maxUATItems)
	})
	if err != nil {
		return "", nil, err
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
	return projectName, items, nil
}

// fetchPlanningMilestones nhờ RAGFlow tổng hợp tài liệu dự án thành kế hoạch
// triển khai theo milestone (Project Planning).
func (s *Service) fetchPlanningMilestones(ctx context.Context, projectID uuid.UUID) (string, []planningRAGMilestone, error) {
	projectName, raw, err := s.completeChat(ctx, projectID, planningPrompt)
	if err != nil {
		return "", nil, err
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
	return projectName, milestones, nil
}

// fetchTestcaseItems nhờ RAGFlow sinh danh sách test case chi tiết từ User
// Story/Acceptance Criteria trong tài liệu dự án.
func (s *Service) fetchTestcaseItems(ctx context.Context, projectID uuid.UUID) (string, []testcaseRAGItem, error) {
	projectName, raw, err := s.completeChat(ctx, projectID, func(name string) string {
		return testcasePrompt(name, maxTestcaseItems)
	})
	if err != nil {
		return "", nil, err
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
	return projectName, items, nil
}

// completeChat gói chung phần lấy project metadata + RAGFlow dataset/chat +
// gọi CompleteChat — 3 loại report chỉ khác nhau ở prompt (buildPrompt nhận
// projectName để build prompt SAU khi đã biết tên dự án).
func (s *Service) completeChat(
	ctx context.Context, projectID uuid.UUID, buildPrompt func(projectName string) string,
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
	chatID, err := s.ensureChat(ctx, projectID, datasetID)
	if err != nil {
		return "", "", apperr.External("Không thể chuẩn bị RAGFlow chat assistant").WithCause(err)
	}
	result, err := s.rag.CompleteChat(ctx, port.RAGChatCompletionRequest{
		ChatID:   chatID,
		Messages: []port.RAGChatMessage{{Role: "user", Content: buildPrompt(projectName)}},
	})
	if err != nil {
		return "", "", apperr.External("RAGFlow chat không khả dụng").WithCause(err)
	}
	return projectName, result.Content, nil
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
