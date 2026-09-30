// Package docxheading xác định đoạn văn nào trong word/document.xml là tiêu đề
// mục và ở cấp nào. Dùng chung cho parser trích xuất text (ingestion) và
// docxmerge (chèn edge case vào URD) — hai nơi PHẢI nhìn thấy cùng một bộ
// tiêu đề, nếu không AI sẽ đề xuất một tiêu đề mà bước chèn không nhận ra.
//
// Thứ tự nhận diện:
//  1. outlineLvl đặt thẳng trên đoạn văn.
//  2. Style của đoạn: outlineLvl trong styles.xml, tên built-in "heading N"
//     (Word bản địa hoá vẫn lưu tên tiếng Anh dù styleId là "u1", "u2"…), hoặc
//     styleId "HeadingN"; có kế thừa qua basedOn.
//  3. Dự phòng cho tài liệu không đánh tiêu đề bằng style: đoạn ngắn nằm ngoài
//     bảng, in đậm toàn bộ, có cỡ chữ riêng và mở đầu bằng số mục ("A.", "I.",
//     "1.", "2.1", "F2 –"). Cấp suy ra từ cỡ chữ: to nhất là cấp 1.
//
// Bước 3 chỉ bổ sung khi số đoạn ứng viên NHIỀU HƠN số tiêu đề tìm được ở bước
// 1-2. Tài liệu dùng style chuẩn thì không bị heuristic làm nhiễu (vd một dòng
// in đậm "1. Lưu ý" trong thân bài); còn URD bôi đậm thủ công mà chỉ có vài
// đoạn mang style tiêu đề — như chính mục "Phụ lục" do docxmerge thêm vào ở
// lần tạo phiên bản trước — vẫn được nhận diện đủ mục.
package docxheading

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// MaxLevel là cấp tiêu đề sâu nhất được nhận (Heading1..Heading6).
const MaxLevel = 6

// maxStyleChain chặn vòng lặp basedOn tự tham chiếu trong styles.xml hỏng.
const maxStyleChain = 16

// maxFallbackRunes: tiêu đề mục hiếm khi dài hơn thế; đoạn in đậm dài hơn
// thường là câu nhấn mạnh trong thân bài.
const maxFallbackRunes = 120

// numberedHeading khớp phần mở đầu kiểu số mục của tiêu đề: "A. ", "III. ",
// "1. ", "2.1 ", "F2 – ", "UC-01: ".
var numberedHeading = regexp.MustCompile( //nolint:gochecknoglobals // regex bất biến, biên dịch 1 lần
	`^(?:[A-Z]|[IVXLCDM]{1,6}|\d{1,3}(?:\.\d{1,3})*|[A-Z]{1,4}[-_]?\d{1,4}(?:\.\d{1,3})*)` +
		`(?:\s*[.)\-–—:]\s*|\s+)\S`)

// Styles ánh xạ styleId -> cấp tiêu đề (1..MaxLevel). Style không phải tiêu đề
// không có mặt trong map.
type Styles map[string]int

// Format là định dạng thu được khi duyệt các run của một đoạn văn.
type Format struct {
	// OutlineLevel là outlineLvl đặt thẳng trên đoạn (đã +1 để 1 = cấp 1);
	// 0 nếu không có.
	OutlineLevel int
	// Bold đúng khi MỌI run có chữ đều in đậm.
	Bold bool
	// FontSize là cỡ chữ lớn nhất (đơn vị half-point, như w:sz) trong các
	// run có chữ; 0 nếu không run nào đặt cỡ chữ tường minh.
	FontSize int
}

// Paragraph là dữ liệu tối thiểu để quyết định một đoạn có phải tiêu đề.
type Paragraph struct {
	StyleID string
	Text    string
	InTable bool
	Format  Format
}

// ParseStyles đọc word/styles.xml. Thiếu file hoặc XML hỏng thì trả map rỗng:
// tài liệu vẫn xử lý được nhờ styleId "HeadingN" và heuristic dự phòng, không
// có lý do biến nó thành lỗi cứng.
func ParseStyles(stylesXML []byte) Styles {
	type rawStyle struct {
		name, basedOn string
		outline       int
	}
	raw := map[string]rawStyle{}
	decoder := xml.NewDecoder(bytes.NewReader(stylesXML))
	var id string
	var current rawStyle
	for {
		token, err := decoder.Token()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return Styles{}
			}
			break
		}
		switch node := token.(type) {
		case xml.StartElement:
			switch node.Name.Local {
			case "style":
				id, current = attr(node.Attr, "styleId"), rawStyle{}
			case "name":
				current.name = attr(node.Attr, "val")
			case "basedOn":
				current.basedOn = attr(node.Attr, "val")
			case "outlineLvl":
				current.outline = outlineLevel(attr(node.Attr, "val"))
			}
		case xml.EndElement:
			if node.Name.Local == "style" && id != "" {
				raw[id] = current
				id = ""
			}
		}
	}

	styles := Styles{}
	for styleID := range raw {
		next := styleID
		for range maxStyleChain {
			s, ok := raw[next]
			if !ok {
				break
			}
			level := s.outline
			if level == 0 {
				level = levelFromName(s.name)
			}
			if level == 0 {
				level = levelFromName(next)
			}
			if level > 0 {
				styles[styleID] = level
				break
			}
			if s.basedOn == "" {
				break
			}
			next = s.basedOn
		}
	}
	return styles
}

