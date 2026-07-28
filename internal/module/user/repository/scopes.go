package repository

import (
	"gorm.io/gorm"

	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	"github.com/quangdung93/docs-hub-api/internal/module/user/domain"
)

// sortableColumns là WHITELIST cột được phép sort. Đây là hàng rào chống SQL
// injection qua tham số sort_by: chỉ giá trị có trong map mới được ghép vào ORDER BY.
var sortableColumns = map[string]string{ //nolint:gochecknoglobals // bảng tra cứu bất biến
	"created_at": "created_at",
	"updated_at": "updated_at",
	"email":      "email",
	"full_name":  "full_name",
	"status":     "status",
}

// scopeFilter áp các điều kiện lọc.
func scopeFilter(f domain.Filter) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if f.Keyword != nil && *f.Keyword != "" {
			kw := "%" + *f.Keyword + "%"
			db = db.Where("(email LIKE ? OR full_name LIKE ?)", kw, kw)
		}
		if f.Status != nil {
			db = db.Where("status = ?", string(*f.Status))
		}
		if f.CreatedFrom != nil {
			db = db.Where("created_at >= ?", *f.CreatedFrom)
		}
		if f.CreatedTo != nil {
			db = db.Where("created_at <= ?", *f.CreatedTo)
		}
		return db
	}
}

// scopeSort áp ORDER BY an toàn (cột từ whitelist, order chỉ asc/desc).
func scopeSort(p pagination.Query) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		col, ok := sortableColumns[p.SortBy]
		if !ok {
			col = "created_at" // mặc định an toàn
		}
		order := "DESC"
		if p.Order == "asc" {
			order = "ASC"
		}
		return db.Order(col + " " + order)
	}
}

// scopePaginate áp LIMIT/OFFSET.
func scopePaginate(p pagination.Query) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Limit(p.Limit).Offset(p.Offset())
	}
}
