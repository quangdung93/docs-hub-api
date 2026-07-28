package validatorx

import (
	"errors"

	"github.com/go-playground/validator/v10"
)

// asValidationErrors tách riêng để errorlint không cảnh báo type-assert trực tiếp.
func asValidationErrors(err error, target *validator.ValidationErrors) bool {
	return errors.As(err, target)
}
