// Package usecase điều phối nhận diện & phân tích edge case cho tài liệu URD
// (URD v1.2 mục XI), không phụ thuộc HTTP/GORM.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/common/errcode"
	"github.com/quangdung93/docs-hub-api/internal/common/port"
	documentdomain "github.com/quangdung93/docs-hub-api/internal/module/document/domain"
	documentusecase "github.com/quangdung93/docs-hub-api/internal/module/document/usecase"
	"github.com/quangdung93/docs-hub-api/internal/module/urd/docxmerge"
	"github.com/quangdung93/docs-hub-api/internal/module/urd/domain"
)

// maxCanonicalTextBytes giới hạn kích thước đọc từ canonical text — chặn tài
// liệu bất thường lớn làm tràn bộ nhớ trước khi cắt tiếp ở urdPrompt.
const maxCanonicalTextBytes = 2 << 20 // 2 MiB

// revisionStatusReady khớp giá trị document_revisions.status do module
// document/ingestion set khi đã ingest xong (xem document/usecase/service.go).
const revisionStatusReady = "ready"

type Service struct {
	repo             domain.Repository
	tx               port.TxManager
	rag              port.RAGClient
	store            port.ObjectStore
	clock            port.Clock
	docSvc           *documentusecase.Service
	bypassProjectACL bool
}

// Option cấu hình hành vi tùy môi trường cho urd service.
type Option func(*Service)

// WithProjectACLBypass chỉ dành cho local development; production không được bật.
func WithProjectACLBypass(enabled bool) Option {
	return func(service *Service) { service.bypassProjectACL = enabled }
}

func New(
	repo domain.Repository, tx port.TxManager, rag port.RAGClient, store port.ObjectStore,
	clock port.Clock, docSvc *documentusecase.Service, options ...Option,
) *Service {
	service := &Service{repo: repo, tx: tx, rag: rag, store: store, clock: clock, docSvc: docSvc}
	for _, option := range options {
		option(service)
	}
	return service
}

// Analyze nhờ AI liệt kê edge case chưa được đề cập trong revision mới nhất
// (đã ingest xong) của 1 tài liệu đã xác nhận là URD. BR: chỉ Editor trở lên.
func (s *Service) Analyze(ctx context.Context, projectID, documentID uuid.UUID) (*domain.Analysis, []domain.EdgeCase, error) {
	actorID, err := s.authorize(ctx, projectID, true)
	if err != nil {
		return nil, nil, err
	}
	d, revisions, err := s.docSvc.Detail(ctx, projectID, documentID)
	if err != nil {
		return nil, nil, err
	}
	if d.DocType != documentdomain.DocTypeURD {
		return nil, nil, apperr.NewBusiness(errcode.URDNotConfirmed, "Tài liệu chưa được xác nhận là URD", false)
	}
	revision := latestReadyRevision(revisions)
	if revision == nil {
		return nil, nil, apperr.NewBusiness(
			errcode.URDRevisionNotReady, "Phiên bản tài liệu chưa sẵn sàng để phân tích", true)
	}
	active, _, err := s.repo.GetActiveAnalysis(ctx, documentID)
	if err != nil {
		return nil, nil, apperr.Database("Không thể kiểm tra phân tích đang hoạt động").WithCause(err)
	}
	if active != nil {
		return nil, nil, apperr.NewBusiness(
			errcode.URDAnalysisActive, "Tài liệu đang có phân tích edge case chưa hoàn tất", false)
	}
	_, reader, err := s.docSvc.CanonicalSource(ctx, projectID, documentID, revision.ID)
	if err != nil {
		return nil, nil, err
	}
	text, err := io.ReadAll(io.LimitReader(reader, maxCanonicalTextBytes))
	reader.Close()
	if err != nil {
		return nil, nil, apperr.Internal("Không thể đọc nội dung tài liệu").WithCause(err)
	}
	raw, err := s.completeChat(ctx, projectID, d.Title, string(text))
	if err != nil {
		return nil, nil, err
	}
	if noDataAnswer(raw) {
		return nil, nil, noDataError(raw)
	}
	descriptions, err := parseEdgeCases(raw)
	if err != nil {
		return nil, nil, apperr.External("RAGFlow trả nội dung không đúng định dạng JSON mong đợi").WithCause(err)
	}
	return s.saveAnalysis(ctx, documentID, revision.ID, actorID, descriptions)
}

