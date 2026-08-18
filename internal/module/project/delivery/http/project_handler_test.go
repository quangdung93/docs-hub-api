package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/common/pagination"
	"github.com/quangdung93/docs-hub-api/internal/middleware"
	projecthttp "github.com/quangdung93/docs-hub-api/internal/module/project/delivery/http"
	"github.com/quangdung93/docs-hub-api/internal/module/project/domain"
	domainmocks "github.com/quangdung93/docs-hub-api/internal/module/project/domain/mocks"
	"github.com/quangdung93/docs-hub-api/internal/module/project/usecase"
	ucmocks "github.com/quangdung93/docs-hub-api/internal/module/project/usecase/mocks"
)

// fakeAuth mô phỏng middleware.Auth thành công: đặt actor cố định vào context
// mà không cần verify JWT thật — dùng riêng cho test handler.
func fakeAuth(userID string, roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor := contextx.Actor{UserID: userID, Roles: roles}
		c.Request = c.Request.WithContext(contextx.WithActor(c.Request.Context(), actor))
		c.Next()
	}
}

// setupRouter dựng router thật (đúng chuỗi middleware + Register) để test cả
// handler LẪN middleware.RequireProjectRole.
func setupRouter(t *testing.T, svc usecase.Service, memberRepo domain.ProjectMemberRepository, userID string) *gin.Engine {
	return setupRouterWithRoles(t, svc, memberRepo, userID)
}

// setupRouterWithRoles giống setupRouter nhưng cho phép gắn thêm role hệ thống
// (ví dụ "admin") vào actor — dùng để test middleware.RequireProjectRole bypass.
func setupRouterWithRoles(
	t *testing.T, svc usecase.Service, memberRepo domain.ProjectMemberRepository, userID string, roles ...string,
) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID(), middleware.ErrorHandler())
	if userID != "" {
		r.Use(fakeAuth(userID, roles...))
	}
	projecthttp.Register(r.Group("/internal/api/v1"), projecthttp.NewHandler(svc), memberRepo)
	return r
}

// activeMember cấu hình memberRepo trả về một membership ACTIVE với role cho
// trước khi middleware.RequireProjectRole tra cứu (project ID lấy từ path :id).
// TẠM KHÔNG DÙNG (RBAC theo role đang tắt ở Register, xem project_handler.go)
// — giữ lại để bật lại nhanh khi RBAC quay lại.
//
//nolint:unused
func activeMember(t *testing.T, projectID, userID uuid.UUID, role domain.Role) *domainmocks.MockProjectMemberRepository {
	t.Helper()
	repo := domainmocks.NewMockProjectMemberRepository(t)
	repo.EXPECT().FindByProjectAndUser(mock.Anything, projectID, userID).
		Return(&domain.ProjectMember{
			ID: uuid.New(), ProjectID: projectID, UserID: userID,
			Role: role, Status: domain.MemberStatusActive,
		}, nil)
	return repo
}

func decodeEnvelope(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var env map[string]any
	require.NoError(t, json.Unmarshal(body, &env))
	return env
}

// --- Create ---

func TestCreate_Success_Returns201(t *testing.T) {
	userID := uuid.New()
	svc := ucmocks.NewMockService(t)
	svc.EXPECT().Create(mock.Anything, mock.MatchedBy(func(in usecase.CreateProjectInput) bool {
		return in.OwnerID == userID && in.Name == "RAG Demo"
	})).Return(domain.NewProject(userID, "RAG Demo", "mô tả", domain.ProjectSettings{}), nil)
	svc.EXPECT().AvatarURL(mock.Anything, mock.Anything).Return("", nil)

	r := setupRouter(t, svc, domainmocks.NewMockProjectMemberRepository(t), userID.String())

	body, _ := json.Marshal(map[string]any{"name": "RAG Demo", "description": "mô tả"})
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/internal/api/v1/projects", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	env := decodeEnvelope(t, w.Body.Bytes())
	require.Equal(t, true, env["success"])
	data := env["data"].(map[string]any)
	require.Equal(t, "RAG Demo", data["name"])
}

