package docxheading

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseStyles(t *testing.T) {
	stylesXML := []byte(`<w:styles xmlns:w="w">
<w:style w:type="paragraph" w:styleId="Normal"><w:name w:val="Normal"/></w:style>
<w:style w:type="paragraph" w:styleId="u1"><w:name w:val="heading 1"/><w:basedOn w:val="Normal"/></w:style>
<w:style w:type="paragraph" w:styleId="Heading2"><w:name w:val="heading 2"/></w:style>
<w:style w:type="paragraph" w:styleId="MucLon"><w:name w:val="Muc lon"/>
  <w:pPr><w:outlineLvl w:val="2"/></w:pPr></w:style>
<w:style w:type="paragraph" w:styleId="MucCon"><w:name w:val="Muc con"/><w:basedOn w:val="Heading2"/></w:style>
<w:style w:type="paragraph" w:styleId="ThanBai"><w:pPr><w:outlineLvl w:val="9"/></w:pPr></w:style>
<w:style w:type="character" w:styleId="Heading1Char"><w:name w:val="Heading 1 Char"/></w:style>
<w:style w:type="paragraph" w:styleId="Loop"><w:basedOn w:val="Loop"/></w:style>
</w:styles>`)

	styles := ParseStyles(stylesXML)

	require.Equal(t, 1, styles["u1"], "Word tiếng Việt: styleId u1 nhưng tên built-in vẫn là heading 1")
	require.Equal(t, 2, styles["Heading2"])
	require.Equal(t, 3, styles["MucLon"], "outlineLvl 2 (0-based) là cấp 3")
	require.Equal(t, 2, styles["MucCon"], "kế thừa qua basedOn")
	require.NotContains(t, styles, "Normal")
	require.NotContains(t, styles, "ThanBai", "outlineLvl 9 là thân bài")
	require.NotContains(t, styles, "Heading1Char")
	require.NotContains(t, styles, "Loop")
}

func TestParseStyles_XMLHongTraMapRong(t *testing.T) {
	require.Empty(t, ParseStyles([]byte(`<w:styles><w:style`)))
	require.Empty(t, ParseStyles(nil))
}

func TestLevels_UuTienStyleVaBoQuaHeuristic(t *testing.T) {
	paragraphs := []Paragraph{
		{StyleID: "Heading1", Text: "Chuc nang"},
		// In đậm có số mục nhưng tài liệu đã dùng style tiêu đề: không phải tiêu đề.
		{Text: "1. Luu y", Format: Format{Bold: true, FontSize: 28}},
		{Text: "Noi dung", Format: Format{OutlineLevel: 2}},
	}
	require.Equal(t, []int{1, 0, 2}, Levels(Styles{}, paragraphs))
}

func TestLevels_DuPhongTheoInDamVaCoChu(t *testing.T) {
	bold := func(text string, size int) Paragraph {
		return Paragraph{Text: text, Format: Format{Bold: true, FontSize: size}}
	}
	paragraphs := []Paragraph{
		bold("Revision History", 24),   // không có số mục
		bold("A. GIỚI THIỆU", 28),      // cấp 1
		bold("I. Thông tin chung", 24), // cấp 2
		{Text: "Nội dung thường", Format: Format{FontSize: 19}},
		bold("F2 – Vào chế độ theo dõi", 21), // cấp 3
		bold("2.1 Đặc tả", 24),               // cấp 2
		bold("1. Danh sách bệnh nhân", 0),    // không có cỡ chữ riêng
		{Text: "C. Trong bảng", InTable: true, Format: Format{Bold: true, FontSize: 28}},
		{Text: "B. Không đậm", Format: Format{FontSize: 28}},
		bold("III. QUY TẮC NGHIỆP VỤ (BUSINESS RULES)", 24), // cấp 2
	}
	require.Equal(t, []int{0, 1, 2, 0, 3, 2, 0, 0, 0, 2}, Levels(Styles{}, paragraphs))
}

// URD bôi đậm thủ công đã qua 1 lần tạo phiên bản: có đúng 1 tiêu đề theo
// style (mục "Phụ lục" do docxmerge thêm) — heuristic vẫn phải bổ sung đủ mục.
func TestLevels_ItTieuDeStyle_VanBoSungTuHeuristic(t *testing.T) {
	paragraphs := []Paragraph{
		{Text: "A. GIỚI THIỆU", Format: Format{Bold: true, FontSize: 28}},
		{Text: "I. Thông tin chung", Format: Format{Bold: true, FontSize: 24}},
		{StyleID: "Heading1", Text: "Phụ lục: Edge Case bổ sung (AI)"},
	}
	require.Equal(t, []int{1, 2, 1}, Levels(Styles{}, paragraphs))
}

func TestLevels_DuPhongBoQuaDoanQuaDai(t *testing.T) {
	long := "1. " + string(bytes.Repeat([]byte("a"), maxFallbackRunes))
	paragraphs := []Paragraph{{Text: long, Format: Format{Bold: true, FontSize: 28}}}
	require.Equal(t, []int{0}, Levels(Styles{}, paragraphs))
}

// trackParagraph chạy Tracker qua 1 đoạn <w:p> như cách parser/docxmerge dùng.
func trackParagraph(t *testing.T, paragraphXML string) Format {
	t.Helper()
	var tracker Tracker
	tracker.Reset()
	decoder := xml.NewDecoder(bytes.NewReader([]byte(paragraphXML)))
	inText := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return tracker.Format()
		}
		require.NoError(t, err)
		switch node := token.(type) {
		case xml.StartElement:
			inText = inText || node.Name.Local == "t"
			tracker.Start(node)
		case xml.CharData:
			if inText {
				tracker.Text(node)
			}
		case xml.EndElement:
			if node.Name.Local == "t" {
				inText = false
			}
			tracker.End(node)
		}
	}
}

func TestTracker(t *testing.T) {
	format := trackParagraph(t, `<w:p xmlns:w="w"><w:pPr><w:rPr><w:sz w:val="40"/></w:rPr></w:pPr>
<w:r><w:rPr><w:b/><w:sz w:val="28"/></w:rPr><w:t>A. </w:t></w:r>
<w:r><w:rPr><w:b w:val="1"/><w:sz w:val="24"/></w:rPr><w:t>GIỚI THIỆU</w:t></w:r>
<w:r><w:t xml:space="preserve"> </w:t></w:r></w:p>`)
	require.Equal(t, Format{Bold: true, FontSize: 28}, format,
		"rPr của dấu đoạn (trong pPr) và run chỉ có khoảng trắng không được tính")

	format = trackParagraph(t, `<w:p xmlns:w="w"><w:r><w:rPr><w:b/></w:rPr><w:t>Đậm</w:t></w:r>
<w:r><w:rPr><w:b w:val="0"/></w:rPr><w:t>không đậm</w:t></w:r></w:p>`)
	require.False(t, format.Bold)

	format = trackParagraph(t, `<w:p xmlns:w="w"><w:pPr><w:outlineLvl w:val="1"/></w:pPr><w:r><w:t>x</w:t></w:r></w:p>`)
	require.Equal(t, 2, format.OutlineLevel)
}
