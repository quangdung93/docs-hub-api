package usecase

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/quangdung93/docs-hub-api/internal/module/report/assets"
	"github.com/quangdung93/docs-hub-api/internal/module/report/domain"
)

const (
	testcaseSheetSummary  = "Summary"
	testcaseSheetCases    = "Test Cases"
	testcaseSheetSample2  = "Test Case 2" // sheet mẫu có sẵn dữ liệu ví dụ, cần dọn để Dashboard không đếm nhầm
	testcaseSheetCover    = "Cover"
	testcaseSheetBugData  = "Bug Data"
	testcaseSheetRTM      = "RTM"
	testcaseSheetRevision = "Revision History"
	testcaseSheetReport   = "Report Test"
	testcaseFirstDataRow  = 7
	testcaseSampleLastRow = 9 // dòng ví dụ có sẵn trong template gốc ở "Test Case 2"
	testcaseBugFirstRow   = 2 // 13 bug mẫu IP-101..IP-113 nằm ở dòng 2-14 của "Bug Data"
	testcaseBugLastRow    = 14
	testcaseRTMFirstRow   = 6 // RTM chỉ có 5 chỗ cho yêu cầu: dòng 6-10, dòng 11 là "Tổng"
	testcaseRTMLastRow    = 10
	testcaseRTMTotalCell  = "I11"
)

type testcaseContentInput struct {
	ProjectName string
	Items       []testcaseRAGItem
}

// renderTestcaseContent sinh nội dung file theo định dạng đã chuẩn hoá — items
// là test case do RAGFlow tổng hợp từ User Story/AC trong tài liệu dự án.
func renderTestcaseContent(format string, in testcaseContentInput) ([]byte, string, error) {
	if format == domain.FormatPDF {
		content, err := buildTestcasePDF(in)
		return content, contentTypePDF, err
	}
	content, err := buildTestcaseWorkbook(in)
	return content, contentTypeXLSX, err
}

// buildTestcaseWorkbook mở template Testcase Report chuẩn ISC (SDLC) đã nhúng
// sẵn, điền tên dự án (Summary) và danh sách test case (sheet "Test Cases").
// Cột A (Testcase ID) là công thức có sẵn trong template (tự sinh từ cột C
// DOC Source) — không đụng vào. Sheet "Test Case 2" là dữ liệu MẪU có sẵn
// trong template gốc, dọn sạch để Dashboard (đếm qua INDIRECT theo tên sheet)
// không cộng nhầm test case ảo vào báo cáo thật.
func buildTestcaseWorkbook(in testcaseContentInput) ([]byte, error) {
	f, err := excelize.OpenReader(bytes.NewReader(assets.TestcaseReportXLSX))
	if err != nil {
		return nil, fmt.Errorf("mở template Testcase Report: %w", err)
	}
	defer f.Close()

	if in.ProjectName != "" {
		if err := f.SetCellValue(testcaseSheetSummary, "C6", in.ProjectName); err != nil {
			return nil, fmt.Errorf("điền sheet Summary ô C6: %w", err)
		}
		if err := f.SetCellValue(testcaseSheetCases, "C3", in.ProjectName); err != nil {
			return nil, fmt.Errorf("điền sheet %s ô C3: %w", testcaseSheetCases, err)
		}
		// Cover!B6 in sẵn "[ĐIỀN TÊN DỰ ÁN]" — chỗ trống chờ điền chứ không phải
		// dữ liệu mẫu, nên phải GHI ĐÈ tên dự án chứ không xoá trắng.
		if err := f.SetCellValue(testcaseSheetCover, "B6", in.ProjectName); err != nil {
			return nil, fmt.Errorf("điền sheet %s ô B6: %w", testcaseSheetCover, err)
		}
	}
	// Cả "Test Cases" lẫn "Test Case 2" đều có sẵn dữ liệu MẪU ở dòng 7-9 trong
	// template gốc — phải dọn cả hai trước khi điền, nếu không phần dư (cột J+
	// hoặc các dòng vượt quá số item thật) sẽ lẫn dữ liệu mẫu vào báo cáo thật.
	if err := clearTestcaseSampleRows(f, testcaseSheetCases); err != nil {
		return nil, err
	}
	if err := clearTestcaseSampleRows(f, testcaseSheetSample2); err != nil {
		return nil, err
	}
	if err := clearTestcaseSampleData(f); err != nil {
		return nil, err
	}
	if err := fillTestcaseRows(f, in.Items); err != nil {
		return nil, err
	}
	if err := fillTestcaseRTM(f, in.Items); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("ghi file Testcase Report: %w", err)
	}
	return buf.Bytes(), nil
}

