package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/common/errcode"
	"github.com/quangdung93/docs-hub-api/internal/module/project/domain"
	domainmocks "github.com/quangdung93/docs-hub-api/internal/module/project/domain/mocks"
)

// newProjectAuthContext dựng gin.Context giả cho path param "id" = projectID,
// actor=nil mô phỏng request CHƯA qua Auth.
func newProjectAuthContext(t *testing.T, actor *contextx.Actor, projectID uuid.UUID) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodDelete, "/internal/api/v1/projects/"+projectID.String(), nil)
	if actor != nil {
		req = req.WithContext(contextx.WithActor(req.Context(), *actor))
	}
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: projectID.String()}}
	return c
}

func TestRequireProjectRole_SystemAdmin_BypassesMembership(t *testing.T) {
	projectID := uuid.New()
	// Không set EXPECT nào -> nếu middleware lỡ gọi repo, NewMockProjectMemberRepository
	// sẽ panic "I don't know what to return" ngay khi test chạy.
	repo := domainmocks.NewMockProjectMemberRepository(t)

	actor := contextx.Actor{UserID: uuid.NewString(), Roles: []string{"admin"}}
	c := newProjectAuthContext(t, &actor, projectID)

	RequireProjectRole(repo, domain.RoleOwner)(c)

	require.False(t, c.IsAborted())
	repo.AssertNotCalled(t, "FindByProjectAndUser", mock.Anything, mock.Anything, mock.Anything)
}

func TestRequireProjectRole_Owner_Allowed(t *testing.T) {
	projectID, userID := uuid.New(), uuid.New()
	repo := domainmocks.NewMockProjectMemberRepository(t)
	repo.EXPECT().FindByProjectAndUser(mock.Anything, projectID, userID).Return(&domain.ProjectMember{
		ProjectID: projectID, UserID: userID, Role: domain.RoleOwner, Status: domain.MemberStatusActive,
	}, nil)

	actor := contextx.Actor{UserID: userID.String()}
	c := newProjectAuthContext(t, &actor, projectID)

	RequireProjectRole(repo, domain.RoleOwner)(c)

	require.False(t, c.IsAborted())
}

func TestRequireProjectRole_ViewerForbidden_OnOwnerOnlyRoute(t *testing.T) {
	projectID, userID := uuid.New(), uuid.New()
	repo := domainmocks.NewMockProjectMemberRepository(t)
	repo.EXPECT().FindByProjectAndUser(mock.Anything, projectID, userID).Return(&domain.ProjectMember{
		ProjectID: projectID, UserID: userID, Role: domain.RoleViewer, Status: domain.MemberStatusActive,
	}, nil)

	actor := contextx.Actor{UserID: userID.String()}
	c := newProjectAuthContext(t, &actor, projectID)

	RequireProjectRole(repo, domain.RoleOwner)(c)

	require.True(t, c.IsAborted())
	te, ok := apperr.AsTechnical(c.Errors.Last().Err)
	require.True(t, ok)
	require.Equal(t, errcode.Auth403, te.Code)
}

func TestRequireProjectRole_NotAMember_Forbidden(t *testing.T) {
	projectID, userID := uuid.New(), uuid.New()
	repo := domainmocks.NewMockProjectMemberRepository(t)
	repo.EXPECT().FindByProjectAndUser(mock.Anything, projectID, userID).Return(nil, domain.ErrNotFound)

	actor := contextx.Actor{UserID: userID.String()}
	c := newProjectAuthContext(t, &actor, projectID)

	RequireProjectRole(repo, domain.RoleOwner)(c)

	require.True(t, c.IsAborted())
	te, ok := apperr.AsTechnical(c.Errors.Last().Err)
	require.True(t, ok)
	require.Equal(t, errcode.Auth403, te.Code)
}

func TestRequireProjectRole_Unauthenticated_Returns401(t *testing.T) {
	projectID := uuid.New()
	repo := domainmocks.NewMockProjectMemberRepository(t)

	c := newProjectAuthContext(t, nil, projectID)

	RequireProjectRole(repo, domain.RoleOwner)(c)

	require.True(t, c.IsAborted())
	te, ok := apperr.AsTechnical(c.Errors.Last().Err)
	require.True(t, ok)
	require.Equal(t, errcode.Auth401, te.Code)
	repo.AssertNotCalled(t, "FindByProjectAndUser", mock.Anything, mock.Anything, mock.Anything)
}
