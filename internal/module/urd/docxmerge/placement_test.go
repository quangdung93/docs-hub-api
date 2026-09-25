package docxmerge

import (
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/module/urd/domain"
)

// structuredDocumentXML mô phỏng 1 URD thật: có tiêu đề nhiều cấp, mục AC
// dạng danh sách đánh số (numPr), mục BR, và 1 chương khác phía sau.
const structuredDocumentXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>
<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Chuc nang Dang nhap</w:t></w:r></w:p>
<w:p><w:pPr><w:pStyle w:val="Heading2"/></w:pPr><w:r><w:t>Tieu chi chap nhan</w:t></w:r></w:p>
<w:p><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="7"/></w:numPr></w:pPr>
<w:r><w:t>AC-01 Dang nhap dung thi vao trang chu</w:t></w:r></w:p>
<w:p><w:pPr><w:pStyle w:val="Heading2"/></w:pPr><w:r><w:t>Quy tac nghiep vu</w:t></w:r></w:p>
<w:p><w:r><w:t>BR-01 Mat khau toi thieu 8 ky tu</w:t></w:r></w:p>
<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Chuc nang Dang xuat</w:t></w:r></w:p>
<w:p><w:r><w:t>Noi dung dang xuat</w:t></w:r></w:p>
<w:sectPr><w:pgSz w:w="12240" w:h="15840"/></w:sectPr></w:body></w:document>`

func mergeStructured(t *testing.T, documentXML string, cases []domain.EdgeCase) string {
	t.Helper()
	original := buildDocx(t, map[string]string{"word/document.xml": documentXML})
	merged, err := Merge(original, cases)
	require.NoError(t, err)
	content := readEntry(t, merged, "word/document.xml")
	requireWellFormedXML(t, content)
	return content
}

// requireWellFormedXML canh điều quan trọng nhất: chèn xong file phải còn
// mở được. XML hỏng thì Word báo lỗi tài liệu, người dùng mất luôn phiên bản
// mới vừa sinh.
func requireWellFormedXML(t *testing.T, content string) {
	t.Helper()
	decoder := xml.NewDecoder(strings.NewReader(content))
	for {
		_, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return
		}
		require.NoError(t, err, "document.xml sau khi chèn phải là XML hợp lệ")
	}
}

func edgeCase(description, resolution, targetHeading string) domain.EdgeCase {
	return domain.EdgeCase{
		ID: uuid.New(), SequenceNo: 1, Description: description,
		Resolution: resolution, TargetHeading: targetHeading, IncludeInDocument: true,
	}
}

func TestMerge_ChenThangVaoCuoiMucAIChiDinh(t *testing.T) {
	content := mergeStructured(t, structuredDocumentXML, []domain.EdgeCase{
		edgeCase("Dang nhap sai qua 5 lan thi sao?", "Khoa tai khoan 15 phut", "Tieu chi chap nhan"),
	})

	require.Contains(t, content, "Khoa tai khoan 15 phut")
	// Không còn phụ lục: nội dung đã vào thẳng mục.
	require.NotContains(t, content, "Phụ lục: Edge Case bổ sung (AI)")
	// Nằm SAU nội dung sẵn có của mục và TRƯỚC tiêu đề mục kế tiếp.
	require.Greater(t, strings.Index(content, "Khoa tai khoan 15 phut"),
		strings.Index(content, "AC-01 Dang nhap dung thi vao trang chu"))
	require.Less(t, strings.Index(content, "Khoa tai khoan 15 phut"),
		strings.Index(content, "Quy tac nghiep vu"))
}

// Đoạn chèn phải kế thừa <w:pPr> của đoạn nội dung cuối trong mục — mục AC
// đánh số thì dòng bổ sung cũng phải nằm trong cùng danh sách đánh số đó,
// không rơi ra thành đoạn văn trơ giữa danh sách.
func TestMerge_KeThuaStyleDanhSoCuaMuc(t *testing.T) {
	content := mergeStructured(t, structuredDocumentXML, []domain.EdgeCase{
		edgeCase("Phien dang nhap het han?", "Bao het phien va quay lai man Dang nhap", "Tieu chi chap nhan"),
	})

	require.Contains(t, content,
		`<w:p><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="7"/></w:numPr></w:pPr>`+
			`<w:r><w:t xml:space="preserve">Phien dang nhap het han?`)
}

// Mục cuối tài liệu: không có tiêu đề kế tiếp nên chèn ngay trước <w:sectPr>
// — vẫn phải nằm trong mục đó, và tuyệt đối không được nằm sau sectPr.
func TestMerge_MucCuoiTaiLieu_ChenTruocSectPr(t *testing.T) {
	content := mergeStructured(t, structuredDocumentXML, []domain.EdgeCase{
		edgeCase("Dang xuat tren nhieu thiet bi?", "Dang xuat tat ca phien", "Chuc nang Dang xuat"),
	})

	require.Greater(t, strings.Index(content, "Dang xuat tat ca phien"),
		strings.Index(content, "Noi dung dang xuat"))
	require.Less(t, strings.Index(content, "Dang xuat tat ca phien"),
		strings.Index(content, "<w:sectPr"))
}

// mucCoMucConXML mô phỏng đúng dạng URD của team (đo trên ISC_MBX URD v1.0):
// mỗi chức năng là 1 mục H2, bên trong có các mục con H3 Workflow / Business
// rules / Wireframe, và mục con cuối kết thúc bằng 1 dòng chú thích ảnh CĂN
// GIỮA. AI chỉ vào tên chức năng (mục H2) vì đó là tiêu đề duy nhất — tên các
// mục con lặp lại ở mọi chức năng nên không khớp được.
const mucCoMucConXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>
<w:p><w:pPr><w:pStyle w:val="Heading2"/></w:pPr><w:r><w:t>Danh sach hop dong</w:t></w:r></w:p>
<w:p><w:r><w:t>Man hinh liet ke hop dong theo khach hang</w:t></w:r></w:p>
<w:p><w:pPr><w:pStyle w:val="Heading3"/></w:pPr><w:r><w:t>Workflow:</w:t></w:r></w:p>
<w:p><w:r><w:t>Nhan vien mo man hinh danh sach</w:t></w:r></w:p>
<w:p><w:pPr><w:pStyle w:val="Heading3"/></w:pPr><w:r><w:t>Business rules (BR):</w:t></w:r></w:p>
<w:p><w:r><w:t>BR-01 Moi trang toi da 50 dong</w:t></w:r></w:p>
<w:p><w:pPr><w:pStyle w:val="Heading3"/></w:pPr><w:r><w:t>Wireframe, Screen description:</w:t></w:r></w:p>
<w:p><w:pPr><w:jc w:val="center"/><w:ind w:left="720"/></w:pPr><w:r><w:t>Hinh 1: Man hinh danh sach</w:t></w:r></w:p>
<w:p><w:pPr><w:pStyle w:val="Heading2"/></w:pPr><w:r><w:t>Chi tiet hop dong</w:t></w:r></w:p>
<w:p><w:r><w:t>Noi dung chi tiet</w:t></w:r></w:p>
<w:sectPr/></w:body></w:document>`