// fillTestcaseRows điền mỗi test case thành 1 dòng bắt đầu từ dòng 7 (B..I) —
// nội dung THẬT do RAGFlow sinh. Cột J trở đi (Origin/Review/Automated/Script/
// Vibe-test/KQ Script/Kết quả/Executed By/ID Bugs) bỏ trống — QA điền tay sau
// khi test thật, giống UAT report.
func fillTestcaseRows(f *excelize.File, items []testcaseRAGItem) error {
	for i, item := range items {
		row := testcaseFirstDataRow + i
		cells := map[string]string{
			fmt.Sprintf("B%d", row): item.ReqID,
			fmt.Sprintf("C%d", row): item.DocSource,
			fmt.Sprintf("D%d", row): item.Group,
			fmt.Sprintf("E%d", row): item.Priority,
			fmt.Sprintf("F%d", row): item.Title,
			fmt.Sprintf("G%d", row): item.Precondition,
			fmt.Sprintf("H%d", row): item.Steps,
			fmt.Sprintf("I%d", row): item.Expected,
		}
		for _, col := range []string{"B", "C", "D", "E", "F", "G", "H", "I"} {
			cell := fmt.Sprintf("%s%d", col, row)
			if err := f.SetCellValue(testcaseSheetCases, cell, cells[cell]); err != nil {
				return fmt.Errorf("điền sheet %s dòng %d: %w", testcaseSheetCases, row, err)
			}
		}
	}
	return nil
}

// clearTestcaseSampleRows xóa dữ liệu ví dụ có sẵn (B..T, dòng 7-9) trong
// template gốc của sheet đã cho — không thể xóa hẳn sheet "Test Case 2" (vẫn
// được Dashboard tham chiếu qua INDIRECT theo tên sheet, xóa sheet sẽ làm
// hỏng công thức #REF!). Cột A (Testcase ID) là công thức, không đụng vào.
func clearTestcaseSampleRows(f *excelize.File, sheet string) error {
	return clearSampleBlock(f, sheet, "B", "T", testcaseFirstDataRow, testcaseSampleLastRow)
}

// rtmRequirement là một dòng của ma trận truy vết: mã yêu cầu và tài liệu nguồn.
type rtmRequirement struct {
	reqID     string
	docSource string
}

// fillTestcaseRTM dựng lại sheet "RTM" (ma trận truy vết yêu cầu) từ chính danh
// sách test case vừa sinh, thay cho 5 yêu cầu VÍ DỤ mà template ship sẵn
// (REQ-01..REQ-05, nguồn "SRS mục 3.1"…) vốn không liên quan gì tới dự án thật.
//
// KHÔNG thể chỉ xoá phần chữ như các sheet khác. Cột E..H đếm bằng
// COUNTIF("*"&$B6&"*"); Req ID rỗng làm điều kiện thành "**" — khớp MỌI ô có chữ
// — nên một dòng yêu cầu trống sẽ báo là phủ toàn bộ test case. Vì vậy mỗi dòng
// chỉ có hai trạng thái: điền đủ (giữ nguyên công thức đếm) hoặc rỗng hoàn toàn
// (xoá cả công thức).
//
// Ô I11 ở dòng "Tổng" nằm chung nhóm shared formula với I6:I10 nên bị xoá lây khi
// dọn — phải đọc trước rồi ghi lại. E11:H11 là SUM(E6:E10) nên tự về 0, đúng ý.
//
// Giới hạn đã biết: template chỉ có 5 chỗ (dòng 6-10). Nhiều hơn 5 yêu cầu thì
// phần dư không có chỗ hiển thị, giống cách planningMilestoneSlots giới hạn task.
func fillTestcaseRTM(f *excelize.File, items []testcaseRAGItem) error {
	tongPhuTram, err := f.GetCellFormula(testcaseSheetRTM, testcaseRTMTotalCell)
	if err != nil {
		return fmt.Errorf("đọc công thức RTM %s: %w", testcaseRTMTotalCell, err)
	}

	requirements := distinctRequirements(items)
	for row := testcaseRTMFirstRow; row <= testcaseRTMLastRow; row++ {
		index := row - testcaseRTMFirstRow
		if index >= len(requirements) {
			if err := clearTestcaseRTMRow(f, row); err != nil {
				return err
			}
			continue
		}
		if err := f.SetCellValue(testcaseSheetRTM,
			fmt.Sprintf("B%d", row), requirements[index].reqID); err != nil {
			return fmt.Errorf("điền RTM ô B%d: %w", row, err)
		}
		if err := f.SetCellValue(testcaseSheetRTM,
			fmt.Sprintf("D%d", row), requirements[index].docSource); err != nil {
			return fmt.Errorf("điền RTM ô D%d: %w", row, err)
		}
		// Cột C (mô tả yêu cầu) để trống: app chỉ có mã và nguồn, tự sinh mô tả
		// là bịa. QA điền tay khi rà soát.
		if err := clearSampleCells(f, testcaseSheetRTM, fmt.Sprintf("C%d", row)); err != nil {
			return err
		}
	}

	if err := f.SetCellFormula(testcaseSheetRTM, testcaseRTMTotalCell, tongPhuTram); err != nil {
		return fmt.Errorf("khôi phục công thức RTM %s: %w", testcaseRTMTotalCell, err)
	}
	return nil
}

