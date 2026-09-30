package docxmerge

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/quangdung93/docs-hub-api/internal/module/urd/domain"
)

// rowIDPattern khớp ô mã ở cột đầu của bảng BR/AC: "BR-19", "AC-11", "BR_01",
// "UC 3". Chỉ bảng có cột mã kiểu này mới được thêm dòng — bảng khác (STT
// thuần số, bảng thông tin chung…) vẫn chèn đoạn văn sau bảng như cũ, vì
// không biết chắc nội dung case hợp với cột nào.
var rowIDPattern = regexp.MustCompile(`^([A-Za-z]{1,6})([-_ ]?)(\d{1,4})$`) //nolint:gochecknoglobals // regex bất biến, biên dịch 1 lần

// bodyTable là 1 bảng nằm TRỰC TIẾP trong <w:body> (không lồng trong bảng
// khác), kèm vị trí byte của thẻ đóng </w:tbl> — điểm chèn dòng mới.
type bodyTable struct {
	start    int
	end      int
	rowCount int
	// lastRow là dòng cuối của bảng: khuôn định dạng cho dòng chèn thêm.
	lastRow tableRow
}

// tableRow giữ XML định dạng của 1 dòng — đủ để dựng dòng mới cùng kiểu mà
// không phải copy nguyên nội dung cũ.
type tableRow struct {
	trPr  []byte
	cells []tableCell
	// hasVMerge: dòng có ô gộp dọc. Copy vMerge sang dòng mới sẽ gộp ô mới
	// vào ô phía trên — sai bố cục, nên dòng kiểu này không dùng làm khuôn.
	hasVMerge bool
}

// tableCell giữ định dạng của 1 ô: tcPr (độ rộng, nền…), pPr và rPr của đoạn
// văn/run ĐẦU TIÊN trong ô (căn lề, font, cỡ chữ), cùng chữ trong ô.
type tableCell struct {
	tcPr, pPr, rPr []byte
	text           string
}

// idTemplate cho biết bảng có dùng được làm đích chèn dòng không, và mã của
// dòng cuối đã tách phần tiền tố/số.
type idTemplate struct {
	prefix, sep string
	number      int
	width       int
}

// rowTemplate trả khuôn mã của bảng nếu bảng đủ điều kiện thêm dòng: có ít
// nhất 1 dòng tiêu đề + 1 dòng dữ liệu, dòng cuối không gộp ô dọc, có từ 2 cột
// trở lên và ô đầu là mã kiểu "BR-19".
func (t bodyTable) rowTemplate() (idTemplate, bool) {
	row := t.lastRow
	if t.rowCount < 2 || row.hasVMerge || len(row.cells) < 2 {
		return idTemplate{}, false
	}
	m := rowIDPattern.FindStringSubmatch(strings.TrimSpace(row.cells[0].text))
	if m == nil {
		return idTemplate{}, false
	}
	number, err := strconv.Atoi(m[3])
	if err != nil {
		return idTemplate{}, false
	}
	return idTemplate{prefix: m[1], sep: m[2], number: number, width: len(m[3])}, true
}

// id dựng mã thứ n tính từ mã dòng cuối, giữ nguyên tiền tố, dấu nối và số
// chữ số ("BR-09" + 1 -> "BR-10", "AC-011" + 1 -> "AC-012").
func (id idTemplate) id(offset int) string {
	return fmt.Sprintf("%s%s%0*d", id.prefix, id.sep, id.width, id.number+offset)
}

// rowCellTexts chia nội dung case vào các cột của dòng mới:
//   - 2 cột (ID | Quy tắc): mô tả + hướng xử lý chung 1 ô, như đoạn chèn thẳng.
//   - từ 3 cột (ID | Given | When | Then): mô tả vào cột thứ 2 (bối cảnh), hướng
//     xử lý vào cột cuối (kết quả mong đợi), các cột giữa để trống — AI không
//     tách được "When" riêng nên để người review điền, không bịa nội dung.
func rowCellTexts(columns int, id string, c domain.EdgeCase) []string {
	texts := make([]string, columns)
	texts[0] = id
	if columns == 2 {
		texts[1] = caseText(c)
		return texts
	}
	texts[1] = c.Description
	texts[columns-1] = resolutionText(c) + " (AI)"
	return texts
}

// buildRowXML dựng 1 <w:tr> mới theo khuôn định dạng của dòng cuối bảng.
func buildRowXML(template tableRow, texts []string) []byte {
	var b strings.Builder
	b.WriteString(`<w:tr>`)
	b.Write(template.trPr)
	for i, cell := range template.cells {
		b.WriteString(`<w:tc>`)
		b.Write(cell.tcPr)
		b.WriteString(`<w:p>`)
		b.Write(cell.pPr)
		if texts[i] != "" {
			b.WriteString(`<w:r>`)
			b.Write(cell.rPr)
			b.WriteString(`<w:t xml:space="preserve">`)
			b.WriteString(xmlEscape(texts[i]))
			b.WriteString(`</w:t></w:r>`)
		}
		b.WriteString(`</w:p></w:tc>`)
	}
	b.WriteString(`</w:tr>`)
	return []byte(b.String())
}