// Mục AI chỉ định CÓ mục con: nội dung phải nằm ngay dưới tên mục đó, KHÔNG
// được trôi xuống cuối mục con cuối cùng. Bản đầu dừng theo cấp tiêu đề nên
// 15/15 dòng rơi vào "Wireframe, Screen description:" của URD Mobix.
func TestMerge_MucCoMucCon_ChenNgayDuoiTieuDeChuKhongTroiXuongMucConCuoi(t *testing.T) {
	content := mergeStructured(t, mucCoMucConXML, []domain.EdgeCase{
		edgeCase("Hop dong khong co hoa don thi sao?", "An nut thanh toan", "Danh sach hop dong"),
	})

	require.NotContains(t, content, "Phụ lục: Edge Case bổ sung (AI)")
	require.Greater(t, strings.Index(content, "An nut thanh toan"),
		strings.Index(content, "Man hinh liet ke hop dong theo khach hang"),
		"phải nằm sau nội dung sẵn có của mục")
	require.Less(t, strings.Index(content, "An nut thanh toan"),
		strings.Index(content, "Workflow:"),
		"KHÔNG được trôi qua mục con đầu tiên")
	require.Less(t, strings.Index(content, "An nut thanh toan"),
		strings.Index(content, "Wireframe, Screen description:"),
		"KHÔNG được rơi xuống mục con cuối cùng")
}