func (s *Service) saveAnalysis(
	ctx context.Context, documentID, revisionID, actorID uuid.UUID, descriptions []string,
) (*domain.Analysis, []domain.EdgeCase, error) {
	analysisID := uuid.New()
	cases := make([]domain.EdgeCase, len(descriptions))
	for i, desc := range descriptions {
		cases[i] = domain.EdgeCase{ID: uuid.New(), AnalysisID: analysisID, SequenceNo: i + 1, Description: desc}
	}
	status := domain.StatusAwaitingInput
	if len(cases) == 0 {
		// Không còn edge case nào đáng chú ý: coi như đã "hoàn thiện" ngay,
		// không cần tạo phiên bản mới vì không có nội dung gì để bổ sung.
		status = domain.StatusCompleted
	}
	a := domain.Analysis{
		ID: analysisID, DocumentID: documentID, RevisionID: revisionID,
		Status: status, TotalCases: len(cases), ResolvedCases: 0, CreatedBy: actorID,
	}
	err := s.tx.Do(ctx, func(txctx context.Context) error { return s.repo.CreateAnalysis(txctx, a, cases) })
	if err != nil {
		return nil, nil, apperr.Database("Không thể lưu phân tích edge case").WithCause(err)
	}
	return &a, cases, nil
}

// ResolutionInput là hướng giải quyết người dùng nhập cho 1 edge case.
// ImageObjectKey (tùy chọn) lấy từ UploadCaseImage gọi trước đó — tách riêng
// bước upload ảnh khỏi bước lưu hướng giải quyết để tránh phải trộn JSON và
// multipart trong cùng 1 request (giống cách document module tách Presign/
// Complete khỏi Update).
type ResolutionInput struct {
	CaseID         uuid.UUID
	Resolution     string
	ImageObjectKey string
}

// UploadCaseImage lưu ảnh minh hoạ cho 1 edge case, trả object key để client
// đính kèm vào ResolutionInput.ImageObjectKey khi gọi SubmitResolutions.
func (s *Service) UploadCaseImage(
	ctx context.Context, projectID, documentID, analysisID, caseID uuid.UUID, fileName, mediaType string, data []byte,
) (string, error) {
	if _, err := s.authorize(ctx, projectID, true); err != nil {
		return "", err
	}
	analysis, cases, err := s.repo.GetAnalysis(ctx, analysisID)
	if err != nil {
		return "", s.mapErr(err)
	}
	if analysis.DocumentID != documentID {
		return "", apperr.NotFound(errcode.NotFound, "Không tìm thấy phân tích edge case")
	}
	if !containsCase(cases, caseID) {
		return "", apperr.BadRequest("Edge case không thuộc phân tích này")
	}
	if len(data) == 0 {
		return "", apperr.BadRequest("Ảnh rỗng")
	}
	key := fmt.Sprintf("urd/%s/%s/%s", analysisID, caseID, safeAssetName(fileName))
	if _, err := s.store.Put(ctx, key, data, mediaType); err != nil {
		return "", apperr.Internal("Không thể lưu ảnh minh hoạ").WithCause(err)
	}
	return key, nil
}

func containsCase(cases []domain.EdgeCase, caseID uuid.UUID) bool {
	for _, c := range cases {
		if c.ID == caseID {
			return true
		}
	}
	return false
}

