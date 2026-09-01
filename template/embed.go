// Package template nhúng các file mẫu (template) tài liệu chuẩn ISC vào binary
// để không phụ thuộc đường dẫn đĩa lúc chạy — xem ADR liên quan tới UAT Report.
package template

import _ "embed"

// UATReportXLSX là template UAT Report chuẩn ISC (4.0-BM/PM/HDCV/FTEL, v2.0).
// Module document dùng file này làm khung khi xuất báo cáo UAT theo scope
// version/change-request/toàn dự án — xem internal/module/document/usecase.
//
//go:embed "4.0-BMPMHDCVFTEL-BM UAT Report_v2.0 final.xlsx"
var UATReportXLSX []byte

// PDFFontRegular và PDFFontBold là font DejaVu Sans nhúng sẵn để sinh UAT
// Report bản PDF — font PDF built-in (Helvetica/Arial) không có dấu tiếng
// Việt, DejaVu Sans thì có đầy đủ. Xem template/fonts/LICENSE.txt.
//
//go:embed "fonts/DejaVuSans.ttf"
var PDFFontRegular []byte

//go:embed "fonts/DejaVuSans-Bold.ttf"
var PDFFontBold []byte
