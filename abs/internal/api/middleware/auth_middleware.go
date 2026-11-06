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

// Gin context keys for authenticated user identity.
const (
	ContextUserIDKey    = "user_id"
	ContextUserEmailKey = "user_email"
	ContextClaimsKey    = "jwt_claims"
)

// Cookie names for HttpOnly secure token storage.
const (
	CookieAccessToken  = "access_token"
	CookieRefreshToken = "refresh_token"
)

// HTTP header keys and prefixes.
const (
	HeaderAuthorization = "Authorization"
	BearerPrefix        = "bearer"
)

// Canonical error messages — single source of truth for all auth error responses.
const (
	MsgTokenExpiredRefresh = "Access token is missing. Use your refresh token to obtain a new one."
	MsgTokenExpired        = "Access token has expired. Please refresh your session."
	MsgNoActiveSession     = "Authentication required. No active session found."
	MsgInvalidToken        = "Invalid token signature or malformed payload."
	MsgTokenNotValid       = "Token is not valid."
	MsgClaimsExtractFailed = "Failed to extract token claims."
	MsgMissingUserID       = "Token does not contain a valid user identifier (sub)."
)

// AuthMiddleware validates JWT access tokens exclusively via Ory Hydra RS256 JWKS.
type AuthMiddleware struct {
	jwksURL    string
	jwksKeys   map[string]*rsa.PublicKey
	jwksExpiry time.Time
	jwksMu     sync.RWMutex
	log        *zap.Logger
}

// NewAuthMiddleware initializes the authentication middleware.
func NewAuthMiddleware(cfg *configs.Config, log *zap.Logger) *AuthMiddleware {
	jwksURL := os.Getenv("HYDRA_JWKS_URL")
	if jwksURL == "" {
		jwksURL = "http://hydra:4444/.well-known/jwks.json"
	}
	return &AuthMiddleware{
		jwksURL:  jwksURL,
		jwksKeys: make(map[string]*rsa.PublicKey),
		log:      log.Named("auth_middleware"),
	}
}

// SetTestPublicKey allows test harnesses to register an RSA public key directly,
// maintaining strict RS256 token verification without HMAC fallback.
func (m *AuthMiddleware) SetTestPublicKey(kid string, pubKey *rsa.PublicKey) {
	m.jwksMu.Lock()
	defer m.jwksMu.Unlock()
	m.jwksKeys[kid] = pubKey
	m.jwksExpiry = time.Now().Add(24 * time.Hour)
}

func (m *AuthMiddleware) getRSAPublicKey(t *jwt.Token) (*rsa.PublicKey, error) {
	kid, _ := t.Header["kid"].(string)

	m.jwksMu.RLock()
	if time.Now().Before(m.jwksExpiry) {
		if kid != "" {
			if key, ok := m.jwksKeys[kid]; ok {
				m.jwksMu.RUnlock()
				return key, nil
			}
		} else if len(m.jwksKeys) == 1 {
			for _, key := range m.jwksKeys {
				m.jwksMu.RUnlock()
				return key, nil
			}
		}
	}
	m.jwksMu.RUnlock()

	if err := m.refreshJWKS(); err != nil {
		m.jwksMu.RLock()
		if kid != "" {
			if key, ok := m.jwksKeys[kid]; ok {
				m.jwksMu.RUnlock()
				return key, nil
			}
		} else if len(m.jwksKeys) == 1 {
			for _, key := range m.jwksKeys {
				m.jwksMu.RUnlock()
				return key, nil
			}
		}
		m.jwksMu.RUnlock()
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}

	m.jwksMu.RLock()
	defer m.jwksMu.RUnlock()
	if kid != "" {
		if key, ok := m.jwksKeys[kid]; ok {
			return key, nil
		}
	} else if len(m.jwksKeys) == 1 {
		for _, key := range m.jwksKeys {
			return key, nil
		}
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
		// 1. Extract Bearer token from HttpOnly cookie or Authorization header
		tokenString := ""
		if cookieToken, err := c.Cookie(CookieAccessToken); err == nil && cookieToken != "" {
			tokenString = cookieToken
		}

		if tokenString == "" {
			authHeader := c.GetHeader(HeaderAuthorization)
			if authHeader != "" {
				parts := strings.SplitN(authHeader, " ", 2)
				if len(parts) == 2 && strings.EqualFold(parts[0], BearerPrefix) {
					tokenString = strings.TrimSpace(parts[1])
				}
			}
		}

		if tokenString == "" {
			// Check if refresh_token cookie exists — if so, the frontend can
			// transparently rotate tokens via /api/v1/auth/refresh without
			// forcing the user to log in again.
			if refreshCookie, err := c.Cookie(CookieRefreshToken); err == nil && refreshCookie != "" {
				m.log.Info("Access token missing but refresh token present — signalling TOKEN_EXPIRED for transparent refresh",
					zap.String("path", c.Request.URL.Path),
					zap.String("client_ip", c.ClientIP()),
				)
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"success": false,
					"error": gin.H{
						"code":    apperrors.CodeTokenExpired,
						"message": MsgTokenExpiredRefresh,
					},
				})
				return
			}

			// Neither access_token nor refresh_token present — force login
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":          apperrors.CodeTokenMissing,
					"message":       MsgNoActiveSession,
					"require_login": true,
				},
			})
			return
		}

		// 2. Parse and validate JWT strictly via RS256 (Ory Hydra JWKS)
		token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodRSA); ok {
				return m.getRSAPublicKey(t)
			}
			return nil, errors.New("unexpected signing method: only RS256 supported from Ory Hydra")
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
						"code":    apperrors.CodeTokenExpired,
						"message": MsgTokenExpired,
					},
				})
				return
			}

			m.log.Warn("Invalid access token", zap.Error(err))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":          apperrors.CodeInvalidToken,
					"message":       MsgInvalidToken,
					"require_login": true,
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

		// 4. Extract claims
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    apperrors.CodeUnauthorized,
					"message": MsgClaimsExtractFailed,
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
					"message": MsgMissingUserID,
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
