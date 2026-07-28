package domain

// Các hành vi nghiệp vụ đặt ngay trên entity (rich domain model) — logic về
// trạng thái/quy tắc sống ở đây, không rải ra usecase hay handler.

// LockAccount chuyển tài khoản sang trạng thái khóa.
//
// LƯU Ý: cố tình KHÔNG đặt tên Lock()/Unlock() — cặp method đó khiến User thỏa
// interface sync.Locker, làm `go vet` (copylocks) hiểu nhầm mỗi lần copy User là
// copy một mutex.
func (u *User) LockAccount() {
	u.Status = StatusLocked
}

// UnlockAccount mở khóa tài khoản (về Active).
func (u *User) UnlockAccount() {
	u.Status = StatusActive
}

// CanLogin trả lỗi nghiệp vụ nếu tài khoản không được phép đăng nhập.
func (u *User) CanLogin() error {
	if u.Status == StatusLocked {
		return ErrUserLocked
	}
	return nil
}

// ChangeProfile cập nhật thông tin hồ sơ. Chặn thao tác khi bị khóa.
func (u *User) ChangeProfile(fullName string) error {
	if u.Status == StatusLocked {
		return ErrUserLocked
	}
	u.FullName = fullName
	return nil
}

// SetStatus đổi trạng thái với kiểm tra hợp lệ.
func (u *User) SetStatus(s Status) error {
	if !s.Valid() {
		return ErrInvalidStatus
	}
	u.Status = s
	return nil
}