// distinctRequirements gom mã yêu cầu KHÔNG trùng theo đúng thứ tự xuất hiện,
// tối đa bằng số dòng RTM có. Test case không có mã yêu cầu bị bỏ qua — prompt
// đã yêu cầu để trống thay vì tự đặt mã (xem testcasePromptTemplate).
func distinctRequirements(items []testcaseRAGItem) []rtmRequirement {
	limit := testcaseRTMLastRow - testcaseRTMFirstRow + 1
	seen := make(map[string]struct{}, len(items))
	requirements := make([]rtmRequirement, 0, limit)
	for _, item := range items {
		reqID := strings.TrimSpace(item.ReqID)
		if reqID == "" {
			continue
		}
		if _, done := seen[reqID]; done {
			continue
		}
		seen[reqID] = struct{}{}
		requirements = append(requirements, rtmRequirement{
			reqID: reqID, docSource: strings.TrimSpace(item.DocSource),
		})
		if len(requirements) == limit {
			break
		}
	}
	return requirements
}

// clearTestcaseRTMRow xoá sạch một dòng yêu cầu, KỂ CẢ công thức đếm — xem lý do
// ở fillTestcaseRTM. Cột A không đụng tới vì không mang nội dung.
//
// Dùng SetCellValue chứ KHÔNG dùng SetCellFormula: SetCellValue gỡ cả công thức
// lẫn giá trị, còn SetCellFormula("") chỉ gỡ công thức và để lại giá trị đã tính
// sẵn trong file — ô sẽ hiện con số cũ của dữ liệu mẫu.
func clearTestcaseRTMRow(f *excelize.File, row int) error {
	for _, col := range []string{"B", "C", "D", "E", "F", "G", "H", "I", "J"} {
		cell := fmt.Sprintf("%s%d", col, row)
		if err := f.SetCellValue(testcaseSheetRTM, cell, ""); err != nil {
			return fmt.Errorf("dọn RTM ô %s: %w", cell, err)
		}
	}
	return nil
}

// clearTestcaseSampleData dọn dữ liệu của dự án mẫu ở năm sheet mà
// clearTestcaseSampleRows không đụng tới — phần lớn khối lượng nằm ở đây.
func clearTestcaseSampleData(f *excelize.File) error {
	// Bug Data: 13 bug giả IP-101..IP-113 trải 22 cột. Đây là nguồn của toàn bộ
	// thống kê ở Dashboard và Report Test (90 công thức COUNTIF/COUNTIFS), để
	// nguyên thì biểu đồ trong báo cáo tính trên bug không có thật.
	if err := clearSampleBlock(f, testcaseSheetBugData, "A", "V",
		testcaseBugFirstRow, testcaseBugLastRow); err != nil {
		return err
	}
	// RTM do fillTestcaseRTM lo — không dọn ở đây, vì dòng yêu cầu trống mà giữ
	// lại công thức đếm sẽ cho số liệu sai (xem chú thích ở fillTestcaseRTM).
	// Summary: version/sprint/ngày test/PIC của dự án mẫu. C12 là công thức đếm
	// số sheet chức năng nên không nằm trong danh sách.
	if err := clearSampleCells(f, testcaseSheetSummary,
		"C8", "C9", "C10", "C11", "C13"); err != nil {
		return err
	}
	// Summary: khối môi trường & phạm vi test, gồm cả các ô gợi ý "VD: ...".
	if err := clearSampleBlock(f, testcaseSheetSummary, "C", "C", 57, 62); err != nil {
		return err
	}
	// Revision History: dòng lịch sử phiên bản của dự án mẫu.
	if err := clearSampleBlock(f, testcaseSheetRevision, "B", "G", 7, 7); err != nil {
		return err
	}
	// Report Test: tên chức năng mẫu (Function A/B/C) ở bảng thống kê bug theo
	// chức năng, và phạm vi test từng round. Cột B (mã F01..F10) là khung bảng.
	if err := clearSampleCells(f, testcaseSheetReport, "C42", "C43", "C44"); err != nil {
		return err
	}
	return clearSampleBlock(f, testcaseSheetReport, "L", "L", 218, 223)
}
