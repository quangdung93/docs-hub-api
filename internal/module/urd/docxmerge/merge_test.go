package docxmerge

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/module/urd/domain"
)

func buildDocx(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func readEntry(t *testing.T, data []byte, name string) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	f, err := zr.Open(name)
	require.NoError(t, err)
	content, err := io.ReadAll(f)
	require.NoError(t, err)
	return string(content)
}

const sampleDocumentXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body><w:p><w:r><w:t>Noi dung goc</w:t></w:r></w:p>
<w:sectPr><w:pgSz w:w="12240" w:h="15840"/></w:sectPr></w:body></w:document>`

func TestMerge_ChenNoiDungTruocSectPr(t *testing.T) {
	original := buildDocx(t, map[string]string{
		"word/document.xml":   sampleDocumentXML,
		"[Content_Types].xml": "<Types/>",
	})
	cases := []domain.EdgeCase{
		{ID: uuid.New(), SequenceNo: 1, Description: "Case mo ta", Resolution: "Huong giai quyet"},
	}

	merged, err := Merge(original, cases)

	require.NoError(t, err)
	content := readEntry(t, merged, "word/document.xml")
	require.Contains(t, content, "Phụ lục: Edge Case bổ sung (AI)")
	require.Contains(t, content, "Case mo ta")
	require.Contains(t, content, "Huong giai quyet")
	// Nội dung mới phải nằm TRƯỚC <w:sectPr>, không phải sau nó.
	caseIdx := bytes.Index([]byte(content), []byte("Case mo ta"))
	sectPrIdx := bytes.Index([]byte(content), []byte("<w:sectPr"))
	require.Less(t, caseIdx, sectPrIdx)
	// Entry khác được giữ nguyên.
	require.Equal(t, "<Types/>", readEntry(t, merged, "[Content_Types].xml"))
}

func TestMerge_KemAnhChiGhiChuTenObjectKey(t *testing.T) {
	original := buildDocx(t, map[string]string{"word/document.xml": sampleDocumentXML})
	cases := []domain.EdgeCase{
		{ID: uuid.New(), SequenceNo: 1, Description: "Case co anh", Resolution: "Xem hinh",
			ImageObjectKey: "urd/analysis-1/case-1/screenshot.png"},
	}

	merged, err := Merge(original, cases)

	require.NoError(t, err)
	content := readEntry(t, merged, "word/document.xml")
	require.Contains(t, content, "urd/analysis-1/case-1/screenshot.png")
}

func TestMerge_KhongCoDocumentXML(t *testing.T) {
	original := buildDocx(t, map[string]string{"[Content_Types].xml": "<Types/>"})

	_, err := Merge(original, nil)

	require.ErrorIs(t, err, ErrNoDocumentXML)
}

func TestMerge_CauTrucKhongDuocHoTro(t *testing.T) {
	// Thiếu cả <w:sectPr> lẫn thẻ đóng </w:body> — không còn điểm neo nào để
	// chèn nội dung.
	original := buildDocx(t, map[string]string{
		"word/document.xml": `<w:document><w:body><w:p>khong co diem neo</w:p></w:document>`,
	})

	_, err := Merge(original, []domain.EdgeCase{{Description: "x"}})

	require.ErrorIs(t, err, ErrUnsupportedStructure)
}

func TestMerge_FileKhongPhaiZip(t *testing.T) {
	_, err := Merge([]byte("not a zip"), nil)

	require.Error(t, err)
}
