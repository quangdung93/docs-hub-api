package usecase

import (
	"fmt"

	"github.com/xuri/excelize/v2"
)

// Ba template báo cáo chuẩn ISC đều mang sẵn dữ liệu của một dự án mẫu: danh
// sách rủi ro ví dụ, bug giả IP-101..IP-113, ma trận yêu cầu REQ-01..REQ-05…
// Phần nào code không ghi đè thì lọt nguyên vào file bàn giao, nên mỗi loại báo
// cáo phải tự dọn trước khi điền nội dung thật.
//
// CHỈ dọn nội dung của dự án mẫu. Nhãn biểu mẫu phải giữ nguyên vì đó là khung
// của biểu mẫu, không phải rác: tiêu đề cột, bảng chú giải (Risk!B9:D12 ánh xạ
// mức ưu tiên sang cách ứng phó), bộ chỉ số chuẩn ISC (PCV/CSAT/SPI/CPI), nhãn
// nhóm cột theo round ("ISC FEEDBACK").

// clearSampleCells xoá nội dung mẫu ở các ô chỉ định của một sheet.
//
// Ô đang giữ công thức được BỎ QUA. Template gom số liệu giữa các sheet bằng
// COUNTIF/COUNTIFS, ghi đè sẽ phá liên kết đó — ví dụ Risk!H16:H22 (TOTAL SCORE
// = LS*IS) hay Bug Data!O14 (suy Bug Status từ trạng thái Jira).
//
// Ô được ghi chuỗi rỗng chứ không gỡ hẳn khỏi file. Cách này an toàn vì mọi
// công thức thống kê trong ba template đều đếm bằng COUNTIF(...,"?*") — đòi hỏi
// ít nhất một ký tự — hoặc khớp giá trị chính xác ("Open", "Pass", 20…), nên
// chuỗi rỗng không lọt vào nhóm nào.
func clearSampleCells(f *excelize.File, sheet string, cells ...string) error {
	for _, cell := range cells {
		formula, err := f.GetCellFormula(sheet, cell)
		if err != nil {
			return fmt.Errorf("đọc công thức %s!%s: %w", sheet, cell, err)
		}
		if formula != "" {
			continue
		}
		if err := f.SetCellValue(sheet, cell, ""); err != nil {
			return fmt.Errorf("dọn dữ liệu mẫu %s!%s: %w", sheet, cell, err)
		}
	}
	return nil
}

// clearSampleBlock xoá vùng chữ nhật [firstCol..lastCol] × [firstRow..lastRow],
// dùng cho các bảng dữ liệu mẫu trải nhiều cột như Bug Data hay Risk.
func clearSampleBlock(f *excelize.File, sheet, firstCol, lastCol string, firstRow, lastRow int) error {
	from, err := excelize.ColumnNameToNumber(firstCol)
	if err != nil {
		return fmt.Errorf("cột %s của sheet %s: %w", firstCol, sheet, err)
	}
	to, err := excelize.ColumnNameToNumber(lastCol)
	if err != nil {
		return fmt.Errorf("cột %s của sheet %s: %w", lastCol, sheet, err)
	}
	for row := firstRow; row <= lastRow; row++ {
		for number := from; number <= to; number++ {
			col, err := excelize.ColumnNumberToName(number)
			if err != nil {
				return fmt.Errorf("cột số %d của sheet %s: %w", number, sheet, err)
			}
			if err := clearSampleCells(f, sheet, fmt.Sprintf("%s%d", col, row)); err != nil {
				return err
			}
		}
	}
	return nil
}
