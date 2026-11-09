package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

// extractSubjectFromJWT is a helper that extracts the subject claim from a JWT
// WITHOUT verifying the signature. This is only used when the JWT was just
// issued by our own Hydra instance in the same request (e.g., GenerateTokens).
// For externally-received JWTs, always use ValidateIDToken with JWKS verification.
func extractSubjectFromJWT(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid token format: must be a JWT")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("failed to decode JWT payload: %v", err)
	}

	var claims struct {
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return "", fmt.Errorf("failed to parse claims payload: %v", err)
	}

	return claims.Sub, nil
}

// ShouldSecureCookie determines whether the Secure flag should be set on cookies.
// In true HTTPS environments (c.Request.TLS != nil, X-Forwarded-Proto == "https", or non-localhost production),
// it sets Secure=true to protect against network sniffing.
// In localhost/development testing over plain HTTP, it sets Secure=false so modern browsers don't drop the cookies.
func ShouldSecureCookie(c *gin.Context, isProduction bool) bool {
	if !isProduction {
		return false
	}
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		return true
	}
	host := c.Request.Host
	if strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1") {
		return false
	}
	return true
}

// FlushAuthCookies clears all authentication and session cookies from the client browser.
// Called whenever session validation fails, refresh token is revoked/invalid, or user is not found.
func FlushAuthCookies(c *gin.Context, isProduction bool) {
	secure := ShouldSecureCookie(c, isProduction)
	c.SetCookie("access_token", "", -1, "/", "", secure, true)
	c.SetCookie("refresh_token", "", -1, "/", "", secure, true)
	c.SetCookie("refresh_token", "", -1, "/api/v1/auth", "", secure, true)
	c.SetCookie("session_id", "", -1, "/", "", secure, true)
	c.SetCookie("login_session_id", "", -1, "/", "", secure, true)
	c.SetCookie("registration_session_id", "", -1, "/", "", secure, true)
	c.SetCookie("login_challenge", "", -1, "/", "", secure, false)
}

