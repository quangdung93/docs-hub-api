// Package assets nhúng tài nguyên tĩnh (template, font) mà module document
// cần lúc chạy vào binary — chỉ dùng nội bộ module này nên đặt dưới internal/
// thay vì một package top-level dùng chung.
package assets

import _ "embed"

// UATReportXLSX là template UAT Report chuẩn ISC (4.0-BM/PM/HDCV/FTEL, v2.0).
// Module document dùng file này làm khung khi xuất báo cáo UAT theo scope
// version/change-request/toàn dự án — xem internal/module/document/usecase.
//
//go:embed "4.0-BMPMHDCVFTEL-BM UAT Report_v2.0 final.xlsx"
var UATReportXLSX []byte

// PDFFontRegular và PDFFontBold là font DejaVu Sans nhúng sẵn để sinh UAT
// Report bản PDF — font PDF built-in (Helvetica/Arial) không có dấu tiếng
// Việt, DejaVu Sans thì có đầy đủ. Xem fonts/LICENSE.txt.
//
//go:embed "fonts/DejaVuSans.ttf"
var PDFFontRegular []byte

//go:embed "fonts/DejaVuSans-Bold.ttf"
var PDFFontBold []byte
