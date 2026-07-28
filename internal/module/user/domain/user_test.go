package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"document-hub-api/internal/module/user/domain"
)

func TestNewUser_DefaultsToActiveVersion1(t *testing.T) {
	u := domain.NewUser("a@b.com", "Nguyễn Văn A", "hash", []string{"user"})
	require.Equal(t, domain.StatusActive, u.Status)
	require.Equal(t, 1, u.Version)
	require.NotEqual(t, "00000000-0000-0000-0000-000000000000", u.ID.String())
}

func TestUser_LockUnlock(t *testing.T) {
	u := domain.NewUser("a@b.com", "A", "h", nil)

	u.LockAccount()
	require.Equal(t, domain.StatusLocked, u.Status)
	require.ErrorIs(t, u.CanLogin(), domain.ErrUserLocked)

	u.UnlockAccount()
	require.Equal(t, domain.StatusActive, u.Status)
	require.NoError(t, u.CanLogin())
}

func TestUser_ChangeProfile_BlockedWhenLocked(t *testing.T) {
	u := domain.NewUser("a@b.com", "A", "h", nil)
	u.LockAccount()

	err := u.ChangeProfile("Tên mới")
	require.ErrorIs(t, err, domain.ErrUserLocked)
	require.Equal(t, "A", u.FullName, "không được đổi khi bị khóa")
}

func TestUser_SetStatus_RejectsInvalid(t *testing.T) {
	u := domain.NewUser("a@b.com", "A", "h", nil)
	require.ErrorIs(t, u.SetStatus("banana"), domain.ErrInvalidStatus)
	require.NoError(t, u.SetStatus(domain.StatusInactive))
	require.Equal(t, domain.StatusInactive, u.Status)
}

func TestStatus_Valid(t *testing.T) {
	require.True(t, domain.StatusActive.Valid())
	require.False(t, domain.Status("xxx").Valid())
}
