package domain

// Status là trạng thái tài khoản người dùng.
type Status string

const (
	StatusActive   Status = "active"
	StatusInactive Status = "inactive"
	StatusLocked   Status = "locked"
)

// Valid kiểm tra status có thuộc tập hợp lệ không.
func (s Status) Valid() bool {
	switch s {
	case StatusActive, StatusInactive, StatusLocked:
		return true
	default:
		return false
	}
}
