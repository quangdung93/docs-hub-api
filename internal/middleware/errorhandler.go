// Package middleware chứa các middleware HTTP cross-cutting.
//
// Đây (cùng với delivery/http) là nơi DUY NHẤT được import gin.
package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"document-hub-api/internal/common/apperr"
	"document-hub-api/internal/common/errcode"
	"document-hub-api/internal/common/response"
	"document-hub-api/pkg/logger"
)

// ErrorHandler là TRÁI TIM của việc chuẩn hóa lỗi theo ISC.
//
// Đây là điểm DUY NHẤT phân loại lỗi -> HTTP status:
//   - BusinessError  -> HTTP 200 + success=false (KHÔNG BAO GIỜ thành 4xx/5xx)
//   - TechnicalError -> HTTP theo mã (4xx/5xx)
//   - context deadline -> REQ_TIMEOUT (408)
//   - còn lại (panic đã recover, lỗi lạ) -> SYS_500, message generic
//
// Handler ở tầng delivery chỉ cần `c.Error(err); return` — không tự phân loại,
// không tự ghi JSON. Nhờ vậy handler luôn "mỏng".
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// Không có lỗi nào được đẩy vào -> handler đã tự ghi response thành công.
		if len(c.Errors) == 0 {
			return
		}
		// Nếu response đã được ghi (ví dụ handler đã trả OK rồi mới lỗi), không ghi đè.
		if c.Writer.Written() {
			return
		}

		err := c.Errors.Last().Err
		log := logger.FromContext(c.Request.Context())

		writeErrorResponse(c, log, err)
	}
}

// writeErrorResponse phân loại err và ghi envelope tương ứng.
func writeErrorResponse(c *gin.Context, log *zap.Logger, err error) {
	// 1) Lỗi nghiệp vụ -> HTTP 200, log WARN.
	if be, ok := apperr.AsBusiness(err); ok {
		log.Warn("lỗi nghiệp vụ",
			zap.String("code", be.Code),
			zap.Error(err),
		)
		response.Error(c, http.StatusOK, response.ErrorBody{
			Code:      be.Code,
			Message:   be.Message,
			Details:   be.Details,
			Retryable: be.Retryable,
		})
		return
	}

	// 2) Lỗi kỹ thuật -> HTTP theo mã.
	if te, ok := apperr.AsTechnical(err); ok {
		logByStatus(log, te.HTTPStatus, "lỗi kỹ thuật", err, zap.String("code", te.Code))
		response.Error(c, te.HTTPStatus, response.ErrorBody{
			Code:      te.Code,
			Message:   te.Message,
			Details:   te.Details,
			Retryable: te.Retryable,
		})
		return
	}

	// 3) Timeout do context -> REQ_TIMEOUT (408).
	if errors.Is(err, context.DeadlineExceeded) {
		log.Warn("request timeout", zap.Error(err))
		response.Error(c, http.StatusRequestTimeout, response.ErrorBody{
			Code:      errcode.ReqTimeout,
			Message:   "Yêu cầu xử lý quá thời gian cho phép",
			Retryable: true,
		})
		return
	}

	// 4) Lỗi không phân loại được -> SYS_500, message generic (không lộ nội bộ).
	log.Error("lỗi hệ thống không xác định", zap.Error(err))
	response.Error(c, http.StatusInternalServerError, response.ErrorBody{
		Code:      errcode.Sys500,
		Message:   "Đã có lỗi hệ thống. Vui lòng thử lại sau.",
		Retryable: false,
	})
}

// logByStatus chọn level log theo HTTP status: 5xx là ERROR, còn lại WARN.
func logByStatus(log *zap.Logger, status int, msg string, err error, fields ...zap.Field) {
	fields = append(fields, zap.Error(err), zap.Int("http_status", status))
	if status >= http.StatusInternalServerError {
		log.Error(msg, fields...)
		return
	}
	log.Warn(msg, fields...)
}
