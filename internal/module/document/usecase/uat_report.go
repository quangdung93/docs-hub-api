package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/module/document/domain"
)

// maxUATItems giới hạn số dòng report — template chừa sẵn dòng 12..200 cho
// Round 1 (công thức COUNTIF trong sheet Report 1_Module dựa vào range đó).
const maxUATItems = 188

// Định dạng file UAT Report hỗ trợ — theo SRS mục IX: "Excel hoặc PDF".
const (
	UATFormatXLSX = "xlsx"
	UATFormatPDF  = "pdf"
)

// UATReportInput là tham số xuất UAT Report theo template chuẩn ISC
// (4.0-BM/PM/HDCV/FTEL). Để trống cả VersionID và ChangeRequestID nghĩa là
// lấy toàn bộ tài liệu của project.
type UATReportInput struct {
	ProjectID   uuid.UUID
	Scope       domain.Scope
	Format      string // "xlsx" (mặc định) hoặc "pdf"
	PO          string
	PM          string
	AccountTest string
	ScopeTest   string
	StartDate   *time.Time
	DueDate     *time.Time
}

// UATReportResult là file đã điền dữ liệu (xlsx hoặc pdf), sẵn sàng trả về client.
type UATReportResult struct {
	FileName    string
	ContentType string
	Content     []byte
}

// ExportUATReport điền danh sách tài liệu trong phạm vi đã chọn (version,
// change request, hoặc toàn dự án) vào template UAT Report chuẩn ISC và trả
// lại nội dung file xlsx/pdf. Không lưu trạng thái — mỗi lần gọi sinh 1 file mới.
//
// Theo SRS mục IX (BR): "Chỉ Editor trở lên được phép xuất UAT Report" — dùng
// authorize(..., write=true) để chặn viewer, khác với các thao tác đọc khác
// của module document.
func (s *Service) ExportUATReport(ctx context.Context, in UATReportInput) (*UATReportResult, error) {
	if _, err := s.authorize(ctx, in.ProjectID, true); err != nil {
		return nil, err
	}
	format, err := normalizeUATFormat(in.Format)
	if err != nil {
		return nil, err
	}
	if err = s.validateUATScope(ctx, in.ProjectID, in.Scope); err != nil {
		return nil, err
	}
	projectName, projectCode, err := s.repo.ProjectMeta(ctx, in.ProjectID)
	if err != nil {
		return nil, s.mapErr(err)
	}
	scopeLabel, scopeKind, err := s.repo.ScopeMeta(ctx, in.ProjectID, in.Scope)
	if err != nil {
		return nil, s.mapErr(err)
	}
	items, err := s.repo.UATItems(ctx, in.ProjectID, in.Scope)
	if err != nil {
		return nil, apperr.Internal("Không thể đọc danh sách tài liệu").WithCause(err)
	}
	if len(items) == 0 {
		return nil, apperr.BadRequest("Không có tài liệu nào trong phạm vi đã chọn")
	}
	if len(items) > maxUATItems {
		return nil, apperr.BadRequest(
			fmt.Sprintf("Phạm vi có %d tài liệu, vượt quá %d dòng template hỗ trợ", len(items), maxUATItems))
	}
	content, contentType, err := renderUATContent(format, uatWorkbookInput{
		ProjectName: projectName, ProjectCode: projectCode,
		ScopeLabel: scopeLabel, ScopeKind: scopeKind,
		PO: in.PO, PM: in.PM, AccountTest: in.AccountTest, ScopeTest: in.ScopeTest,
		StartDate: in.StartDate, DueDate: in.DueDate, Items: items,
	})
	if err != nil {
		return nil, apperr.Internal("Không thể tạo file UAT Report").WithCause(err)
	}
	fileName := fmt.Sprintf("UAT_Report_%s_%s.%s",
		safeName(projectCode), safeName(scopeFileTag(scopeKind)), format)
	return &UATReportResult{FileName: fileName, ContentType: contentType, Content: content}, nil
}

// validateUATScope kiểm tra scope hợp lệ (đúng 1 trong 2 hoặc để trống) và
// thuộc đúng project — tách riêng để giữ ExportUATReport gọn.
func (s *Service) validateUATScope(ctx context.Context, projectID uuid.UUID, scope domain.Scope) error {
	if scope.VersionID != nil && scope.ChangeRequestID != nil {
		return apperr.BadRequest("Chỉ chọn version hoặc change request, không thể cả hai")
	}
	if scope.VersionID == nil && scope.ChangeRequestID == nil {
		return nil
	}
	ok, err := s.repo.ScopeExists(ctx, projectID, scope)
	if err != nil {
		return apperr.Internal("Không thể kiểm tra scope").WithCause(err)
	}
	if !ok {
		return apperr.BadRequest("Scope không thuộc project")
	}
	return nil
}

func normalizeUATFormat(format string) (string, error) {
	switch format {
	case "", UATFormatXLSX:
		return UATFormatXLSX, nil
	case UATFormatPDF:
		return UATFormatPDF, nil
	default:
		return "", apperr.BadRequest("Định dạng chỉ hỗ trợ xlsx hoặc pdf")
	}
}

// renderUATContent sinh nội dung file theo định dạng đã chuẩn hoá.
func renderUATContent(format string, in uatWorkbookInput) ([]byte, string, error) {
	if format == UATFormatPDF {
		content, err := buildUATPDF(in)
		return content, "application/pdf", err
	}
	content, err := buildUATWorkbook(in)
	return content, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", err
}

func scopeFileTag(kind string) string {
	switch kind {
	case domain.ScopeKindVersion:
		return "version"
	case domain.ScopeKindChangeRequest:
		return "cr"
	default:
		return "all"
	}
}
