// =============================================================================
// Samvada Platform — ABS Admin ReBAC Authentication Middleware
// =============================================================================
// Package: middleware
//
// Responsibilities:
//   1. Enforces dual-token authentication for every Admin API call:
//      - Bearer JWT Access Token in the 'Authorization' header.
//      - Refresh Token in the 'X-Refresh-Token' header.
//   2. Validates JWT signature and expiry.
//   3. Validates refresh token presence and active state.
//   4. Queries Ory Keto ReBAC engine to ensure the subject holds the special Admin role.
// =============================================================================

package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/application/admin"
	"github.com/samvada/abs/internal/infrastructure/keto"
	apperrors "github.com/samvada/proto/pkg/errors"
)

// AdminAuthMiddleware validates admin dual-token requests and checks Ory Keto roles.
type AdminAuthMiddleware struct {
	adminService admin.AdminService
	ketoClient   keto.Client
	jwtSecret    []byte
	authMW       *AuthMiddleware
	log          *zap.Logger
}

// NewAdminAuthMiddleware creates a new AdminAuthMiddleware instance.
func NewAdminAuthMiddleware(
	adminService admin.AdminService,
	ketoClient keto.Client,
	jwtSecret string,
	authMW *AuthMiddleware,
	log *zap.Logger,
) *AdminAuthMiddleware {
	return &AdminAuthMiddleware{
		adminService: adminService,
		ketoClient:   ketoClient,
		jwtSecret:    []byte(jwtSecret),
		authMW:       authMW,
		log:          log.Named("admin_auth_middleware"),
	}
}

// RequireAdminKeto enforces dual-token header validation and Ory Keto ReBAC check.
func (m *AdminAuthMiddleware) RequireAdminKeto() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. Extract Bearer Access Token from Authorization header
		authHeader := c.GetHeader(HeaderAuthorization)
		tokenString := ""
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], BearerPrefix) {
				tokenString = strings.TrimSpace(parts[1])
			}
		}

		if tokenString == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "MISSING_ACCESS_TOKEN",
					"message": "Admin authorization requires a valid Bearer access token in the Authorization header.",
				},
			})
			return
		}

		// 2. Extract Refresh Token from X-Refresh-Token header (support aliases)
		refreshToken := c.GetHeader("X-Refresh-Token")
		if refreshToken == "" {
			refreshToken = c.GetHeader("Refresh-Token")
		}
		if refreshToken == "" {
			refreshToken = c.GetHeader("X-Admin-Refresh-Token")
		}

		if refreshToken == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "MISSING_REFRESH_TOKEN",
					"message": "Admin API operations require both access token and refresh token passed in headers. Missing required X-Refresh-Token header.",
				},
			})
			return
		}

		// 3. Validate Refresh Token active state
		if _, err := m.adminService.ValidateRefreshToken(c.Request.Context(), refreshToken); err != nil {
			m.log.Warn("Invalid or expired admin refresh token presented", zap.Error(err))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "INVALID_REFRESH_TOKEN",
					"message": "The provided X-Refresh-Token is invalid, revoked, or expired.",
				},
			})
			return
		}

		// 4. Parse and Validate JWT signature and expiry
		token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodRSA); ok && m.authMW != nil {
				return m.authMW.getRSAPublicKey(t)
			}
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); ok {
				return m.jwtSecret, nil
			}
			return nil, errors.New("unexpected signing method")
		})

		if err != nil {
			if errors.Is(err, jwt.ErrTokenExpired) {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"success": false,
					"error": gin.H{
						"code":    apperrors.CodeTokenExpired,
						"message": "Admin access token has expired. Re-authenticate or refresh token pair.",
					},
				})
				return
			}

			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    apperrors.CodeInvalidToken,
					"message": MsgInvalidToken,
				},
			})
			return
		}

		if !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    apperrors.CodeInvalidToken,
					"message": MsgTokenNotValid,
				},
			})
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "INVALID_CLAIMS",
					"message": "Failed to parse JWT claims.",
				},
			})
			return
		}

		// 5. Extract Subject / User ID
		sub := ""
		if uid, ok := claims["user_id"].(string); ok && uid != "" {
			sub = uid
		} else if s, ok := claims["sub"].(string); ok {
			sub = s
		}

		if sub == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "MISSING_SUBJECT",
					"message": "Access token does not contain a valid user identifier (sub).",
				},
			})
			return
		}

		// 6. Check Ory Keto ReBAC Permission (Special Admin Role)
		ctx, cancel := context.WithTimeout(c.Request.Context(), 4*time.Second)
		defer cancel()

		allowed, ketoErr := m.ketoClient.CheckPermission(ctx, "platform", "platform", "admin", sub)
		if ketoErr != nil {
			m.log.Error("Ory Keto ReBAC check error", zap.Error(ketoErr), zap.String("subject", sub))
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "AUTHORIZATION_SERVICE_UNAVAILABLE",
					"message": "Unable to verify admin role against Ory Keto ReBAC engine.",
				},
			})
			return
		}

		if !allowed {
			// Check secondary Zanzibar representation platform/roles:admin/member
			allowedAlt, _ := m.ketoClient.CheckPermission(ctx, "platform", "roles:admin", "member", sub)
			if !allowedAlt {
				m.log.Warn("Admin API call denied: Subject lacks admin relation in Keto", zap.String("subject", sub))
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"success": false,
					"error": gin.H{
						"code":    "FORBIDDEN_KETO_ADMIN_ROLE_REQUIRED",
						"message": "Forbidden: Subject does not possess the special Admin role in Ory Keto ReBAC.",
					},
				})
				return
			}
		}

		// 7. Inject verified context for subsequent handlers
		c.Set(ContextUserIDKey, sub)
		c.Set(ContextClaimsKey, claims)
		c.Set("is_admin", true)

		c.Next()
	}
}
