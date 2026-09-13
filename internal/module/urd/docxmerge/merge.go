// Package docxmerge chèn thêm nội dung edge case vào cuối 1 file .docx đã có,
// tạo ra 1 file .docx mới — dùng cho bước "tạo phiên bản URD mới" của tính
// năng Nhận diện & Phân tích Edge Case cho URD (URD v1.2 mục XI).
//
// .docx là 1 file zip chứa word/document.xml; ta chèn thêm các đoạn <w:p>
// ngay TRƯỚC <w:sectPr> cuối cùng (section properties của body PHẢI luôn là
// con cuối cùng của <w:body> theo chuẩn OOXML — chèn sau nó, tức ngay trước
// </w:body>, sẽ tạo ra file docx không hợp lệ).
//
// Giới hạn đã biết (cần nêu rõ với QA/PM): chỉ hỗ trợ tài liệu .docx dạng đơn
// giản (1 section). Ảnh minh hoạ chỉ được nhắc tới bằng tên object key dạng
// text, KHÔNG nhúng ảnh thật vào docx — nhúng ảnh cần thêm quan hệ
// (relationships) + content types phức tạp hơn nhiều, để dành cho một bản sau
// nếu cần; ảnh gốc vẫn xem được qua object store.
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

// Merge đọc original (.docx hợp lệ), chèn thêm mục "Phụ lục: Edge Case bổ
// sung (AI)" liệt kê từng case + hướng giải quyết, trả về bytes của file
// .docx mới. Các entry khác trong zip được copy nguyên vẹn.
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
	merged, err := insertAppendix(content, cases)
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

// insertAppendix chèn các đoạn văn mới ngay trước <w:sectPr> CUỐI CÙNG (dùng
// LastIndex vì văn bản nhiều section có thể có nhiều <w:sectPr> lồng trong
// <w:pPr>, chỉ cái cuối cùng mới là section properties của <w:body>).
func insertAppendix(documentXML []byte, cases []domain.EdgeCase) ([]byte, error) {
	insertAt := bytes.LastIndex(documentXML, []byte("<w:sectPr"))
	if insertAt == -1 {
		insertAt = bytes.LastIndex(documentXML, []byte("</w:body>"))
	}
	if insertAt == -1 {
		return nil, ErrUnsupportedStructure
	}
	appendix := buildAppendixXML(cases)
	out := make([]byte, 0, len(documentXML)+len(appendix))
	out = append(out, documentXML[:insertAt]...)
	out = append(out, appendix...)
	out = append(out, documentXML[insertAt:]...)
	return out, nil
}

func buildAppendixXML(cases []domain.EdgeCase) []byte {
	var buf bytes.Buffer
	buf.WriteString(heading("Phụ lục: Edge Case bổ sung (AI)"))
	for i, c := range cases {
		buf.WriteString(paragraph(fmt.Sprintf("Case %d: %s", i+1, c.Description)))
		resolution := c.Resolution
		if c.ImageObjectKey != "" {
			resolution += fmt.Sprintf(" (ảnh đính kèm: %s)", c.ImageObjectKey)
		}
		buf.WriteString(paragraph("Hướng giải quyết: " + resolution))
	}
	return buf.Bytes()
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
