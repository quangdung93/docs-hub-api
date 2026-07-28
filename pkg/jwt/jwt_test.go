package jwt_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/quangdung393/docs-hub-api/pkg/jwt"
)

func newManager(t *testing.T) *jwt.Manager {
	t.Helper()
	m, err := jwt.NewManager(jwt.Config{Secret: "test-secret", Issuer: "test", AccessTTL: time.Hour})
	require.NoError(t, err)
	return m
}

func TestSignAndVerify_RoundTrip(t *testing.T) {
	m := newManager(t)
	now := time.Now()

	token, err := m.Sign("user-1", "a@b.com", []string{"admin"}, now)
	require.NoError(t, err)

	claims, err := m.Verify(token)
	require.NoError(t, err)
	require.Equal(t, "user-1", claims.UserID)
	require.Equal(t, "a@b.com", claims.Email)
	require.Equal(t, []string{"admin"}, claims.Roles)
}

func TestVerify_ExpiredToken(t *testing.T) {
	m := newManager(t)
	past := time.Now().Add(-2 * time.Hour)

	token, err := m.Sign("user-1", "a@b.com", nil, past)
	require.NoError(t, err)

	_, err = m.Verify(token)
	require.ErrorIs(t, err, jwt.ErrExpiredToken)
}

func TestVerify_TamperedToken(t *testing.T) {
	m := newManager(t)
	_, err := m.Verify("khong.phai.token")
	require.ErrorIs(t, err, jwt.ErrInvalidToken)
}

func TestNewManager_RejectsEmptySecret(t *testing.T) {
	_, err := jwt.NewManager(jwt.Config{Secret: ""})
	require.Error(t, err)
}
