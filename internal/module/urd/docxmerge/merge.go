// Package docxmerge chèn nội dung edge case vào 1 file .docx đã có, tạo ra 1
// file .docx mới — dùng cho bước "tạo phiên bản URD mới" của tính năng Nhận
// diện & Phân tích Edge Case cho URD (URD v1.2 mục XI).
//
// Nội dung được chèn THẲNG vào mục mà AI chỉ định (TargetHeading, ví dụ mục
// tiêu chí chấp nhận/quy tắc nghiệp vụ của chức năng liên quan) thay vì gom
// hết vào một phụ lục cuối tài liệu. Điểm chèn là cuối phần nội dung nằm
// TRỰC TIẾP dưới tiêu đề đó — ngay trước tiêu đề kế tiếp bất kỳ cấp nào — nên
// mục có mục con thì nội dung vẫn nằm dưới tên mục chứ không trôi xuống mục
// con cuối cùng (xem sectionInsertPoint để biết vì sao). Đề xuất của AI
// KHÔNG được tin ngay:
// placement.go đối chiếu nó với tiêu đề thật trong word/document.xml, chỉ
// chèn khi khớp đúng 1 mục; không khớp/khớp nhiều mục thì case đó lùi về phụ
// lục cuối tài liệu — thà để người đọc thấy ở phụ lục còn hơn chèn nhầm mục
// (lỗi âm thầm: file vẫn mở được, chỉ sai nội dung).
//
// .docx là 1 file zip chứa word/document.xml; phần chèn cuối tài liệu nằm
// ngay TRƯỚC <w:sectPr> cuối cùng (section properties của body PHẢI luôn là
// con cuối cùng của <w:body> theo chuẩn OOXML — chèn sau nó, tức ngay trước
// </w:body>, sẽ tạo ra file docx không hợp lệ).
//
// Giới hạn đã biết (cần nêu rõ với QA/PM): chỉ hỗ trợ tài liệu .docx dạng đơn
// giản (1 section); nhận diện mục dựa vào style Heading1..Heading6, tài liệu
// đánh tiêu đề bằng cách bôi đậm thủ công sẽ không khớp được mục nào và rơi
// hết về phụ lục. Nội dung của mục nằm trong bảng thì đoạn mới được chèn sau
// bảng chứ không thành 1 dòng mới của bảng. Ảnh minh hoạ chỉ được nhắc tới
// bằng tên object key dạng text, KHÔNG nhúng ảnh thật vào docx — nhúng ảnh
// cần thêm quan hệ (relationships) + content types phức tạp hơn nhiều, để
// dành cho một bản sau nếu cần; ảnh gốc vẫn xem được qua object store.
package docxmerge

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"

	"github.com/quangdung93/docs-hub-api/internal/module/urd/domain"
)

const documentXMLPath = "word/document.xml"

// maxEntryBytes chặn decompression bomb khi giải nén từng entry trong .docx
// (200 MiB thừa sức cho mọi tài liệu URD hợp lệ, nhưng chặn được 1 entry zip
// giả mạo giải nén ra dung lượng khổng lồ).
const maxEntryBytes = 200 << 20

// ErrUnsupportedStructure báo document.xml không có cấu trúc <w:body> đơn
// giản mà package này hỗ trợ (ví dụ thiếu <w:sectPr> lẫn </w:body>).
var ErrUnsupportedStructure = errors.New("docxmerge: cấu trúc document.xml không được hỗ trợ")

// ErrNoDocumentXML báo file không phải .docx hợp lệ (thiếu word/document.xml).
var ErrNoDocumentXML = errors.New("docxmerge: không tìm thấy word/document.xml")

// Merge đọc original (.docx hợp lệ), chèn nội dung từng case + hướng giải
// quyết vào đúng mục AI chỉ định (phần không xác định được mục thì gom vào
// mục "Phụ lục: Edge Case bổ sung (AI)" ở cuối), trả về bytes của file .docx
// mới. Các entry khác trong zip được copy nguyên vẹn.
func Merge(original []byte, cases []domain.EdgeCase) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(original), int64(len(original)))
	if err != nil {
		return nil, fmt.Errorf("đọc file .docx gốc: %w", err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	found := false
	for _, f := range zr.File {
		if err := copyOrMergeEntry(zw, f, cases, &found); err != nil {
			return nil, err
		}
	}
	if !found {
		return nil, ErrNoDocumentXML
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("đóng file .docx mới: %w", err)
	}
	return buf.Bytes(), nil
}

func copyOrMergeEntry(zw *zip.Writer, f *zip.File, cases []domain.EdgeCase, found *bool) error {
	w, err := zw.CreateHeader(&zip.FileHeader{Name: f.Name, Method: f.Method})
	if err != nil {
		return fmt.Errorf("ghi entry %s: %w", f.Name, err)
	}
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("đọc entry %s: %w", f.Name, err)
	}
	defer rc.Close()
	if f.Name != documentXMLPath {
		if err := copyEntryLimited(w, rc); err != nil {
			return fmt.Errorf("copy entry %s: %w", f.Name, err)
		}
		return nil
	}
	*found = true
	content, err := readEntryLimited(rc)
	if err != nil {
		return fmt.Errorf("đọc %s: %w", documentXMLPath, err)
	}
	merged, err := insertCases(content, cases)
	if err != nil {
		return err
	}
	if _, err := w.Write(merged); err != nil {
		return fmt.Errorf("ghi %s đã chèn: %w", documentXMLPath, err)
	}
	return nil
}

