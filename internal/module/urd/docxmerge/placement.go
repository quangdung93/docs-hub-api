package docxmerge

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/quangdung93/docs-hub-api/internal/module/urd/domain"
)

// maxHeadingLevel là cấp tiêu đề sâu nhất Word dùng (Heading1..Heading6).
const maxHeadingLevel = 6

// bodyParagraph là 1 đoạn văn nằm TRỰC TIẾP trong <w:body> — không nằm trong
// <w:tbl> — kèm vị trí byte của nó trong document.xml, đủ để chèn nội dung
// mới ngay trước nó.
//
// Đoạn văn trong bảng bị loại khỏi danh sách này vì chèn 1 <w:p> vào giữa
// bảng sẽ rơi vào TRONG ô, không phải nội dung tiếp theo của mục. Hệ quả:
// mục có nội dung nằm hết trong bảng thì điểm chèn rơi xuống sau bảng —
// vẫn đúng mục, chỉ không chèn thành 1 dòng mới của bảng.
type bodyParagraph struct {
	// start là vị trí byte của ký tự "<" trong "<w:p".
	start int
	// text là toàn bộ nội dung <w:t> của đoạn, đã gộp lại.
	text string
	// headingLevel là 1..6 nếu pStyle là Heading1..Heading6, 0 nếu không phải
	// tiêu đề mục. Theo đúng quy ước mà parser trích xuất text đang dùng
	// (internal/module/ingestion/parser_office.go) — cùng 1 tài liệu thì AI
	// nhìn thấy tiêu đề nào, ở đây cũng nhận ra tiêu đề đó.
	headingLevel int
	// pPr là XML nguyên bản của <w:pPr>, copy sang đoạn chèn mới để giữ đúng
	// style/bullet/đánh số của mục. nil nếu đoạn không có pPr, hoặc pPr có
	// chứa <w:sectPr> (copy vào sẽ tạo ngắt section — không an toàn).
	pPr []byte
}

// insertion là 1 lần chèn nội dung vào vị trí byte cụ thể trong document.xml.
type insertion struct {
	offset int
	xml    []byte
}

// scanBody đọc document.xml và trả về danh sách đoạn văn cấp <w:body> theo
// đúng thứ tự xuất hiện, kèm vị trí byte.
func scanBody(documentXML []byte) ([]bodyParagraph, error) {
	decoder := xml.NewDecoder(bytes.NewReader(documentXML))
	s := bodyScanner{documentXML: documentXML}
	for {
		// InputOffset() TRƯỚC khi đọc token = vị trí bắt đầu của token sắp
		// đọc (khoảng trắng giữa các thẻ cũng là 1 token CharData riêng).
		offset := int(decoder.InputOffset())
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", documentXMLPath, err)
		}
		s.handle(token, offset, int(decoder.InputOffset()))
	}
	return s.paragraphs, nil
}

// bodyScanner giữ trạng thái khi duyệt token của document.xml.
type bodyScanner struct {
	documentXML []byte
	paragraphs  []bodyParagraph
	current     *bodyParagraph
	text        strings.Builder
	tableDepth  int
	pPrStart    int
	inPPr       bool
	inText      bool
	pPrHasSect  bool
}

func (s *bodyScanner) handle(token xml.Token, start, end int) {
	switch node := token.(type) {
	case xml.StartElement:
		s.startElement(node, start)
	case xml.CharData:
		if s.inText {
			s.text.Write(node)
		}
	case xml.EndElement:
		s.endElement(node, end)
	}
}

func (s *bodyScanner) startElement(node xml.StartElement, start int) {
	switch node.Name.Local {
	case "tbl":
		s.tableDepth++
	case "p":
		if s.tableDepth == 0 && s.current == nil {
			s.current = &bodyParagraph{start: start}
			s.text.Reset()
			s.pPrHasSect = false
		}
	default:
		if s.current != nil {
			s.startParagraphChild(node, start)
		}
	}
}

// startParagraphChild xử lý các thẻ nằm trong 1 đoạn văn cấp body đang mở.
func (s *bodyScanner) startParagraphChild(node xml.StartElement, start int) {
	switch node.Name.Local {
	case "pPr":
		if s.current.pPr == nil && !s.inPPr {
			s.inPPr = true
			s.pPrStart = start
		}
	case "pStyle":
		if s.inPPr {
			s.current.headingLevel = headingLevel(attrValue(node.Attr, "val"))
		}
	case "sectPr":
		// <w:sectPr> lồng trong <w:pPr> = ngắt section: không copy pPr này
		// sang đoạn chèn mới, nếu không sẽ tạo thêm 1 ngắt section lạ.
		if s.inPPr {
			s.pPrHasSect = true
		}
	case "t":
		s.inText = true
	}
}

func (s *bodyScanner) endElement(node xml.EndElement, end int) {
	switch node.Name.Local {
	case "tbl":
		s.tableDepth--
	case "t":
		s.inText = false
	case "pPr":
		if s.inPPr {
			s.inPPr = false
			if !s.pPrHasSect {
				s.current.pPr = s.documentXML[s.pPrStart:end]
			}
		}
	case "p":
		if s.current != nil {
			s.current.text = strings.TrimSpace(s.text.String())
			s.paragraphs = append(s.paragraphs, *s.current)
			s.current = nil
		}
	}
}

