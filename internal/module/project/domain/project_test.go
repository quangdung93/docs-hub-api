package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/module/project/domain"
)

func TestNewProject_DefaultsToActiveStatus(t *testing.T) {
	ownerID := uuid.New()
	p := domain.NewProject(ownerID, "RAG Demo", "mô tả", domain.ProjectSettings{Model: "gpt-4o-mini", TopK: 5})

	require.Equal(t, domain.DefaultProjectStatus, p.Status)
	require.Equal(t, ownerID, p.OwnerID)
	require.True(t, p.IsOwner(ownerID))
	require.NotEqual(t, uuid.Nil, p.ID)
}

func TestProject_ChangeProfile_PartialUpdate(t *testing.T) {
	p := domain.NewProject(uuid.New(), "Old", "old desc", domain.ProjectSettings{TopK: 1})

	newName := "New"
	p.ChangeProfile(&newName, nil, nil)

	require.Equal(t, "New", p.Name)
	require.Equal(t, "old desc", p.Description, "description=nil phải giữ nguyên")
	require.Equal(t, 1, p.Settings.TopK, "settings=nil phải giữ nguyên")
}

func TestProject_ChangeProfile_UpdatesSettings(t *testing.T) {
	p := domain.NewProject(uuid.New(), "P", "", domain.ProjectSettings{TopK: 1})

	newSettings := domain.ProjectSettings{Model: "gpt-4o", TopK: 10, ChunkSize: 800, AllowedFormats: []string{"pdf"}}
	p.ChangeProfile(nil, nil, &newSettings)

	require.Equal(t, newSettings, p.Settings)
	require.Equal(t, "P", p.Name, "name=nil phải giữ nguyên")
}

func TestProjectSettings_ValueScan_RoundTrip(t *testing.T) {
	s := domain.ProjectSettings{Model: "gpt-4o-mini", TopK: 5, ChunkSize: 500, AllowedFormats: []string{"pdf", "docx"}}

	raw, err := s.Value()
	require.NoError(t, err)

	var got domain.ProjectSettings
	require.NoError(t, got.Scan(raw))
	require.Equal(t, s, got)
}

func TestProjectSettings_Scan_Nil(t *testing.T) {
	var s domain.ProjectSettings
	require.NoError(t, s.Scan(nil))
	require.Equal(t, domain.ProjectSettings{}, s)
}

func TestProjectSettings_Scan_UnsupportedType(t *testing.T) {
	var s domain.ProjectSettings
	require.Error(t, s.Scan(123))
}

func TestRole_Valid(t *testing.T) {
	require.True(t, domain.RoleOwner.Valid())
	require.True(t, domain.RoleEditor.Valid())
	require.True(t, domain.RoleViewer.Valid())
	require.False(t, domain.Role("admin").Valid())
}

func TestNewInvite_DefaultsToPending(t *testing.T) {
	projectID, userID := uuid.New(), uuid.New()
	m := domain.NewInvite(projectID, userID, domain.RoleEditor)

	require.Equal(t, domain.MemberStatusPending, m.Status)
	require.Nil(t, m.JoinedAt)
	require.Equal(t, domain.RoleEditor, m.Role)
}

func TestProjectMember_Accept_HappyPath(t *testing.T) {
	m := domain.NewInvite(uuid.New(), uuid.New(), domain.RoleViewer)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	require.NoError(t, m.Accept(now))
	require.Equal(t, domain.MemberStatusActive, m.Status)
	require.NotNil(t, m.JoinedAt)
	require.True(t, now.Equal(*m.JoinedAt))
}

func TestProjectMember_Accept_AlreadyActive_IsBusinessError(t *testing.T) {
	m := domain.NewInvite(uuid.New(), uuid.New(), domain.RoleViewer)
	require.NoError(t, m.Accept(time.Now()))

	err := m.Accept(time.Now())
	require.ErrorIs(t, err, domain.ErrInviteNotPending)
}