// Hệ quả thứ hai của lỗi cũ: đoạn chèn kế thừa <w:pPr> của đoạn cuối mục con
// Wireframe — vốn là chú thích ảnh căn giữa — nên quy tắc nghiệp vụ hiện ra
// giữa trang như chú thích ảnh (8/15 dòng trên URD Mobix).
func TestMerge_MucCoMucCon_KhongKeThuaDinhDangChuThichAnh(t *testing.T) {
	content := mergeStructured(t, mucCoMucConXML, []domain.EdgeCase{
		edgeCase("Loc theo khoang ngay qua dai?", "Gioi han 12 thang", "Danh sach hop dong"),
	})

	doan := doanChuaChuoi(t, content, "Gioi han 12 thang")
	require.NotContains(t, doan, `<w:jc w:val="center"/>`, "không được căn giữa như chú thích ảnh")
	require.NotContains(t, doan, `<w:ind w:left="720"/>`, "không được thụt lề theo chú thích ảnh")
}

// Tiêu đề cha đi liền tiêu đề con (mục không có nội dung trực tiếp): đoạn chèn
// thành dòng đầu của mục, nằm giữa hai tiêu đề, không kế thừa style của ai.
func TestMerge_MucKhongCoNoiDungTrucTiep_ChenGiuaHaiTieuDe(t *testing.T) {
	const lienTiep = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>
<w:p><w:pPr><w:pStyle w:val="Heading2"/></w:pPr><w:r><w:t>Gia han</w:t></w:r></w:p>
<w:p><w:pPr><w:pStyle w:val="Heading3"/></w:pPr><w:r><w:t>Workflow:</w:t></w:r></w:p>
<w:p><w:r><w:t>Nhan vien bam Gia han</w:t></w:r></w:p>
<w:sectPr/></w:body></w:document>`

	content := mergeStructured(t, lienTiep, []domain.EdgeCase{
		edgeCase("Goi API gia han bi loi mang?", "Bao loi va giu nguyen man hinh", "Gia han"),
	})

	require.Greater(t, strings.Index(content, "Bao loi va giu nguyen man hinh"),
		strings.Index(content, "Gia han"))
	require.Less(t, strings.Index(content, "Bao loi va giu nguyen man hinh"),
		strings.Index(content, "Workflow:"))
	require.Contains(t, content,
		`<w:p><w:r><w:t xml:space="preserve">Goi API gia han bi loi mang?`,
		"không có nội dung trực tiếp thì đoạn chèn không mang pPr nào")
}

// doanChuaChuoi trả về nguyên văn <w:p>…</w:p> chứa chuỗi cần tìm, để soi
// riêng phần định dạng của đúng đoạn đó.
func doanChuaChuoi(t *testing.T, content, canTim string) string {
	t.Helper()
	viTri := strings.Index(content, canTim)
	require.GreaterOrEqual(t, viTri, 0, "không tìm thấy %q trong document.xml", canTim)
	dau := strings.LastIndex(content[:viTri], "<w:p>")
	if moKemThuocTinh := strings.LastIndex(content[:viTri], "<w:p "); moKemThuocTinh > dau {
		dau = moKemThuocTinh
	}
	require.GreaterOrEqual(t, dau, 0, "không tìm thấy thẻ mở <w:p> của đoạn")
	cuoi := strings.Index(content[viTri:], "</w:p>")
	require.GreaterOrEqual(t, cuoi, 0, "không tìm thấy thẻ đóng </w:p> của đoạn")
	return content[dau : viTri+cuoi]
}

// AI copy tiêu đề từ bản text trích xuất (parser thêm tiền tố "#") — bỏ "#",
// khoảng trắng thừa và khác hoa/thường vẫn phải khớp đúng mục.
func TestMerge_KhopTieuDeDuCoTienToThangVaKhacHoaThuong(t *testing.T) {
	content := mergeStructured(t, structuredDocumentXML, []domain.EdgeCase{
		edgeCase("Mat khau qua ngan?", "Bao loi do dai toi thieu", "##   QUY TAC NGHIEP VU  "),
	})

	require.NotContains(t, content, "Phụ lục: Edge Case bổ sung (AI)")
	require.Greater(t, strings.Index(content, "Bao loi do dai toi thieu"),
		strings.Index(content, "BR-01 Mat khau toi thieu 8 ky tu"))
	require.Less(t, strings.Index(content, "Bao loi do dai toi thieu"),
		strings.Index(content, "Chuc nang Dang xuat"))
}

func TestMerge_KhongKhopTieuDeThiLuiVePhuLuc(t *testing.T) {
	content := mergeStructured(t, structuredDocumentXML, []domain.EdgeCase{
		edgeCase("Case khong ro thuoc muc nao", "Huong xu ly X", "Muc khong ton tai trong tai lieu"),
	})

	require.Contains(t, content, "Phụ lục: Edge Case bổ sung (AI)")
	require.Contains(t, content, "Case 1: Case khong ro thuoc muc nao")
	require.Contains(t, content, "Hướng giải quyết: Huong xu ly X")
}

func TestMerge_AIKhongXacDinhDuocMucThiLuiVePhuLuc(t *testing.T) {
	content := mergeStructured(t, structuredDocumentXML, []domain.EdgeCase{
		edgeCase("Case AI khong biet dat vao dau", "Huong xu ly Y", ""),
	})

	require.Contains(t, content, "Phụ lục: Edge Case bổ sung (AI)")
	require.Contains(t, content, "Huong xu ly Y")
}

// Tài liệu có 2 mục trùng tên: không đoán bừa mục nào, lùi về phụ lục — chèn
// nhầm mục là lỗi âm thầm, file vẫn mở được nên rất khó phát hiện.
func TestMerge_TieuDeTrungNhauThiLuiVePhuLuc(t *testing.T) {
	const trungTen = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>
<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Dang nhap</w:t></w:r></w:p>
<w:p><w:pPr><w:pStyle w:val="Heading2"/></w:pPr><w:r><w:t>Tieu chi chap nhan</w:t></w:r></w:p>
<w:p><w:r><w:t>AC dang nhap</w:t></w:r></w:p>
<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Dang ky</w:t></w:r></w:p>
<w:p><w:pPr><w:pStyle w:val="Heading2"/></w:pPr><w:r><w:t>Tieu chi chap nhan</w:t></w:r></w:p>
<w:p><w:r><w:t>AC dang ky</w:t></w:r></w:p>
<w:sectPr/></w:body></w:document>`

	content := mergeStructured(t, trungTen, []domain.EdgeCase{
		edgeCase("Case mo ho", "Huong xu ly Z", "Tieu chi chap nhan"),
	})

	require.Contains(t, content, "Phụ lục: Edge Case bổ sung (AI)",
		"tiêu đề trùng nhau thì không được đoán bừa")
}