func TestCreate_ValidationError_Returns400(t *testing.T) {
	userID := uuid.New()
	r := setupRouter(t, ucmocks.NewMockService(t), domainmocks.NewMockProjectMemberRepository(t), userID.String())

	body, _ := json.Marshal(map[string]any{"description": "thiếu name"})
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/internal/api/v1/projects", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	env := decodeEnvelope(t, w.Body.Bytes())
	require.Equal(t, false, env["success"])
	errObj := env["error"].(map[string]any)
	require.Equal(t, "REQ_400", errObj["code"])
}

func TestCreate_Unauthenticated_Returns401(t *testing.T) {
	r := setupRouter(t, ucmocks.NewMockService(t), domainmocks.NewMockProjectMemberRepository(t), "")

	body, _ := json.Marshal(map[string]any{"name": "X"})
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/internal/api/v1/projects", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

// --- List ---

func TestList_Success_Returns200Paged(t *testing.T) {
	userID := uuid.New()
	svc := ucmocks.NewMockService(t)
	svc.EXPECT().List(mock.Anything, userID, mock.AnythingOfType("pagination.Query")).
		Return([]domain.Project{*domain.NewProject(userID, "P1", "", domain.ProjectSettings{})},
			pagination.Meta{Page: 1, Limit: 20, TotalItems: 1}, nil)
	svc.EXPECT().AvatarURL(mock.Anything, mock.Anything).Return("", nil)

	r := setupRouter(t, svc, domainmocks.NewMockProjectMemberRepository(t), userID.String())

	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/internal/api/v1/projects", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	env := decodeEnvelope(t, w.Body.Bytes())
	require.Equal(t, true, env["success"])
	meta := env["meta"].(map[string]any)
	require.NotNil(t, meta["pagination"])
}

// --- Update ---

func TestUpdate_OwnerAllowed_Returns200(t *testing.T) {
	userID, projectID := uuid.New(), uuid.New()
	updated := domain.NewProject(userID, "Đã đổi tên", "", domain.ProjectSettings{})
	updated.ID = projectID

	svc := ucmocks.NewMockService(t)
	svc.EXPECT().Update(mock.Anything, mock.MatchedBy(func(in usecase.UpdateProjectInput) bool {
		return in.ID == projectID && in.Name != nil && *in.Name == "Đã đổi tên"
	})).Return(updated, nil)
	svc.EXPECT().AvatarURL(mock.Anything, mock.Anything).Return("", nil)

	r := setupRouter(t, svc, domainmocks.NewMockProjectMemberRepository(t), userID.String())

	body, _ := json.Marshal(map[string]any{"name": "Đã đổi tên"})
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPatch, "/internal/api/v1/projects/"+projectID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

// TẠM TẮT (RBAC theo role đang tắt ở Register, xem project_handler.go) — bật
// lại khi RBAC quay lại.
//
// func TestUpdate_ViewerForbidden_Returns403(t *testing.T) {
// 	userID, projectID := uuid.New(), uuid.New()
// 	r := setupRouter(t, ucmocks.NewMockService(t),
// 		activeMember(t, projectID, userID, domain.RoleViewer), userID.String())
//
// 	body, _ := json.Marshal(map[string]any{"name": "X"})
// 	w := httptest.NewRecorder()
// 	req := httptest.NewRequestWithContext(t.Context(), http.MethodPatch, "/internal/api/v1/projects/"+projectID.String(), bytes.NewReader(body))
// 	req.Header.Set("Content-Type", "application/json")
// 	r.ServeHTTP(w, req)
//
// 	require.Equal(t, http.StatusForbidden, w.Code)
// }
//
// // TestUpdate_EditorForbidden_Returns403 — theo BR (URD/SRS): Editor chỉ upload+query,
// // sửa cấu hình/thông tin dự án là hành vi quản trị, chỉ Owner mới được làm.
// func TestUpdate_EditorForbidden_Returns403(t *testing.T) {
// 	userID, projectID := uuid.New(), uuid.New()
// 	r := setupRouter(t, ucmocks.NewMockService(t),
// 		activeMember(t, projectID, userID, domain.RoleEditor), userID.String())
//
// 	body, _ := json.Marshal(map[string]any{"name": "X"})
// 	w := httptest.NewRecorder()
// 	req := httptest.NewRequestWithContext(t.Context(), http.MethodPatch, "/internal/api/v1/projects/"+projectID.String(), bytes.NewReader(body))
// 	req.Header.Set("Content-Type", "application/json")
// 	r.ServeHTTP(w, req)
//
// 	require.Equal(t, http.StatusForbidden, w.Code)
// }

func TestUpdate_ProjectNotFound_Returns404(t *testing.T) {
	userID, projectID := uuid.New(), uuid.New()
	svc := ucmocks.NewMockService(t)
	svc.EXPECT().Update(mock.Anything, mock.Anything).Return(nil, domain.ErrProjectNotFound())

	r := setupRouter(t, svc, domainmocks.NewMockProjectMemberRepository(t), userID.String())

	body, _ := json.Marshal(map[string]any{"name": "X"})
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPatch, "/internal/api/v1/projects/"+projectID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	env := decodeEnvelope(t, w.Body.Bytes())
	errObj := env["error"].(map[string]any)
	require.Equal(t, "PRJ_404", errObj["code"])
}

// --- Delete ---

func TestDelete_Owner_Returns204(t *testing.T) {
	userID, projectID := uuid.New(), uuid.New()
	svc := ucmocks.NewMockService(t)
	svc.EXPECT().Delete(mock.Anything, usecase.DeleteProjectInput{
		ID: projectID, ConfirmName: "Core Banking",
	}).Return(nil)

	r := setupRouter(t, svc, domainmocks.NewMockProjectMemberRepository(t), userID.String())

	body, _ := json.Marshal(map[string]any{"confirm_name": "Core Banking"})
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/internal/api/v1/projects/"+projectID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestDelete_ConfirmNameMismatch_Returns200Business(t *testing.T) {
	userID, projectID := uuid.New(), uuid.New()
	svc := ucmocks.NewMockService(t)
	svc.EXPECT().Delete(mock.Anything, mock.Anything).Return(domain.ErrConfirmNameMismatch)

	r := setupRouter(t, svc, domainmocks.NewMockProjectMemberRepository(t), userID.String())

	body, _ := json.Marshal(map[string]any{"confirm_name": "Sai Ten"})
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/internal/api/v1/projects/"+projectID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	env := decodeEnvelope(t, w.Body.Bytes())
	require.Equal(t, false, env["success"])
	errObj := env["error"].(map[string]any)
	require.Equal(t, "CONFIRM_NAME_MISMATCH", errObj["code"])
}

func TestDelete_MissingConfirmName_Returns400(t *testing.T) {
	userID, projectID := uuid.New(), uuid.New()
	r := setupRouter(t, ucmocks.NewMockService(t),
		domainmocks.NewMockProjectMemberRepository(t), userID.String())

	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/internal/api/v1/projects/"+projectID.String(), bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

// TẠM TẮT (RBAC theo role đang tắt ở Register, xem project_handler.go) — bật
// lại khi RBAC quay lại.
//
// func TestDelete_EditorForbidden_Returns403(t *testing.T) {
// 	userID, projectID := uuid.New(), uuid.New()
// 	r := setupRouter(t, ucmocks.NewMockService(t),
// 		activeMember(t, projectID, userID, domain.RoleEditor), userID.String())
//
// 	body, _ := json.Marshal(map[string]any{"confirm_name": "bất kỳ"})
// 	w := httptest.NewRecorder()
// 	req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/internal/api/v1/projects/"+projectID.String(), bytes.NewReader(body))
// 	req.Header.Set("Content-Type", "application/json")
// 	r.ServeHTTP(w, req)
//
// 	require.Equal(t, http.StatusForbidden, w.Code)
// }

func TestDelete_SystemAdmin_BypassesMembership_Returns204(t *testing.T) {
	userID, projectID := uuid.New(), uuid.New()
	svc := ucmocks.NewMockService(t)
	svc.EXPECT().Delete(mock.Anything, usecase.DeleteProjectInput{
		ID: projectID, ConfirmName: "Core Banking",
	}).Return(nil)

	// Admin hệ thống KHÔNG phải thành viên dự án -> memberRepo không set EXPECT nào,
	// nếu middleware lỡ gọi FindByProjectAndUser test sẽ panic ngay.
	memberRepo := domainmocks.NewMockProjectMemberRepository(t)
	r := setupRouterWithRoles(t, svc, memberRepo, userID.String(), "admin")

	body, _ := json.Marshal(map[string]any{"confirm_name": "Core Banking"})
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/internal/api/v1/projects/"+projectID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	memberRepo.AssertNotCalled(t, "FindByProjectAndUser", mock.Anything, mock.Anything, mock.Anything)
}

// --- Avatar ---

func TestRequestAvatarUpload_Success_Returns200(t *testing.T) {
	userID, projectID := uuid.New(), uuid.New()
	svc := ucmocks.NewMockService(t)
	svc.EXPECT().RequestAvatarUpload(mock.Anything, usecase.RequestAvatarUploadInput{
		ProjectID: projectID, MimeType: "image/png", SizeBytes: 100,
	}).Return(&usecase.RequestAvatarUploadResult{
		UploadURL: "https://minio.local/put-url", ExpiresAt: time.Now(),
	}, nil)

	r := setupRouter(t, svc, domainmocks.NewMockProjectMemberRepository(t), userID.String())

	body, _ := json.Marshal(map[string]any{"mime_type": "image/png", "size_bytes": 100})
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(),
		http.MethodPost, "/internal/api/v1/projects/"+projectID.String()+"/avatar/upload-url", bytes.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	env := decodeEnvelope(t, w.Body.Bytes())
	data := env["data"].(map[string]any)
	require.Equal(t, "https://minio.local/put-url", data["upload_url"])
}

func TestRequestAvatarUpload_InvalidFormat_Returns200Business(t *testing.T) {
	userID, projectID := uuid.New(), uuid.New()
	svc := ucmocks.NewMockService(t)
	svc.EXPECT().RequestAvatarUpload(mock.Anything, mock.Anything).Return(nil, domain.ErrInvalidAvatarFormat)

	r := setupRouter(t, svc, domainmocks.NewMockProjectMemberRepository(t), userID.String())

	body, _ := json.Marshal(map[string]any{"mime_type": "application/pdf", "size_bytes": 100})
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(),
		http.MethodPost, "/internal/api/v1/projects/"+projectID.String()+"/avatar/upload-url", bytes.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// Điểm mấu chốt chuẩn ISC: lỗi nghiệp vụ -> HTTP 200.
	require.Equal(t, http.StatusOK, w.Code)
	env := decodeEnvelope(t, w.Body.Bytes())
	require.Equal(t, false, env["success"])
	errObj := env["error"].(map[string]any)
	require.Equal(t, "IMAGE_INVALID", errObj["code"])
}

func TestCompleteAvatarUpload_Success_Returns200(t *testing.T) {
	userID, projectID := uuid.New(), uuid.New()
	updated := domain.NewProject(userID, "RAG Demo", "", domain.ProjectSettings{})
	updated.ID = projectID
	updated.SetAvatar("projects/" + projectID.String() + "/avatar")

	svc := ucmocks.NewMockService(t)
	svc.EXPECT().CompleteAvatarUpload(mock.Anything, projectID).Return(updated, nil)
	svc.EXPECT().AvatarURL(mock.Anything, updated).Return("https://minio.local/get-url", nil)

	r := setupRouter(t, svc, domainmocks.NewMockProjectMemberRepository(t), userID.String())

	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(),
		http.MethodPost, "/internal/api/v1/projects/"+projectID.String()+"/avatar/complete", nil,
	)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	env := decodeEnvelope(t, w.Body.Bytes())
	data := env["data"].(map[string]any)
	require.Equal(t, "https://minio.local/get-url", data["avatar_url"])
}

func TestCompleteAvatarUpload_NotUploaded_Returns200Business(t *testing.T) {
	userID, projectID := uuid.New(), uuid.New()
	svc := ucmocks.NewMockService(t)
	svc.EXPECT().CompleteAvatarUpload(mock.Anything, projectID).Return(nil, domain.ErrAvatarNotUploaded)

	r := setupRouter(t, svc, domainmocks.NewMockProjectMemberRepository(t), userID.String())

	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(),
		http.MethodPost, "/internal/api/v1/projects/"+projectID.String()+"/avatar/complete", nil,
	)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	env := decodeEnvelope(t, w.Body.Bytes())
	require.Equal(t, false, env["success"])
	errObj := env["error"].(map[string]any)
	require.Equal(t, "AVATAR_NOT_UPLOADED", errObj["code"])
}

// --- Members ---

func TestListMembers_ViewerAllowed_Returns200(t *testing.T) {
	userID, projectID := uuid.New(), uuid.New()
	svc := ucmocks.NewMockService(t)
	svc.EXPECT().ListMembers(mock.Anything, projectID).
		Return([]domain.ProjectMember{*domain.NewInvite(projectID, uuid.New(), domain.RoleViewer)}, nil)

	r := setupRouter(t, svc, domainmocks.NewMockProjectMemberRepository(t), userID.String())

	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/internal/api/v1/projects/"+projectID.String()+"/members", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestInviteMember_Owner_Returns201(t *testing.T) {
	userID, projectID, inviteeID := uuid.New(), uuid.New(), uuid.New()
	svc := ucmocks.NewMockService(t)
	svc.EXPECT().InviteMember(mock.Anything, usecase.InviteMemberInput{
		ProjectID: projectID, UserID: inviteeID, Role: domain.RoleEditor,
	}).Return(domain.NewInvite(projectID, inviteeID, domain.RoleEditor), nil)

	r := setupRouter(t, svc, domainmocks.NewMockProjectMemberRepository(t), userID.String())

	body, _ := json.Marshal(map[string]any{"user_id": inviteeID.String(), "role": "editor"})
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(),
		http.MethodPost, "/internal/api/v1/projects/"+projectID.String()+"/members", bytes.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
}

// TẠM TẮT (RBAC theo role đang tắt ở Register, xem project_handler.go) — bật
// lại khi RBAC quay lại.
//
// func TestInviteMember_EditorForbidden_Returns403(t *testing.T) {
// 	userID, projectID, inviteeID := uuid.New(), uuid.New(), uuid.New()
// 	r := setupRouter(t, ucmocks.NewMockService(t),
// 		activeMember(t, projectID, userID, domain.RoleEditor), userID.String())
//
// 	body, _ := json.Marshal(map[string]any{"user_id": inviteeID.String(), "role": "editor"})
// 	w := httptest.NewRecorder()
// 	req := httptest.NewRequestWithContext(t.Context(),
// 		http.MethodPost, "/internal/api/v1/projects/"+projectID.String()+"/members", bytes.NewReader(body),
// 	)
// 	req.Header.Set("Content-Type", "application/json")
// 	r.ServeHTTP(w, req)
//
// 	require.Equal(t, http.StatusForbidden, w.Code)
// }

func TestInviteMember_AlreadyMember_Returns200Business(t *testing.T) {
	userID, projectID, inviteeID := uuid.New(), uuid.New(), uuid.New()
	svc := ucmocks.NewMockService(t)
	svc.EXPECT().InviteMember(mock.Anything, mock.Anything).Return(nil, domain.ErrAlreadyMember)

	r := setupRouter(t, svc, domainmocks.NewMockProjectMemberRepository(t), userID.String())

	body, _ := json.Marshal(map[string]any{"user_id": inviteeID.String(), "role": "editor"})
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(),
		http.MethodPost, "/internal/api/v1/projects/"+projectID.String()+"/members", bytes.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// Điểm mấu chốt chuẩn ISC: lỗi nghiệp vụ -> HTTP 200.
	require.Equal(t, http.StatusOK, w.Code)
	env := decodeEnvelope(t, w.Body.Bytes())
	require.Equal(t, false, env["success"])
	errObj := env["error"].(map[string]any)
	require.Equal(t, "ALREADY_MEMBER", errObj["code"])
}

func TestChangeMemberRole_Owner_Returns200(t *testing.T) {
	userID, projectID, targetID := uuid.New(), uuid.New(), uuid.New()
	updated := domain.NewInvite(projectID, targetID, domain.RoleViewer)
	updated.Status = domain.MemberStatusActive

	svc := ucmocks.NewMockService(t)
	svc.EXPECT().ChangeMemberRole(mock.Anything, usecase.ChangeMemberRoleInput{
		ProjectID: projectID, UserID: targetID, Role: domain.RoleViewer,
	}).Return(updated, nil)

	r := setupRouter(t, svc, domainmocks.NewMockProjectMemberRepository(t), userID.String())

	body, _ := json.Marshal(map[string]any{"role": "viewer"})
	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(),
		http.MethodPatch,
		"/internal/api/v1/projects/"+projectID.String()+"/members/"+targetID.String(),
		bytes.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestRemoveMember_Owner_Returns204(t *testing.T) {
	userID, projectID, targetID := uuid.New(), uuid.New(), uuid.New()
	svc := ucmocks.NewMockService(t)
	svc.EXPECT().RemoveMember(mock.Anything, projectID, targetID).Return(nil)

	r := setupRouter(t, svc, domainmocks.NewMockProjectMemberRepository(t), userID.String())

	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(),
		http.MethodDelete, "/internal/api/v1/projects/"+projectID.String()+"/members/"+targetID.String(), nil,
	)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestAcceptInvite_Success_Returns200(t *testing.T) {
	userID, projectID := uuid.New(), uuid.New()
	accepted := domain.NewInvite(projectID, userID, domain.RoleViewer)
	accepted.Status = domain.MemberStatusActive

	svc := ucmocks.NewMockService(t)
	svc.EXPECT().AcceptInvite(mock.Anything, projectID, userID).Return(accepted, nil)

	// Accept KHÔNG qua RequireProjectRole (chưa active) -> memberRepo không được gọi.
	r := setupRouter(t, svc, domainmocks.NewMockProjectMemberRepository(t), userID.String())

	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(),
		http.MethodPost, "/internal/api/v1/projects/"+projectID.String()+"/members/me/accept", nil,
	)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestAcceptInvite_NotPending_Returns200Business(t *testing.T) {
	userID, projectID := uuid.New(), uuid.New()
	svc := ucmocks.NewMockService(t)
	svc.EXPECT().AcceptInvite(mock.Anything, projectID, userID).Return(nil, domain.ErrInviteNotPending)

	r := setupRouter(t, svc, domainmocks.NewMockProjectMemberRepository(t), userID.String())

	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(),
		http.MethodPost, "/internal/api/v1/projects/"+projectID.String()+"/members/me/accept", nil,
	)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	env := decodeEnvelope(t, w.Body.Bytes())
	require.Equal(t, false, env["success"])
	errObj := env["error"].(map[string]any)
	require.Equal(t, "INVITE_NOT_PENDING", errObj["code"])
}
