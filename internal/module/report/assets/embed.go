// Package assets nhúng tài nguyên tĩnh (template, font) mà module report cần
// lúc chạy vào binary — chỉ dùng nội bộ module này nên đặt dưới internal/ thay
// vì một package top-level dùng chung.
package assets

import _ "embed"

// UATReportXLSX là template UAT Report chuẩn ISC (4.0-BM/PM/HDCV/FTEL, v2.0) —
// bản sao của internal/module/document/assets. Mỗi module tự chứa asset riêng
// để giữ vertical-slice độc lập; nội dung mỗi ô do RAGFlow sinh (xem usecase),
// khác cách module document điền từ danh sách document/revision thô.
//
//go:embed "4.0-BMPMHDCVFTEL-BM UAT Report_v2.0 final.xlsx"
var UATReportXLSX []byte

// ProjectPlanXLSX là template Project Plan chuẩn ISC (3.0-BM/PM/HDCV/FTEL, v3.0).
//
//go:embed "3.0-BMPMHDCVFTEL-BM Project Plan_v3.0 final.xlsx"
var ProjectPlanXLSX []byte

// TestcaseReportXLSX là template Testcase Report chuẩn ISC (SDLC).
//
//go:embed "ISC_Template_SDLC_TestCase_Report_Version.xlsx"
var TestcaseReportXLSX []byte

// PDFFontRegular và PDFFontBold là font DejaVu Sans nhúng sẵn để sinh report
// bản PDF — font PDF built-in (Helvetica/Arial) không có dấu tiếng Việt.
//
//go:embed "fonts/DejaVuSans.ttf"
var PDFFontRegular []byte

//go:embed "fonts/DejaVuSans-Bold.ttf"
var PDFFontBold []byte
