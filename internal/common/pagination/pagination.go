// Package pagination chuẩn hóa tham số phân trang đầu vào và metadata đầu ra
// theo đúng cấu trúc ISC (templates/03).
package pagination

import "math"

const (
	defaultPage  = 1
	defaultLimit = 20
	maxLimit     = 100
)

// Query là tham số phân trang lấy từ query string (snake_case theo templates/02).
type Query struct {
	Page   int    `form:"page"`
	Limit  int    `form:"limit"`
	SortBy string `form:"sort_by"`
	Order  string `form:"order"`
}

// Normalize áp giá trị mặc định và chặn giá trị nguy hiểm.
// LƯU Ý: whitelist cột sort được thực thi ở tầng repository (scopes), không ở đây —
// package này không biết về schema.
func (q Query) Normalize() Query {
	out := q
	if out.Page < 1 {
		out.Page = defaultPage
	}
	if out.Limit < 1 {
		out.Limit = defaultLimit
	}
	if out.Limit > maxLimit {
		out.Limit = maxLimit
	}
	if out.Order != "asc" && out.Order != "desc" {
		out.Order = "desc"
	}
	return out
}

// Offset tính offset cho câu SQL LIMIT/OFFSET.
func (q Query) Offset() int {
	return (q.Page - 1) * q.Limit
}

// Meta là metadata phân trang trả về client. Đúng 6 field theo templates/03.
type Meta struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	TotalItems int64 `json:"total_items"`
	TotalPages int64 `json:"total_pages"`
	HasNext    bool  `json:"has_next"`
	HasPrev    bool  `json:"has_prev"`
}

// NewMeta dựng metadata từ tham số đã normalize và tổng số bản ghi.
func NewMeta(page, limit int, totalItems int64) Meta {
	totalPages := int64(0)
	if limit > 0 && totalItems > 0 {
		totalPages = int64(math.Ceil(float64(totalItems) / float64(limit)))
	}
	return Meta{
		Page:       page,
		Limit:      limit,
		TotalItems: totalItems,
		TotalPages: totalPages,
		HasNext:    int64(page) < totalPages,
		HasPrev:    page > 1,
	}
}
