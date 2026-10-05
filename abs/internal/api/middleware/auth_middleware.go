// =============================================================================
// Samvada Platform — ABS JWT Authentication Middleware
// =============================================================================
// Package: middleware
//
// Responsibilities:
//   1. Extracts Bearer token from the HTTP Authorization header.
//   2. Validates cryptographic signature and claims using JWT secret.
//   3. Detects expired tokens specifically (returns HTTP 401 with code: TOKEN_EXPIRED)
//      to trigger transparent frontend token refresh via axios interceptors.
//   4. Injects authenticated user_id, email, and claims into the Gin context.
// =============================================================================

package middleware

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"github.com/samvada/abs/configs"
	apperrors "github.com/samvada/proto/pkg/errors"
)

const (
	ContextUserIDKey    = "user_id"
	ContextUserEmailKey = "user_email"
	ContextClaimsKey    = "jwt_claims"
)

// AuthMiddleware validates JWT access tokens.
type AuthMiddleware struct {
	jwtSecret  []byte
	jwksURL    string
	jwksKeys   map[string]*rsa.PublicKey
	jwksExpiry time.Time
	jwksMu     sync.RWMutex
	appEnv     string
	log        *zap.Logger
}

// NewAuthMiddleware initializes the authentication middleware.
func NewAuthMiddleware(cfg *configs.Config, log *zap.Logger) *AuthMiddleware {
	jwksURL := os.Getenv("HYDRA_JWKS_URL")
	if jwksURL == "" {
		jwksURL = "http://hydra:4444/.well-known/jwks.json"
	}
	return &AuthMiddleware{
		jwtSecret: []byte(cfg.Auth.JWTSecret),
		jwksURL:   jwksURL,
		jwksKeys:  make(map[string]*rsa.PublicKey),
		appEnv:    cfg.App.Env,
		log:       log.Named("auth_middleware"),
	}
}

func (m *AuthMiddleware) getRSAPublicKey(t *jwt.Token) (*rsa.PublicKey, error) {
	kid, _ := t.Header["kid"].(string)

	m.jwksMu.RLock()
	if time.Now().Before(m.jwksExpiry) {
		if key, ok := m.jwksKeys[kid]; ok {
			m.jwksMu.RUnlock()
			return key, nil
		}
	}
	m.jwksMu.RUnlock()

	if err := m.refreshJWKS(); err != nil {
		m.jwksMu.RLock()
		if key, ok := m.jwksKeys[kid]; ok {
			m.jwksMu.RUnlock()
			return key, nil
		}
		m.jwksMu.RUnlock()
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}

	m.jwksMu.RLock()
	defer m.jwksMu.RUnlock()
	if key, ok := m.jwksKeys[kid]; ok {
		return key, nil
	}
	return nil, fmt.Errorf("key kid %q not found in JWKS", kid)
}

func (m *AuthMiddleware) refreshJWKS() error {
	m.jwksMu.Lock()
	defer m.jwksMu.Unlock()

	url := m.jwksURL
	if url == "" {
		url = "http://hydra:4444/.well-known/jwks.json"
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		if strings.Contains(url, "hydra:4444") {
			resp, err = client.Get("http://localhost:4444/.well-known/jwks.json")
		}
		if err != nil {
			return err
		}
	}
	defer resp.Body.Close()

	var jwks struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return err
	}

	newKeys := make(map[string]*rsa.PublicKey)
	for _, keyJSON := range jwks.Keys {
		var keyMeta struct {
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		}
		if err := json.Unmarshal(keyJSON, &keyMeta); err != nil {
			continue
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(keyMeta.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(keyMeta.E)
		if err != nil {
			continue
		}
		var eVal int
		for _, b := range eBytes {
			eVal = eVal<<8 + int(b)
		}
		newKeys[keyMeta.Kid] = &rsa.PublicKey{
			N: new(big.Int).SetBytes(nBytes),
			E: eVal,
		}
	}

	m.jwksKeys = newKeys
	m.jwksExpiry = time.Now().Add(1 * time.Hour)
	return nil
}

// RequireAuth is a Gin middleware that enforces authentication on protected routes.
func (m *AuthMiddleware) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. Development bypass header (only permitted in non-production environments)
		if m.appEnv == "development" {
			if devUserID := c.GetHeader("X-Dev-User-ID"); devUserID != "" {
				m.log.Debug("Dev user bypass active", zap.String("dev_user_id", devUserID))
				c.Set(ContextUserIDKey, devUserID)
				c.Set(ContextUserEmailKey, c.GetHeader("X-Dev-User-Email"))
				c.Next()
				return
			}
		}

		// 2. Extract Bearer token from HttpOnly cookie or Authorization header
		tokenString := ""
		if cookieToken, err := c.Cookie("access_token"); err == nil && cookieToken != "" {
			tokenString = cookieToken
		}

		if tokenString == "" {
			authHeader := c.GetHeader("Authorization")
			if authHeader != "" {
				parts := strings.SplitN(authHeader, " ", 2)
				if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
					tokenString = strings.TrimSpace(parts[1])
				}
			}
		}

		if tokenString == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    apperrors.CodeUnauthorized,
					"message": "Authentication required. Missing Bearer token in Authorization header or cookie.",
				},
			})
			return
		}

		// 3. Parse and validate JWT
		token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodRSA); ok {
				return m.getRSAPublicKey(t)
			}
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); ok {
				return m.jwtSecret, nil
			}
			return nil, errors.New("unexpected signing method")
		})

		if err != nil {
			// Specific detection of token expiration:
			// Allows frontend Axios interceptor to catch 401 with code: TOKEN_EXPIRED
			// and seamlessly trigger /api/v1/auth/refresh without user interruption.
			if errors.Is(err, jwt.ErrTokenExpired) {
				m.log.Warn("Access token expired", zap.Error(err))
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"success": false,
					"error": gin.H{
						"code":    "TOKEN_EXPIRED",
						"message": "Access token has expired. Please refresh your session.",
					},
				})
				return
			}

			m.log.Warn("Invalid access token", zap.Error(err))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "INVALID_TOKEN",
					"message": "Invalid token signature or malformed payload.",
				},
			})
			return
		}

		if !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    apperrors.CodeUnauthorized,
					"message": "Token is not valid.",
				},
			})
			return
		}

		// 4. Extract claims
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    apperrors.CodeUnauthorized,
					"message": "Failed to extract token claims.",
				},
			})
			return
		}

		// 5. Extract user_id (prefer canonical 'user_id' UUID from auth DB, fallback to 'sub')
		userID := ""
		if uid, ok := claims["user_id"].(string); ok && uid != "" {
			userID = uid
		} else if sub, ok := claims["sub"].(string); ok {
			userID = sub
		}

		if userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    apperrors.CodeUnauthorized,
					"message": "Token does not contain a valid user identifier (sub).",
				},
			})
			return
		}

		email, _ := claims["email"].(string)

		// 6. Inject into Gin Context
		c.Set(ContextUserIDKey, userID)
		c.Set(ContextUserEmailKey, email)
		c.Set(ContextClaimsKey, claims)

		c.Next()
	}
}

// GetUserID extracts the authenticated user_id from the Gin context.
func GetUserID(c *gin.Context) string {
	if val, ok := c.Get(ContextUserIDKey); ok {
		if id, ok := val.(string); ok {
			return id
		}
	}
	return ""
}

// GetUserEmail extracts the authenticated user email from the Gin context.
func GetUserEmail(c *gin.Context) string {
	if val, ok := c.Get(ContextUserEmailKey); ok {
		if email, ok := val.(string); ok {
			return email
		}
	}
	return ""
}