// Mục có nội dung nằm trong bảng: đoạn mới phải nằm SAU </w:tbl>, không được
// rơi vào trong ô của bảng.
func TestMerge_KhongChenVaoBenTrongBang(t *testing.T) {
	const coBang = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>
<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Tieu chi chap nhan</w:t></w:r></w:p>
<w:tbl><w:tr><w:tc><w:p><w:r><w:t>AC-01 trong bang</w:t></w:r></w:p></w:tc></w:tr></w:tbl>
<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Muc ke tiep</w:t></w:r></w:p>
<w:sectPr/></w:body></w:document>`

	content := mergeStructured(t, coBang, []domain.EdgeCase{
		edgeCase("Case bang", "Huong xu ly bang", "Tieu chi chap nhan"),
	})

	require.Greater(t, strings.Index(content, "Huong xu ly bang"), strings.Index(content, "</w:tbl>"),
		"không được chèn vào trong ô của bảng")
	require.Less(t, strings.Index(content, "Huong xu ly bang"), strings.Index(content, "Muc ke tiep"),
		"vẫn phải nằm trong mục đã chỉ định")
}

// Nhiều case cùng 1 mục: giữ nguyên thứ tự; case không khớp mục nào vẫn
// xuống phụ lục — 2 cơ chế chạy song song trong cùng 1 lần merge.
func TestMerge_NhieuCaseVuaChenThangVuaVaoPhuLuc(t *testing.T) {
	content := mergeStructured(t, structuredDocumentXML, []domain.EdgeCase{
		edgeCase("Case mot", "Xu ly mot", "Tieu chi chap nhan"),
		edgeCase("Case hai", "Xu ly hai", "Tieu chi chap nhan"),
		edgeCase("Case ba", "Xu ly ba", "Muc khong ton tai"),
	})

	require.Less(t, strings.Index(content, "Xu ly mot"), strings.Index(content, "Xu ly hai"),
		"giữ đúng thứ tự case trong cùng 1 mục")
	require.Less(t, strings.Index(content, "Xu ly hai"), strings.Index(content, "Quy tac nghiep vu"),
		"cả 2 case phải nằm trong mục đã chỉ định")
	require.Contains(t, content, "Phụ lục: Edge Case bổ sung (AI)")
	require.Contains(t, content, "Case 1: Case ba", "phụ lục chỉ còn case không định vị được")
	require.NotContains(t, content, "Case 2: ")
}

// Tài liệu không dùng style Heading (tiêu đề bôi đậm thủ công): không nhận
// diện được mục nào, toàn bộ lùi về phụ lục — đúng hành vi cũ, không hỏng.
func TestMerge_TaiLieuKhongCoStyleHeading_VeHetPhuLuc(t *testing.T) {
	const khongHeading = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>
<w:p><w:pPr><w:b/></w:pPr><w:r><w:t>Tieu chi chap nhan</w:t></w:r></w:p>
<w:p><w:r><w:t>AC-01</w:t></w:r></w:p>
<w:sectPr/></w:body></w:document>`

	content := mergeStructured(t, khongHeading, []domain.EdgeCase{
		edgeCase("Case x", "Xu ly x", "Tieu chi chap nhan"),
	})

	require.Contains(t, content, "Phụ lục: Edge Case bổ sung (AI)")
	require.Contains(t, content, "Xu ly x")
}

func TestNormalizeHeading(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"bỏ tiền tố markdown", "## Tiêu chí chấp nhận", "tiêu chí chấp nhận"},
		{"gộp khoảng trắng thừa", "Tiêu   chí\tchấp nhận", "tiêu chí chấp nhận"},
		{"bỏ dấu câu cuối", "Tiêu chí chấp nhận:", "tiêu chí chấp nhận"},
		{"không phân biệt hoa thường", "TIÊU CHÍ CHẤP NHẬN", "tiêu chí chấp nhận"},
		{"rỗng", "   ", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, normalizeHeading(tc.in))
		})
	}
}

func TestHeadingLevel(t *testing.T) {
	require.Equal(t, 1, headingLevel("Heading1"))
	require.Equal(t, 3, headingLevel("heading3"))
	require.Equal(t, 0, headingLevel("Normal"))
	require.Equal(t, 0, headingLevel("Heading9"), "Word chỉ có Heading1..6")
	require.Equal(t, 0, headingLevel(""))
}