// Levels trả cấp tiêu đề cho từng đoạn (cùng thứ tự đầu vào), 0 nếu không phải
// tiêu đề.
func Levels(styles Styles, paragraphs []Paragraph) []int {
	levels := make([]int, len(paragraphs))
	explicit, candidates := 0, 0
	for i, p := range paragraphs {
		levels[i] = explicitLevel(styles, p)
		if levels[i] > 0 {
			explicit++
		} else if isFallbackCandidate(p) {
			candidates++
		}
	}
	if candidates <= explicit {
		return levels
	}

	// Dự phòng: xếp hạng các cỡ chữ của đoạn ứng viên, to nhất là cấp 1.
	sizeRank := map[int]int{}
	var sizes []int
	for i, p := range paragraphs {
		if levels[i] == 0 && isFallbackCandidate(p) {
			if _, seen := sizeRank[p.Format.FontSize]; !seen {
				sizeRank[p.Format.FontSize] = 0
				sizes = append(sizes, p.Format.FontSize)
			}
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(sizes)))
	for rank, size := range sizes {
		sizeRank[size] = min(rank+1, MaxLevel)
	}
	for i, p := range paragraphs {
		if levels[i] == 0 && isFallbackCandidate(p) {
			levels[i] = sizeRank[p.Format.FontSize]
		}
	}
	return levels
}

func explicitLevel(styles Styles, p Paragraph) int {
	if p.Format.OutlineLevel > 0 {
		return p.Format.OutlineLevel
	}
	if level, ok := styles[p.StyleID]; ok {
		return level
	}
	return levelFromName(p.StyleID)
}

func isFallbackCandidate(p Paragraph) bool {
	text := strings.TrimSpace(p.Text)
	return !p.InTable && p.Format.Bold && p.Format.FontSize > 0 &&
		text != "" && utf8.RuneCountInString(text) <= maxFallbackRunes &&
		numberedHeading.MatchString(text)
}

// levelFromName nhận "Heading2", "heading 2" -> 2; khác -> 0.
func levelFromName(name string) int {
	lower := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), " ", "")
	if !strings.HasPrefix(lower, "heading") {
		return 0
	}
	level, err := strconv.Atoi(strings.TrimPrefix(lower, "heading"))
	if err != nil || level < 1 || level > MaxLevel {
		return 0
	}
	return level
}

// outlineLevel đổi w:outlineLvl (0-based, 9 = thân bài) thành cấp 1..MaxLevel.
func outlineLevel(value string) int {
	level, err := strconv.Atoi(value)
	if err != nil || level < 0 || level >= MaxLevel {
		return 0
	}
	return level + 1
}

func attr(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// Tracker gom định dạng của MỘT đoạn văn trong lúc duyệt token XML. Gọi Reset
// khi gặp <w:p>, chuyển mọi StartElement/EndElement và phần chữ trong <w:t>
// của đoạn vào Start/End/Text, rồi đọc Format khi gặp </w:p>.
type Tracker struct {
	inPPr, inRun, inRPr bool
	runBold, runText    bool
	runSize             int
	format              Format
	anyText, allBold    bool
}

// Reset chuẩn bị cho một đoạn văn mới.
func (t *Tracker) Reset() {
	*t = Tracker{allBold: true}
}

// Start xử lý thẻ mở.
func (t *Tracker) Start(node xml.StartElement) {
	switch node.Name.Local {
	case "pPr":
		t.inPPr = true
	case "outlineLvl":
		if t.inPPr && !t.inRun {
			t.format.OutlineLevel = outlineLevel(attr(node.Attr, "val"))
		}
	case "r":
		t.inRun, t.runBold, t.runText, t.runSize = true, false, false, 0
	case "rPr":
		t.inRPr = t.inRun
	case "b":
		if t.inRPr {
			t.runBold = isOn(attr(node.Attr, "val"))
		}
	case "sz":
		if t.inRPr {
			if size, err := strconv.Atoi(attr(node.Attr, "val")); err == nil {
				t.runSize = size
			}
		}
	}
}

// Text ghi nhận chữ nằm trong <w:t> của run hiện tại.
func (t *Tracker) Text(data []byte) {
	if t.inRun && len(bytes.TrimSpace(data)) > 0 {
		t.runText = true
	}
}

// End xử lý thẻ đóng.
func (t *Tracker) End(node xml.EndElement) {
	switch node.Name.Local {
	case "pPr":
		t.inPPr = false
	case "rPr":
		t.inRPr = false
	case "r":
		if t.inRun && t.runText {
			t.anyText = true
			t.allBold = t.allBold && t.runBold
			t.format.FontSize = max(t.format.FontSize, t.runSize)
		}
		t.inRun = false
	}
}

// Format trả định dạng đã gom của đoạn văn.
func (t *Tracker) Format() Format {
	f := t.format
	f.Bold = t.anyText && t.allBold
	return f
}

// isOn đọc giá trị bật/tắt của OOXML: vắng mặt = bật.
func isOn(value string) bool {
	switch strings.ToLower(value) {
	case "0", "false", "off", "none":
		return false
	default:
		return true
	}
}
