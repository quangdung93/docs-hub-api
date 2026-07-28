package apperr

import "errors"

// AsBusiness kiểm tra chuỗi lỗi có chứa *BusinessError không.
// Trả về (lỗi, true) nếu có. Dùng trong middleware phân loại lỗi.
func AsBusiness(err error) (*BusinessError, bool) {
	var be *BusinessError
	if errors.As(err, &be) {
		return be, true
	}
	return nil, false
}

// AsTechnical kiểm tra chuỗi lỗi có chứa *TechnicalError không.
func AsTechnical(err error) (*TechnicalError, bool) {
	var te *TechnicalError
	if errors.As(err, &te) {
		return te, true
	}
	return nil, false
}

// IsBusiness trả về true nếu err là (hoặc bọc) một BusinessError.
func IsBusiness(err error) bool {
	_, ok := AsBusiness(err)
	return ok
}

// IsTechnical trả về true nếu err là (hoặc bọc) một TechnicalError.
func IsTechnical(err error) bool {
	_, ok := AsTechnical(err)
	return ok
}