// copyEntryLimited copy tối đa maxEntryBytes+1 byte rồi báo lỗi nếu vượt —
// chặn decompression bomb (gosec G110) thay vì io.Copy không giới hạn.
func copyEntryLimited(w io.Writer, r io.Reader) error {
	n, err := io.CopyN(w, r, maxEntryBytes+1)
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("copy: %w", err)
	}
	if n > maxEntryBytes {
		return fmt.Errorf("entry vượt quá giới hạn %d bytes", maxEntryBytes)
	}
	return nil
}

// readEntryLimited tương tự copyEntryLimited nhưng đọc hẳn vào bộ nhớ (dùng
// cho word/document.xml — cần toàn bộ nội dung để chèn phụ lục).
func readEntryLimited(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxEntryBytes+1))
	if err != nil {
		return nil, fmt.Errorf("đọc entry: %w", err)
	}
	if len(data) > maxEntryBytes {
		return nil, fmt.Errorf("entry vượt quá giới hạn %d bytes", maxEntryBytes)
	}
	return data, nil
}

// insertCases chèn từng case vào đúng mục AI chỉ định; case nào không định
// vị được mục thì gom vào phụ lục cuối tài liệu.
func insertCases(documentXML []byte, cases []domain.EdgeCase) ([]byte, error) {
	bodyEnd := appendixOffset(documentXML)
	if bodyEnd == -1 {
		return nil, ErrUnsupportedStructure
	}
	// Không đọc được cấu trúc XML thì vẫn còn đường lui: gom tất cả vào phụ
	// lục theo vị trí chuỗi như bản trước. Bước này chạy trong luồng tạo
	// phiên bản URD mới — hỏng ở đây là người dùng mất trắng công đã nhập
	// (đã xảy ra với URD dạng PDF, xem usecase.finalizeAnalysis), nên tuyệt
	// đối không biến 1 tài liệu lạ cấu trúc thành lỗi cứng.
	paragraphs, err := scanBody(documentXML)
	if err != nil {
		paragraphs = nil
	}
	inserts, leftover := planInsertions(paragraphs, bodyEnd, cases)
	if len(leftover) > 0 {
		inserts = append(inserts, insertion{offset: bodyEnd, xml: buildAppendixXML(leftover)})
	}
	return applyInsertions(documentXML, inserts), nil
}

// appendixOffset trả vị trí ngay trước <w:sectPr> CUỐI CÙNG (dùng LastIndex
// vì văn bản nhiều section có thể có nhiều <w:sectPr> lồng trong <w:pPr>,
// chỉ cái cuối cùng mới là section properties của <w:body>), -1 nếu không
// còn điểm neo nào.
func appendixOffset(documentXML []byte) int {
	if at := bytes.LastIndex(documentXML, []byte("<w:sectPr")); at != -1 {
		return at
	}
	return bytes.LastIndex(documentXML, []byte("</w:body>"))
}

func buildAppendixXML(cases []domain.EdgeCase) []byte {
	var buf bytes.Buffer
	buf.WriteString(heading("Phụ lục: Edge Case bổ sung (AI)"))
	for i, c := range cases {
		buf.WriteString(paragraph(fmt.Sprintf("Case %d: %s", i+1, c.Description)))
		buf.WriteString(paragraph("Hướng giải quyết: " + resolutionText(c)))
	}
	return buf.Bytes()
}

// caseText là nội dung 1 dòng chèn thẳng vào mục — gộp mô tả + hướng giải
// quyết, kèm hậu tố "(AI)" để người review biết đoạn nào do hệ thống bổ
// sung: nội dung giờ nằm rải trong thân bài chứ không gom 1 chỗ như phụ lục
// nữa, không đánh dấu thì không truy vết được.
func caseText(c domain.EdgeCase) string {
	return fmt.Sprintf("%s Hướng xử lý: %s (AI)", c.Description, resolutionText(c))
}

// resolutionText ghép hướng giải quyết với chú thích ảnh đính kèm (nếu có).
func resolutionText(c domain.EdgeCase) string {
	if c.ImageObjectKey == "" {
		return c.Resolution
	}
	return fmt.Sprintf("%s (ảnh đính kèm: %s)", c.Resolution, c.ImageObjectKey)
}

// inlineParagraph dựng đoạn văn chèn thẳng vào mục, kế thừa <w:pPr> của đoạn
// nội dung cuối cùng trong mục để giữ đúng bullet/đánh số của mục đó.
func inlineParagraph(pPr []byte, text string) string {
	return `<w:p>` + string(pPr) + `<w:r><w:t xml:space="preserve">` + xmlEscape(text) + `</w:t></w:r></w:p>`
}

func heading(text string) string {
	return `<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t xml:space="preserve">` +
		xmlEscape(text) + `</w:t></w:r></w:p>`
}

func paragraph(text string) string {
	return `<w:p><w:r><w:t xml:space="preserve">` + xmlEscape(text) + `</w:t></w:r></w:p>`
}

func xmlEscape(s string) string {
	var buf bytes.Buffer
	if err := xml.EscapeText(&buf, []byte(s)); err != nil {
		return s
	}
	return buf.String()
}