func attrValue(attrs []xml.Attr, name string) string {
	for _, attr := range attrs {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

// headingLevel đổi pStyle thành cấp tiêu đề: "Heading2" -> 2, khác -> 0.
func headingLevel(style string) int {
	lower := strings.ToLower(style)
	if !strings.HasPrefix(lower, "heading") {
		return 0
	}
	level, err := strconv.Atoi(strings.TrimPrefix(lower, "heading"))
	if err != nil || level < 1 || level > maxHeadingLevel {
		return 0
	}
	return level
}

// normalizeHeading chuẩn hoá tiêu đề trước khi so khớp. AI copy tiêu đề từ
// bản text đã trích xuất — nơi parser thêm tiền tố "#" cho Heading1..6 — nên
// phải bỏ "#", gộp khoảng trắng thừa, bỏ dấu câu cuối và so khớp không phân
// biệt hoa/thường.
func normalizeHeading(heading string) string {
	normalized := strings.TrimSpace(heading)
	normalized = strings.TrimLeft(normalized, "#")
	normalized = strings.Join(strings.Fields(normalized), " ")
	normalized = strings.Trim(normalized, " .:;")
	return strings.ToLower(normalized)
}

// planInsertions chia cases làm 2 nhóm: nhóm chèn THẲNG được vào mục AI chỉ
// định, và nhóm còn lại (leftover) phải gom xuống phụ lục cuối tài liệu.
//
// bodyEnd là vị trí kết thúc phần nội dung (ngay trước <w:sectPr> của body).
func planInsertions(
	paragraphs []bodyParagraph, bodyEnd int, cases []domain.EdgeCase,
) (inserts []insertion, leftover []domain.EdgeCase) {
	for _, c := range cases {
		index := matchHeading(paragraphs, c.TargetHeading)
		if index < 0 {
			leftover = append(leftover, c)
			continue
		}
		offset, pPr := sectionInsertPoint(paragraphs, index, bodyEnd)
		inserts = append(inserts, insertion{offset: offset, xml: []byte(inlineParagraph(pPr, caseText(c)))})
	}
	return inserts, leftover
}

// matchHeading tìm đoạn tiêu đề khớp với đề xuất của AI, trả -1 nếu không
// khớp hoặc khớp NHIỀU HƠN 1 mục.
//
// Khớp nhiều mục (tài liệu có 2 tiêu đề trùng tên) cố ý bị coi như không
// khớp: chèn vào nhầm mục là lỗi âm thầm — file vẫn mở được, chỉ sai nội
// dung — nên thà lùi về phụ lục để người đọc tự thấy còn hơn đoán bừa.
func matchHeading(paragraphs []bodyParagraph, targetHeading string) int {
	target := normalizeHeading(targetHeading)
	if target == "" {
		return -1
	}
	found := -1
	for i, p := range paragraphs {
		if p.headingLevel == 0 || normalizeHeading(p.text) != target {
			continue
		}
		if found >= 0 {
			return -1
		}
		found = i
	}
	return found
}

// sectionInsertPoint trả vị trí chèn (cuối mục, ngay trước tiêu đề cùng cấp
// hoặc cấp cao hơn tiếp theo) và pPr của đoạn nội dung cuối cùng trong mục
// để đoạn chèn mới kế thừa đúng bullet/đánh số.
func sectionInsertPoint(paragraphs []bodyParagraph, index, bodyEnd int) (offset int, pPr []byte) {
	level := paragraphs[index].headingLevel
	offset = bodyEnd
	for j := index + 1; j < len(paragraphs); j++ {
		if paragraphs[j].headingLevel > 0 && paragraphs[j].headingLevel <= level {
			offset = paragraphs[j].start
			break
		}
		// Chỉ kế thừa style từ đoạn nội dung thường, không lấy của tiêu đề
		// con — nếu không đoạn chèn sẽ thành 1 tiêu đề mới.
		if paragraphs[j].headingLevel == 0 && paragraphs[j].pPr != nil {
			pPr = paragraphs[j].pPr
		}
	}
	return offset, pPr
}

// applyInsertions chèn các đoạn nội dung vào document.xml. Các lần chèn cùng
// vị trí được gộp theo đúng thứ tự case, rồi chèn từ CUỐI lên ĐẦU để vị trí
// của các lần chèn phía trước không bị dịch.
func applyInsertions(documentXML []byte, inserts []insertion) []byte {
	if len(inserts) == 0 {
		return documentXML
	}
	merged := make(map[int][]byte, len(inserts))
	offsets := make([]int, 0, len(inserts))
	for _, ins := range inserts {
		if _, seen := merged[ins.offset]; !seen {
			offsets = append(offsets, ins.offset)
		}
		merged[ins.offset] = append(merged[ins.offset], ins.xml...)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(offsets)))
	out := documentXML
	for _, offset := range offsets {
		chunk := merged[offset]
		next := make([]byte, 0, len(out)+len(chunk))
		next = append(next, out[:offset]...)
		next = append(next, chunk...)
		next = append(next, out[offset:]...)
		out = next
	}
	return out
}
