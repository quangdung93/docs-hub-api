// Package validatorx bọc go-playground/validator: cấu hình để tên field trong
// thông báo lỗi dùng tag `json` (snake_case) thay vì tên field Go, và chuyển lỗi
// thành map details phù hợp với envelope ISC.
package validatorx

import (
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

// Validator bọc *validator.Validate. Được inject qua constructor (không global).
type Validator struct {
	v *validator.Validate
}

// New tạo Validator đã cấu hình dùng tag json cho tên field.
func New() *Validator {
	v := validator.New(validator.WithRequiredStructEnabled())
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" || name == "" {
			return fld.Name
		}
		return name
	})
	return &Validator{v: v}
}

// Struct validate một struct. Trả nil nếu hợp lệ.
func (val *Validator) Struct(s any) error {
	return val.v.Struct(s)
}

// ToDetails chuyển lỗi validate thành map[field]thông_điệp để đưa vào
// error.details của envelope. Nếu err không phải ValidationErrors, trả nil.
func ToDetails(err error) map[string]string {
	var ve validator.ValidationErrors
	if !asValidationErrors(err, &ve) {
		return nil
	}
	details := make(map[string]string, len(ve))
	for _, fe := range ve {
		details[fe.Field()] = messageFor(fe)
	}
	return details
}

// messageFor sinh thông điệp tiếng Việt theo loại rule vi phạm.
func messageFor(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "Trường bắt buộc"
	case "email":
		return "Email không hợp lệ"
	case "min":
		return "Giá trị/độ dài tối thiểu là " + fe.Param()
	case "max":
		return "Giá trị/độ dài tối đa là " + fe.Param()
	case "oneof":
		return "Giá trị phải thuộc: " + fe.Param()
	case "uuid", "uuid4":
		return "Định dạng UUID không hợp lệ"
	default:
		return "Không hợp lệ (" + fe.Tag() + ")"
	}
}
