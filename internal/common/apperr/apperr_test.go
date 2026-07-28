package apperr_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"document-hub-api/internal/common/apperr"
	"document-hub-api/internal/common/errcode"
)

func TestBusinessError_WithDetailsDoesNotMutateSentinel(t *testing.T) {
	sentinel := apperr.NewBusiness(errcode.DuplicateEmail, "Email đã tồn tại", false)

	withDetails := sentinel.WithDetails(map[string]string{"field": "email"})

	require.Nil(t, sentinel.Details, "sentinel gốc phải không bị thay đổi")
	require.NotNil(t, withDetails.Details)
	require.Equal(t, sentinel.Code, withDetails.Code)
}

func TestAsBusiness_UnwrapsWrappedError(t *testing.T) {
	base := apperr.NewBusiness(errcode.UserLocked, "Tài khoản bị khóa", false)
	wrapped := fmt.Errorf("usecase lock: %w", base)

	be, ok := apperr.AsBusiness(wrapped)
	require.True(t, ok)
	require.Equal(t, errcode.UserLocked, be.Code)
	require.False(t, apperr.IsTechnical(wrapped))
}

func TestTechnicalError_HTTPStatusInferred(t *testing.T) {
	require.Equal(t, http.StatusBadRequest, apperr.BadRequest("x").HTTPStatus)
	require.Equal(t, http.StatusUnauthorized, apperr.Unauthorized("x").HTTPStatus)
	require.Equal(t, http.StatusForbidden, apperr.Forbidden("x").HTTPStatus)
	require.Equal(t, http.StatusNotFound, apperr.NotFound(errcode.UserNotFound, "x").HTTPStatus)
	require.Equal(t, http.StatusInternalServerError, apperr.Internal("x").HTTPStatus)
	require.Equal(t, http.StatusServiceUnavailable, apperr.DatabaseUnavailable("x").HTTPStatus)
}

func TestWithCause_PreservesChain(t *testing.T) {
	root := errors.New("gốc")
	te := apperr.Internal("lỗi hệ thống").WithCause(root)
	require.ErrorIs(t, te, root)
}
