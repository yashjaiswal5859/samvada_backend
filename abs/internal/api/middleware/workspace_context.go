// =============================================================================
// Samvada Platform — ABS Workspace Context & Tenant Isolation Middleware
// =============================================================================
// Package: middleware
//
// Responsibilities:
//   1. Extracts the target workspace identifier from X-Workspace-ID header,
//      path parameter, or query string.
//   2. Enforces multi-tenant isolation by verifying that the authenticated user
//      has an active membership record in the workspace_members table.
//   3. Uses Redis caching (60s TTL) to reduce database load on repeated queries.
//   4. Injects workspace_id and workspace_role into the Gin context.
// =============================================================================

package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/samvada/abs/internal/infrastructure/redis"
	apperrors "github.com/samvada/proto/pkg/errors"
)

const (
	ContextWorkspaceIDKey   = "workspace_id"
	ContextWorkspaceRoleKey = "workspace_role"
)

// WorkspaceMiddleware resolves and enforces tenant context.
type WorkspaceMiddleware struct {
	db     *gorm.DB
	redis  *redis.Client
	appEnv string
	log    *zap.Logger
}

// NewWorkspaceMiddleware initializes the workspace middleware.
func NewWorkspaceMiddleware(db *gorm.DB, rdb *redis.Client, appEnv string, log *zap.Logger) *WorkspaceMiddleware {
	return &WorkspaceMiddleware{
		db:     db,
		redis:  rdb,
		appEnv: appEnv,
		log:    log.Named("workspace_middleware"),
	}
}

// ResolveWorkspace extracts workspace ID without strictly requiring it (for optional workspace routes).
func (m *WorkspaceMiddleware) ResolveWorkspace() gin.HandlerFunc {
	return func(c *gin.Context) {
		wsID := m.extractWorkspaceID(c)
		if wsID != "" {
			c.Set(ContextWorkspaceIDKey, wsID)
		}
		c.Next()
	}
}

// RequireWorkspace enforces that a valid workspace is targeted and the user has membership.
func (m *WorkspaceMiddleware) RequireWorkspace() gin.HandlerFunc {
	return func(c *gin.Context) {
		wsID := m.extractWorkspaceID(c)
		if wsID == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error": gin.H{
					"code":    apperrors.CodeInvalidInput,
					"message": "Missing workspace identifier. Provide via X-Workspace-ID header or path parameter.",
				},
			})
			return
		}

		userID := GetUserID(c)
		if userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    apperrors.CodeUnauthorized,
					"message": "Authentication required prior to workspace validation.",
				},
			})
			return
		}

		// Development bypass for automated tests
		if m.appEnv == "development" && c.GetHeader("X-Dev-User-ID") != "" {
			c.Set(ContextWorkspaceIDKey, wsID)
			c.Set(ContextWorkspaceRoleKey, "workspace_admin")
			c.Next()
			return
		}

		// 1. Check Redis cache for fast membership verification
		cacheKey := fmt.Sprintf("samvada:ws_member:%s:%s", wsID, userID)
		ctx, cancel := context.WithTimeout(c.Request.Context(), 1*time.Second)
		defer cancel()

		if m.redis != nil {
			if role, err := m.redis.Get(ctx, cacheKey); err == nil && role != "" {
				c.Set(ContextWorkspaceIDKey, wsID)
				c.Set(ContextWorkspaceRoleKey, role)
				c.Next()
				return
			}
		}

		// 2. Query PostgreSQL workspace_members table
		var role string
		err := m.db.WithContext(c.Request.Context()).
			Table("workspace_members").
			Select("workspace_role").
			Where("workspace_id = ? AND user_id = ?", wsID, userID).
			Scan(&role).Error

		if err != nil || role == "" {
			m.log.Warn("Workspace access denied",
				zap.String("workspace_id", wsID),
				zap.String("user_id", userID),
				zap.Error(err),
			)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error": gin.H{
					"code":    apperrors.CodeForbidden,
					"message": "Access denied. You are not a member of the requested workspace.",
				},
			})
			return
		}

		// Cache successful membership for 60 seconds
		if m.redis != nil {
			_ = m.redis.Set(ctx, cacheKey, role, 60*time.Second)
		}

		c.Set(ContextWorkspaceIDKey, wsID)
		c.Set(ContextWorkspaceRoleKey, role)
		c.Next()
	}
}

func (m *WorkspaceMiddleware) extractWorkspaceID(c *gin.Context) string {
	// 1. Header takes highest precedence
	if header := c.GetHeader("X-Workspace-ID"); header != "" {
		return header
	}

	// 2. Path parameter ':workspace_id'
	if param := c.Param("workspace_id"); param != "" {
		return param
	}

	// 3. Query string '?workspace_id=...'
	if query := c.Query("workspace_id"); query != "" {
		return query
	}

	return ""
}

// GetWorkspaceID retrieves the active workspace ID from context.
func GetWorkspaceID(c *gin.Context) string {
	if val, ok := c.Get(ContextWorkspaceIDKey); ok {
		if id, ok := val.(string); ok {
			return id
		}
	}
	return ""
}

// GetWorkspaceRole retrieves the user's role in the active workspace.
func GetWorkspaceRole(c *gin.Context) string {
	if val, ok := c.Get(ContextWorkspaceRoleKey); ok {
		if role, ok := val.(string); ok {
			return role
		}
	}
	return ""
}
