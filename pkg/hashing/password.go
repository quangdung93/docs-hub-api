// Package hashing băm và kiểm tra mật khẩu bằng bcrypt.
package hashing

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// ErrMismatch được trả về khi mật khẩu không khớp hash.
var ErrMismatch = errors.New("mật khẩu không khớp")

// Hasher băm/verify mật khẩu với cost cấu hình sẵn.
type Hasher struct {
	cost int
}

// NewHasher tạo Hasher. cost <= 0 dùng mặc định của bcrypt (10).
func NewHasher(cost int) *Hasher {
	if cost <= 0 {
		cost = bcrypt.DefaultCost
	}
	return &Hasher{cost: cost}
}

// Hash băm mật khẩu thô thành chuỗi hash để lưu DB.
func (h *Hasher) Hash(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), h.cost)
	if err != nil {
		return "", fmt.Errorf("băm mật khẩu thất bại: %w", err)
	}
	return string(b), nil
}

// Compare kiểm tra mật khẩu thô có khớp hash không.
// Trả ErrMismatch nếu không khớp (không lộ chi tiết).
func (h *Hasher) Compare(hash, plain string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)); err != nil {
		return ErrMismatch
	}
	return nil
}