// SubmitResolutions lưu hướng giải quyết cho các case chỉ định. Khi đã đủ
// hướng giải quyết cho TOÀN BỘ case của phân tích, tự động tạo phiên bản URD
// mới (merge nội dung vào .docx gốc) và đánh dấu phân tích hoàn tất.
func (s *Service) SubmitResolutions(
	ctx context.Context, projectID, documentID, analysisID uuid.UUID, items []ResolutionInput,
) (*domain.Analysis, error) {
	if _, err := s.authorize(ctx, projectID, true); err != nil {
		return nil, err
	}
	analysis, cases, err := s.repo.GetAnalysis(ctx, analysisID)
	if err != nil {
		return nil, s.mapErr(err)
	}
	if analysis.DocumentID != documentID {
		return nil, apperr.NotFound(errcode.NotFound, "Không tìm thấy phân tích edge case")
	}
	updated, err := s.applyResolutions(cases, items)
	if err != nil {
		return nil, err
	}
	var result *domain.Analysis
	err = s.tx.Do(ctx, func(txctx context.Context) error {
		var e error
		result, e = s.repo.SaveResolutions(txctx, analysisID, updated)
		return e
	})
	if err != nil {
		return nil, apperr.Database("Không thể lưu hướng giải quyết").WithCause(err)
	}
	if result.ResolvedCases < result.TotalCases {
		return result, nil
	}
	return s.finalizeAnalysis(ctx, projectID, documentID, result)
}

func (s *Service) applyResolutions(
	cases []domain.EdgeCase, items []ResolutionInput,
) ([]domain.EdgeCase, error) {
	byID := make(map[uuid.UUID]domain.EdgeCase, len(cases))
	for _, c := range cases {
		byID[c.ID] = c
	}
	updated := make([]domain.EdgeCase, 0, len(items))
	for _, item := range items {
		existing, ok := byID[item.CaseID]
		if !ok {
			return nil, apperr.BadRequest("Edge case không thuộc phân tích này")
		}
		if strings.TrimSpace(item.Resolution) == "" {
			return nil, apperr.BadRequest("Phải nhập hướng giải quyết cho từng edge case")
		}
		existing.Resolution = item.Resolution
		existing.Resolved = true
		if item.ImageObjectKey != "" {
			existing.ImageObjectKey = item.ImageObjectKey
		}
		updated = append(updated, existing)
	}
	return updated, nil
}

// finalizeAnalysis tạo phiên bản URD mới bằng cách merge nội dung edge case
// (mô tả + hướng giải quyết) vào cuối file .docx gốc, rồi đánh dấu phân tích
// hoàn tất. Chạy SAU khi resolved_cases == total_cases.
func (s *Service) finalizeAnalysis(
	ctx context.Context, projectID, documentID uuid.UUID, a *domain.Analysis,
) (*domain.Analysis, error) {
	_, cases, err := s.repo.GetAnalysis(ctx, a.ID)
	if err != nil {
		return nil, s.mapErr(err)
	}
	revision, reader, err := s.docSvc.Download(ctx, projectID, documentID, a.RevisionID)
	if err != nil {
		return nil, err
	}
	original, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		return nil, apperr.Internal("Không thể đọc tài liệu URD gốc").WithCause(err)
	}
	merged, err := docxmerge.Merge(original, cases)
	if err != nil {
		return nil, apperr.Internal("Không thể tạo phiên bản URD mới: định dạng .docx gốc không được hỗ trợ để tự động chèn nội dung").
			WithCause(err)
	}
	if _, _, err = s.docSvc.CreateRevisionFromBytes(
		ctx, projectID, documentID, revision.Scope, revision.FileName, revision.MediaType, merged,
	); err != nil {
		return nil, err
	}
	completed, err := s.repo.MarkCompleted(ctx, a.ID)
	if err != nil {
		return nil, apperr.Database("Không thể cập nhật trạng thái hoàn tất").WithCause(err)
	}
	return completed, nil
}

