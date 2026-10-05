// =============================================================================
// Samvada Platform — ABS Developer API Key Authentication Middleware
// =============================================================================

package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/domain"
	apperrors "github.com/samvada/proto/pkg/errors"
)

const ContextAPIKeyIDKey = "api_key_id"

type APIKeyMiddleware struct {
	integrationRepo domain.IntegrationRepository
	log             *zap.Logger
}

func NewAPIKeyMiddleware(integrationRepo domain.IntegrationRepository, log *zap.Logger) *APIKeyMiddleware {
	return &APIKeyMiddleware{
		integrationRepo: integrationRepo,
		log:             log.Named("api_key_middleware"),
	}
}

// RequireAPIKey checks X-API-Key or Bearer sk_live_... for programmatic API authorization.
func (m *APIKeyMiddleware) RequireAPIKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		rawKey := c.GetHeader("X-API-Key")
		if rawKey == "" {
			authHeader := c.GetHeader("Authorization")
			if strings.HasPrefix(authHeader, "Bearer sk_live_") {
				rawKey = strings.TrimPrefix(authHeader, "Bearer ")
			}
		}

		if rawKey == "" || !strings.HasPrefix(rawKey, "sk_live_") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    apperrors.CodeUnauthorized,
					"message": "Valid API key starting with 'sk_live_' required in X-API-Key header",
				},
			})
			return
		}

		hash := sha256.Sum256([]byte(rawKey))
		hashedSecret := hex.EncodeToString(hash[:])

		wsIDStr := GetWorkspaceID(c)
		if wsIDStr == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "MISSING_WORKSPACE_CONTEXT",
					"message": "X-Workspace-ID header or URL parameter required",
				},
			})
			return
		}

		wsID, err := uuid.Parse(wsIDStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "INVALID_WORKSPACE_ID",
					"message": "Invalid workspace UUID format",
				},
			})
			return
		}

		keys, err := m.integrationRepo.ListAPIKeys(c.Request.Context(), wsID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "INTERNAL_ERROR",
					"message": "Failed to verify API key",
				},
			})
			return
		}

		var matchedKey *domain.APIKey
		for _, k := range keys {
			if k.HashedSecret == hashedSecret {
				matchedKey = k
				break
			}
		}

		if matchedKey == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "INVALID_API_KEY",
					"message": "Provided API key is invalid or has been revoked",
				},
			})
			return
		}

		c.Set(ContextAPIKeyIDKey, matchedKey.ID.String())
		c.Next()
	}
}
