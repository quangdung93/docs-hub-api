package ingestion

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParserRegistry_TextVaMarkdown(t *testing.T) {
	tests := []struct {
		mediaType string
		version   string
	}{
		{"text/plain", "text-v1"},
		{"text/markdown", "markdown-v1"},
	}
	for _, tt := range tests {
		t.Run(tt.mediaType, func(t *testing.T) {
			parsed, err := NewParserRegistry().Parse(context.Background(), tt.mediaType, strings.NewReader("dòng 1\r\ndòng 2"))
			require.NoError(t, err)
			require.Equal(t, "dòng 1\ndòng 2", parsed.Text)
			require.Equal(t, tt.version, parsed.ParserVersion)
		})
	}
}

func TestParserRegistry_TuChoiDuLieuKhongHopLe(t *testing.T) {
	tests := []struct {
		name, mediaType, content string
	}{
		{"mime chưa hỗ trợ", "application/msword", "doc"},
		{"nội dung rỗng", "text/plain", " \n\t"},
		{"không phải UTF-8", "text/plain", string([]byte{0xff, 0xfe})},
		{"chứa ký tự null", "text/plain", "abc\x00def"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewParserRegistry().Parse(context.Background(), tt.mediaType, strings.NewReader(tt.content))
			require.Error(t, err)
		})
	}
}

func TestParserRegistry_CSV(t *testing.T) {
	parsed, err := NewParserRegistry().Parse(context.Background(), "text/csv", strings.NewReader("name,note\nAn,\"xin\nchào\""))
	require.NoError(t, err)
	require.Equal(t, "Row 1\tA=name\tB=note\nRow 2\tA=An\tB=xin chào\n", parsed.Text)
	require.Equal(t, "csv-v1", parsed.ParserVersion)
}

func TestParserRegistry_DOCX(t *testing.T) {
	docx := zipFixture(t, map[string]string{
		"word/document.xml": `<w:document xmlns:w="urn:w"><w:body>
			<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Giới thiệu</w:t></w:r></w:p>
			<w:p><w:r><w:t>Nội dung </w:t></w:r><w:r><w:t>DOCX</w:t></w:r></w:p>
		</w:body></w:document>`,
	})
	parsed, err := NewParserRegistry().Parse(context.Background(),
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document", bytes.NewReader(docx))
	require.NoError(t, err)
	require.Equal(t, "# Giới thiệu\nNội dung DOCX\n", parsed.Text)
	require.Equal(t, "docx-v1", parsed.ParserVersion)
}

func TestParserRegistry_XLSX(t *testing.T) {
	xlsx := zipFixture(t, map[string]string{
		"xl/workbook.xml":            `<workbook xmlns:r="urn:r"><sheets><sheet name="Chi phí" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/sharedStrings.xml":       `<sst><si><t>Hạng mục</t></si><si><t>Máy chủ</t></si></sst>`,
		"xl/worksheets/sheet1.xml": `<worksheet><sheetData>
			<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="inlineStr"><is><t>Số tiền</t></is></c></row>
			<row r="2"><c r="A2" t="s"><v>1</v></c><c r="B2"><v>1200</v></c></row>
		</sheetData></worksheet>`,
	})
	parsed, err := NewParserRegistry().Parse(context.Background(),
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", bytes.NewReader(xlsx))
	require.NoError(t, err)
	require.Equal(t, "# Sheet: Chi phí\nRow 1\tA=Hạng mục\tB=Số tiền\nRow 2\tA=Máy chủ\tB=1200\n", parsed.Text)
	require.Equal(t, "xlsx-v1", parsed.ParserVersion)
}

func TestCanonicalPDFText_GiuRanhGioiTrang(t *testing.T) {
	text, err := canonicalPDFText("Trang một\fTrang hai\f")
	require.NoError(t, err)
	require.Equal(t, "# PDF page 1\nTrang một\n\n# PDF page 2\nTrang hai", text)

	_, err = canonicalPDFText(" \f\n")
	require.ErrorContains(t, err, "OCR")
}

func TestParserRegistry_PDFTextLayer(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("máy test chưa cài pdftotext")
	}
	parsed, err := NewParserRegistry().Parse(context.Background(), "application/pdf", bytes.NewReader(minimalPDF("Hello PDF")))
	require.NoError(t, err)
	require.Contains(t, parsed.Text, "# PDF page 1")
	require.Contains(t, parsed.Text, "Hello PDF")
	require.Equal(t, "pdftotext-v1", parsed.ParserVersion)
}

func minimalPDF(text string) []byte {
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	stream := fmt.Sprintf("BT /F1 12 Tf 72 720 Td (%s) Tj ET", text)
	objects = append(objects, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for index, object := range objects {
		offsets[index+1] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for index := 1; index < len(offsets); index++ {
		fmt.Fprintf(&out, "%010d 00000 n \n", offsets[index])
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return out.Bytes()
}

func zipFixture(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var data bytes.Buffer
	w := zip.NewWriter(&data)
	for name, content := range entries {
		entry, err := w.Create(name)
		require.NoError(t, err)
		_, err = entry.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return data.Bytes()
}