// Get trả 1 phân tích cụ thể (để FE mở lại modal đang dở hoặc xem lịch sử).
func (s *Service) Get(ctx context.Context, projectID, documentID, analysisID uuid.UUID) (*domain.Analysis, []domain.EdgeCase, error) {
	if _, err := s.authorize(ctx, projectID, false); err != nil {
		return nil, nil, err
	}
	a, cases, err := s.repo.GetAnalysis(ctx, analysisID)
	if err != nil {
		return nil, nil, s.mapErr(err)
	}
	if a.DocumentID != documentID {
		return nil, nil, apperr.NotFound(errcode.NotFound, "Không tìm thấy phân tích edge case")
	}
	return a, cases, nil
}

// ListSummaries trả phân tích mới nhất của mỗi tài liệu trong project — dùng
// hiển thị cột "Hoàn thiện" trong bảng Quản lý dự án.
func (s *Service) ListSummaries(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID]domain.Analysis, error) {
	if _, err := s.authorize(ctx, projectID, false); err != nil {
		return nil, err
	}
	summaries, err := s.repo.Summaries(ctx, projectID)
	if err != nil {
		return nil, apperr.Database("Không thể đọc tóm tắt phân tích").WithCause(err)
	}
	return summaries, nil
}

// completeChat gọi RAGFlow chat assistant CHUNG với module chat/report (cùng
// cột projects.ragflow_chat_id, xem ensureChat) nhưng KHÔNG dựa vào truy hồi
// RAG theo câu hỏi — toàn văn tài liệu URD được nhúng thẳng vào prompt vì cần
// AI đọc hết để tìm chỗ còn thiếu edge case, không phải trả lời 1 câu hỏi cụ
// thể trên tập tài liệu dự án.
func (s *Service) completeChat(ctx context.Context, projectID uuid.UUID, documentTitle, canonicalText string) (string, error) {
	datasetID, err := s.repo.RAGFlowDatasetID(ctx, projectID)
	if err != nil {
		return "", apperr.Database("Không thể đọc RAGFlow dataset mapping").WithCause(err)
	}
	if datasetID == "" {
		return "", apperr.External("Project chưa được đồng bộ sang RAGFlow")
	}
	chatID, err := s.ensureChat(ctx, projectID, datasetID)
	if err != nil {
		return "", apperr.External("Không thể chuẩn bị RAGFlow chat assistant").WithCause(err)
	}
	result, err := s.rag.CompleteChat(ctx, port.RAGChatCompletionRequest{
		ChatID:   chatID,
		Messages: []port.RAGChatMessage{{Role: "user", Content: urdPrompt(documentTitle, canonicalText)}},
		// Không cần trích dẫn: chỉ parse Content thành JSON (xem report/usecase
		// completeChat — cùng lý do).
		WantReference: false,
	})
	if err != nil {
		return "", apperr.External("RAGFlow chat không khả dụng").WithCause(err)
	}
	return result.Content, nil
}

// ensureChat copy-adapt từ report/usecase/service.go (cùng cột projects.ragflow_chat_id).
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
	} else if !containsString(chat.DatasetIDs, datasetID) {
		if err = s.rag.UpdateChatDatasets(ctx, chat.ID, []string{datasetID}); err != nil {
			return "", err
		}
	}
	return s.repo.SaveRAGFlowChatID(ctx, projectID, chat.ID)
}

func containsString(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

func latestReadyRevision(revisions []documentdomain.Revision) *documentdomain.Revision {
	for i := range revisions {
		if revisions[i].Status == revisionStatusReady {
			return &revisions[i]
		}
	}
	return nil
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
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrNotFound) {
		return apperr.NotFound(errcode.NotFound, "Không tìm thấy phân tích edge case")
	}
	return apperr.Database("Lỗi đọc dữ liệu phân tích").WithCause(err)
}

//nolint:gochecknoglobals // regex bất biến, cùng cách document/usecase/service.go làm
var safeAssetNamePattern = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func safeAssetName(name string) string {
	name = safeAssetNamePattern.ReplaceAllString(name, "_")
	if name == "" {
		return "image"
	}
	if len(name) > 100 {
		name = name[:100]
	}
	return name
}
