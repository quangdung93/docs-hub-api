package domain

import "time"

// Filter là tiêu chí lọc danh sách user. Con trỏ = "không lọc theo tiêu chí này".
type Filter struct {
	Keyword     *string // tìm gần đúng theo email/full_name
	Status      *Status
	CreatedFrom *time.Time
	CreatedTo   *time.Time
}
