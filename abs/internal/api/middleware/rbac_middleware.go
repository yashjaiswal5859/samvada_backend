// =============================================================================
// Samvada Platform — ABS Ory Keto ReBAC (Relationship-Based Access Control)
// =============================================================================
// Package: middleware
//
// Responsibilities:
//   1. Enforces Google Zanzibar-style relationship-based access control (ReBAC)
//      via Ory Keto on tenant workspace operations.
//   2. Queries Keto tuples (e.g., workspaces:<workspace_id>#<relation>@<user_id>).
//   3. Supports fine-grained relationship hierarchy (owner > admin > operator > viewer).
//   4. Enforces authoritative ReBAC verification strictly via Ory Keto.
// =============================================================================

package middleware

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/samvada/abs/internal/infrastructure/keto"
	"github.com/samvada/abs/internal/infrastructure/redis"
)

// ReBAC Namespaces and Role Constants
const (
	NamespaceWorkspaces = "workspaces"
	NamespacePlatform   = "platform"
	NamespaceAgents     = "agents"

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

// candidateRelationsForMinRole returns the list of Ory Keto relations that satisfy a minimum required role.
func candidateRelationsForMinRole(minRole string) []string {
	switch minRole {
	case RoleOwner:
		return []string{RoleOwner}
	case RoleAdmin, RoleWorkspaceAdmin:
		return []string{RoleOwner, RoleAdmin, RoleWorkspaceAdmin}
	case RoleOperator:
		return []string{RoleOwner, RoleAdmin, RoleWorkspaceAdmin, RoleOperator}
	case RoleViewer:
		return []string{RoleOwner, RoleAdmin, RoleWorkspaceAdmin, RoleOperator, RoleViewer, "member"}
	default:
		return []string{RoleOwner, RoleAdmin, RoleWorkspaceAdmin, RoleOperator}
	}
}

// ReBACMiddleware handles relationship-based authorization with Ory Keto.
type ReBACMiddleware struct {
	ketoClient keto.Client
	db         *gorm.DB
	redis      *redis.Client
	appEnv     string
	log        *zap.Logger
}

var (
	defaultReBACMu sync.RWMutex
	defaultReBAC   *ReBACMiddleware
)

// SetDefaultReBACMiddleware registers the active ReBAC middleware instance globally for route handlers.
func SetDefaultReBACMiddleware(mw *ReBACMiddleware) {
	defaultReBACMu.Lock()
	defer defaultReBACMu.Unlock()
	defaultReBAC = mw
}

// GetDefaultReBACMiddleware returns the active ReBAC middleware instance.
func GetDefaultReBACMiddleware() *ReBACMiddleware {
	defaultReBACMu.RLock()
	defer defaultReBACMu.RUnlock()
	return defaultReBAC
}

// NewReBACMiddleware creates a new ReBACMiddleware instance.
func NewReBACMiddleware(
	ketoClient keto.Client,
	db *gorm.DB,
	redis *redis.Client,
	appEnv string,
	log *zap.Logger,
) *ReBACMiddleware {
	mw := &ReBACMiddleware{
		ketoClient: ketoClient,
		db:         db,
		redis:      redis,
		appEnv:     appEnv,
		log:        log.Named("rebac_middleware"),
	}
	SetDefaultReBACMiddleware(mw)
	return mw
}

// RequireWorkspaceReBAC enforces that the authenticated caller holds a relationship
// to the target workspace that satisfies the required minRole in Ory Keto.
func (m *ReBACMiddleware) RequireWorkspaceReBAC(minRole string) gin.HandlerFunc {
	candidates := candidateRelationsForMinRole(minRole)

	return func(c *gin.Context) {
		wsID := GetWorkspaceID(c)
		if wsID == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "MISSING_WORKSPACE_CONTEXT",
					"message": "Valid workspace context required for ReBAC authorization",
				},
			})
			return
		}

		userID := GetUserID(c)
		if userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "UNAUTHORIZED",
					"message": "Authentication required prior to ReBAC permission check",
				},
			})
			return
		}

		// 1. Fast Redis cache check
		cacheKey := fmt.Sprintf("samvada:rebac:%s:%s:%s", wsID, userID, minRole)
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		if m != nil && m.redis != nil {
			if cachedRel, err := m.redis.Get(ctx, cacheKey); err == nil && cachedRel != "" {
				c.Set(ContextWorkspaceRoleKey, cachedRel)
				c.Next()
				return
			}
		}

		// 2. Authoritative Ory Keto ReBAC Check (Google Zanzibar tuples)
		if m == nil || m.ketoClient == nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "KETO_CLIENT_UNINITIALIZED",
					"message": "Ory Keto authorization client is uninitialized",
				},
			})
			return
		}

		var isAllowed bool
		var matchedRelation string
		var ketoError error

		for _, rel := range candidates {
			allowed, err := m.ketoClient.CheckPermission(ctx, NamespaceWorkspaces, wsID, rel, userID)
			if err != nil {
				ketoError = err
				break
			}
			if allowed {
				isAllowed = true
				matchedRelation = rel
				break
			}
		}

		if ketoError != nil {
			if m.log != nil {
				m.log.Error("Ory Keto ReBAC service unavailable",
					zap.Error(ketoError),
					zap.String("workspace_id", wsID),
					zap.String("user_id", userID),
				)
			}
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "KETO_SERVICE_UNAVAILABLE",
					"message": "Authorization engine (Ory Keto) is currently unavailable",
				},
			})
			return
		}

		if isAllowed {
			if m.redis != nil {
				_ = m.redis.Set(ctx, cacheKey, matchedRelation, 60*time.Second)
			}
			c.Set(ContextWorkspaceRoleKey, matchedRelation)
			c.Next()
			return
		}

		// Explicitly rejected by Keto ReBAC engine
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "FORBIDDEN_KETO_REBAC_DENIED",
				"message": fmt.Sprintf("Access denied: Subject lacks required ReBAC relation (%s) on workspace in Ory Keto", minRole),
			},
		})
		return
	}
}

// RequireMinWorkspaceRole is the canonical route middleware enforcing ReBAC role verification.
func RequireMinWorkspaceRole(minRole string) gin.HandlerFunc {
	rebac := GetDefaultReBACMiddleware()
	if rebac != nil {
		return rebac.RequireWorkspaceReBAC(minRole)
	}

	// Fallback when default ReBAC middleware is not yet initialized
	minLevel, ok := roleHierarchy[minRole]
	if !ok {
		minLevel = 1
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

// RequireWorkspaceRole ensures the caller has one of the explicitly allowed roles within the active workspace.
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