// sectionTable tìm bảng CUỐI CÙNG nằm trong phần nội dung trực tiếp của mục
// [sectionStart, sectionEnd) mà thêm dòng được, trả chỉ số bảng hoặc -1.
func sectionTable(tables []bodyTable, sectionStart, sectionEnd int) int {
	found := -1
	for i, t := range tables {
		if t.start <= sectionStart || t.end >= sectionEnd {
			continue
		}
		if _, ok := t.rowTemplate(); ok {
			found = i
		}
	}
	return found
}

// tableScanner theo dõi các bảng cấp body trong lúc duyệt token, độc lập với
// phần theo dõi đoạn văn của bodyScanner. Bảng lồng trong ô (depth > 1) bị bỏ
// qua hoàn toàn: dòng/ô của nó không phải dòng/ô của bảng BR/AC.
type tableScanner struct {
	documentXML []byte
	tables      []bodyTable
	depth       int
	table       bodyTable
	row         *tableRow
	cell        *tableCell
	cellParas   int
	inRun       bool
	inText      bool
	text        strings.Builder
	// capture là thẻ định dạng đang được cắt nguyên văn (trPr/tcPr/pPr/rPr);
	// rỗng khi không cắt gì.
	capture      string
	captureStart int
}

func (s *tableScanner) handle(token xml.Token, start, end int) {
	switch node := token.(type) {
	case xml.StartElement:
		s.startElement(node, start)
	case xml.CharData:
		if s.inText && s.depth == 1 {
			s.text.Write(node)
		}
	case xml.EndElement:
		s.endElement(node, start, end)
	}
}

func (s *tableScanner) startElement(node xml.StartElement, start int) {
	name := node.Name.Local
	if name == "tbl" {
		s.depth++
		if s.depth == 1 {
			s.table = bodyTable{start: start}
		}
		return
	}
	if s.depth != 1 {
		return
	}
	switch name {
	case "tr":
		s.row = &tableRow{}
	case "tc":
		if s.row != nil {
			s.cell = &tableCell{}
			s.cellParas = 0
			s.text.Reset()
		}
	case "p":
		if s.cell != nil {
			s.cellParas++
		}
	case "r":
		s.inRun = true
	case "t":
		s.inText = true
	case "vMerge":
		if s.row != nil && s.capture == "tcPr" {
			s.row.hasVMerge = true
		}
	}
	s.beginCapture(name, start)
}

// beginCapture bắt đầu cắt XML của thẻ định dạng cần giữ làm khuôn. Chỉ lấy
// pPr/rPr của đoạn văn đầu tiên trong ô, và rPr phải nằm trong run (rPr trong
// pPr là định dạng dấu xuống dòng, không phải định dạng chữ).
func (s *tableScanner) beginCapture(name string, start int) {
	if s.capture != "" {
		return
	}
	switch {
	case name == "trPr" && s.row != nil && s.cell == nil,
		name == "tcPr" && s.cell != nil,
		name == "pPr" && s.cell != nil && s.cellParas == 1 && s.cell.pPr == nil,
		name == "rPr" && s.cell != nil && s.cellParas == 1 && s.inRun && s.cell.rPr == nil:
		s.capture, s.captureStart = name, start
	}
}

func (s *tableScanner) endElement(node xml.EndElement, start, end int) {
	name := node.Name.Local
	if name == "tbl" {
		if s.depth == 1 {
			s.table.end = start
			s.tables = append(s.tables, s.table)
		}
		s.depth--
		return
	}
	if s.depth != 1 {
		return
	}
	if name == s.capture {
		s.endCapture(s.documentXML[s.captureStart:end])
	}
	switch name {
	case "r":
		s.inRun = false
	case "t":
		s.inText = false
	case "tc":
		if s.cell != nil && s.row != nil {
			s.cell.text = s.text.String()
			s.row.cells = append(s.row.cells, *s.cell)
		}
		s.cell = nil
	case "tr":
		if s.row != nil {
			s.table.lastRow = *s.row
			s.table.rowCount++
		}
		s.row = nil
	}
}

func (s *tableScanner) endCapture(raw []byte) {
	switch s.capture {
	case "trPr":
		s.row.trPr = raw
	case "tcPr":
		s.cell.tcPr = raw
	case "pPr":
		s.cell.pPr = raw
	case "rPr":
		s.cell.rPr = raw
	}
	s.capture = ""
}
