// Package ptr chứa tiện ích generic thao tác con trỏ, dùng chung toàn repo.
package ptr

// Of trả về con trỏ tới v. Hữu ích khi cần *T cho literal.
func Of[T any](v T) *T {
	return &v
}

// Deref trả về giá trị mà p trỏ tới, hoặc zero-value nếu p == nil.
func Deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// DerefOr trả về *p nếu p != nil, ngược lại trả về fallback.
func DerefOr[T any](p *T, fallback T) T {
	if p == nil {
		return fallback
	}
	return *p
}
