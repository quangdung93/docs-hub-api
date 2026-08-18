package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/quangdung93/docs-hub-api/internal/common/apperr"
	"github.com/quangdung93/docs-hub-api/internal/common/contextx"
	"github.com/quangdung93/docs-hub-api/internal/module/project/domain"
)

// systemAdminRole là role HỆ THỐNG (toàn cục, lấy từ JWT claims — khác với Role
// trong project_members). Admin hệ thống toàn quyền trên MỌI dự án, kể cả khi
// không phải thành viên — giống hệt cách middleware.RequireRoles dùng actor.HasRole.
const systemAdminRole = "admin"

// RequireProjectRole chặn request nếu actor không phải thành viên ACTIVE của dự án
// (path param "id") với vai trò thuộc allowedRoles. Owner LUÔN được toàn quyền,
// bất kể allowedRoles truyền vào (ví dụ Editor: upload+query, Viewer: chỉ query).
// Admin hệ thống (role "admin" toàn cục) bypass toàn bộ kiểm tra thành viên.
//
// Phải đặt SAU Auth (cần actor trong context). Không tìm thấy thành viên hoặc
// không đủ quyền -> AUTH_403 (không phân biệt để tránh lộ dự án có tồn tại hay
// không cho người ngoài).
func RequireProjectRole(repo domain.ProjectMemberRepository, allowedRoles ...domain.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := contextx.ActorFrom(c.Request.Context())
		if !ok {
			abortWith(c, apperr.Unauthorized("Chưa xác thực"))
			return
		}

		// Admin hệ thống: toàn quyền trên mọi dự án, không cần là thành viên.
		if actor.HasRole(systemAdminRole) {
			c.Next()
			return
		}

		projectID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			abortWith(c, apperr.BadRequest("ID dự án không hợp lệ"))
			return
		}
		userID, err := uuid.Parse(actor.UserID)
		if err != nil {
			abortWith(c, apperr.Unauthorized("Định danh người dùng không hợp lệ"))
			return
		}

		member, err := repo.FindByProjectAndUser(c.Request.Context(), projectID, userID)
		if err != nil || member.Status != domain.MemberStatusActive {
			abortWith(c, apperr.Forbidden("Bạn không có quyền thực hiện thao tác này trên dự án"))
			return
		}

		if member.Role == domain.RoleOwner || roleAllowed(member.Role, allowedRoles) {
			c.Next()
			return
		}
		abortWith(c, apperr.Forbidden("Bạn không có quyền thực hiện thao tác này trên dự án"))
	}
}

func roleAllowed(role domain.Role, allowed []domain.Role) bool {
	for _, r := range allowed {
		if r == role {
			return true
		}
	}
	return false
}
