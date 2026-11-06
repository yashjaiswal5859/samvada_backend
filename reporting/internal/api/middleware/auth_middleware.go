// =============================================================================
// Samvada Platform — Reporting Engine Auth Middleware (HttpOnly Cookie Priority)
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

	"github.com/samvada/reporting/configs"
	apperrors "github.com/samvada/proto/pkg/errors"
)

const (
	ContextUserIDKey    = "user_id"
	ContextUserEmailKey = "user_email"
	ContextOrgIDKey     = "org_id"
)

// Cookie names for HttpOnly secure token storage.
const (
	CookieAccessToken  = "access_token"
	CookieRefreshToken = "refresh_token"
)

// HTTP header keys and prefixes.
const (
	HeaderAuthorization = "Authorization"
	HeaderOrgID         = "X-Org-ID"
	BearerPrefix        = "bearer"
)

type AuthMiddleware struct {
	jwksURL    string
	jwksKeys   map[string]*rsa.PublicKey
	jwksExpiry time.Time
	jwksMu     sync.RWMutex
	log        *zap.Logger
}

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

func (m *AuthMiddleware) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. Extract Bearer token from HttpOnly cookie first, then Authorization header
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
			// Check if refresh_token cookie exists for transparent refresh
			if refreshCookie, err := c.Cookie(CookieRefreshToken); err == nil && refreshCookie != "" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"success": false,
					"error": gin.H{
						"code":    apperrors.CodeTokenExpired,
						"message": "Access token is missing. Use your refresh token to obtain a new one.",
					},
				})
				return
			}

			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":          apperrors.CodeTokenMissing,
					"message":       "Authentication required. No active session found.",
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
			if errors.Is(err, jwt.ErrTokenExpired) {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"success": false,
					"error": gin.H{
						"code":    apperrors.CodeTokenExpired,
						"message": "Access token expired. Please refresh your session.",
					},
				})
				return
			}

			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    apperrors.CodeInvalidToken,
					"message": "Cryptographic signature validation failed.",
				},
			})
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    apperrors.CodeInvalidToken,
					"message": "Invalid token claims.",
				},
			})
			return
		}

		// 4. Inject claims into context
		if sub, ok := claims["sub"].(string); ok {
			c.Set(ContextUserIDKey, sub)
		}
		if uid, ok := claims["user_id"].(string); ok && uid != "" {
			c.Set(ContextUserIDKey, uid)
		}
		if email, ok := claims["email"].(string); ok {
			c.Set(ContextUserEmailKey, email)
		}
		if orgID, ok := claims["org_id"].(string); ok {
			c.Set(ContextOrgIDKey, orgID)
		}

		c.Next()
	}
}
