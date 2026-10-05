package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Role hierarchy constants
const (
	RoleOwner          = "owner"
	RoleAdmin          = "admin"
	RoleWorkspaceAdmin = "workspace_admin"
	RoleOperator       = "operator"
	RoleViewer         = "viewer"
)

// roleHierarchy defines the permission weighting (higher = more privileged)
var roleHierarchy = map[string]int{
	RoleViewer:         1,
	RoleOperator:       2,
	RoleAdmin:          3,
	RoleWorkspaceAdmin: 3,
	RoleOwner:          4,
}

// RequireWorkspaceRole ensures the caller has one of the explicitly allowed roles within the active workspace
func RequireWorkspaceRole(allowedRoles ...string) gin.HandlerFunc {
	allowedSet := make(map[string]bool, len(allowedRoles))
	for _, r := range allowedRoles {
		allowedSet[r] = true
	}

	return func(c *gin.Context) {
		roleVal, exists := c.Get(ContextWorkspaceRoleKey)
		if !exists {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "FORBIDDEN_NO_WORKSPACE_ROLE",
					"message": "User does not have a role in the active workspace",
				},
			})
			return
		}

		userRole, _ := roleVal.(string)
		isAllowed := allowedSet[userRole]
		if !isAllowed && userRole == RoleWorkspaceAdmin && allowedSet[RoleAdmin] {
			isAllowed = true
		}
		if !isAllowed {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "FORBIDDEN_INSUFFICIENT_ROLE",
					"message": "User role does not satisfy the required permission for this operation",
				},
			})
			return
		}

		c.Next()
	}
}

// RequireMinWorkspaceRole ensures the caller has at least the minimum required hierarchical role
func RequireMinWorkspaceRole(minRole string) gin.HandlerFunc {
	minLevel, ok := roleHierarchy[minRole]
	if !ok {
		minLevel = 1 // default to viewer
	}

	return func(c *gin.Context) {
		roleVal, exists := c.Get(ContextWorkspaceRoleKey)
		if !exists {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "FORBIDDEN_NO_WORKSPACE_ROLE",
					"message": "User does not have an active role in this workspace",
				},
			})
			return
		}

		userRole, _ := roleVal.(string)
		userLevel, exists := roleHierarchy[userRole]
		if !exists || userLevel < minLevel {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "FORBIDDEN_INSUFFICIENT_PERMISSIONS",
					"message": "Access denied: minimum required role is " + minRole,
				},
			})
			return
		}

		c.Next()
	}
}
