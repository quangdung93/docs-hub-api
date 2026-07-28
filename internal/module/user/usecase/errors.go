package usecase

import "errors"

// isRepoErr kiểm tra err có khớp sentinel của repository không (dùng errors.Is
// để xuyên qua lỗi bọc).
func isRepoErr(err, target error) bool {
	return errors.Is(err, target)
}
