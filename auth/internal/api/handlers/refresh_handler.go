package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	apidto "github.com/ridesafety/auth-service/internal/api/dto"
	apprefresh "github.com/ridesafety/auth-service/internal/application/refresh"
	"github.com/ridesafety/auth-service/internal/platform/cache"
	"github.com/ridesafety/auth-service/internal/shared/response"
)

type RefreshHandler struct {
	refreshService apprefresh.Service
	cacheProvider  cache.CacheProvider
	isProduction   bool
}

func NewRefreshHandler(service apprefresh.Service, cacheProvider cache.CacheProvider, isProduction bool) *RefreshHandler {
	return &RefreshHandler{
		refreshService: service,
		cacheProvider:  cacheProvider,
		isProduction:   isProduction,
	}
}

type GracePeriodPayload struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

func (h *RefreshHandler) Refresh(c *gin.Context) {
	refreshToken, err := c.Cookie("refresh_token")
	if err != nil || refreshToken == "" {
		var req struct {
			RefreshToken string `json:"refreshToken"`
		}
		if bindErr := c.ShouldBindJSON(&req); bindErr == nil && req.RefreshToken != "" {
			refreshToken = req.RefreshToken
		}
	}
	if refreshToken == "" {
		FlushAuthCookies(c, h.isProduction)
		response.UnauthorizedRequireLogin(c, "AUTH_SESSION_EXPIRED", "Refresh token is missing or expired")
		return
	}

	// ── 1. Check Redis Cache for Grace Period ──
	hSum := sha256.Sum256([]byte(refreshToken))
	cacheKey := "grace_period:refresh:" + hex.EncodeToString(hSum[:16])
	cachedVal, err := h.cacheProvider.Get(c.Request.Context(), cacheKey)
	if err == nil && cachedVal != "" {
		var cached GracePeriodPayload
		if json.Unmarshal([]byte(cachedVal), &cached) == nil {
			// Serve the rotated tokens directly from the grace cache (race condition winner's result)
			secure := ShouldSecureCookie(c, h.isProduction)
			c.SetCookie("refresh_token", cached.RefreshToken, 7*24*3600, "/", "", secure, true)
			if cached.AccessToken != "" {
				c.SetCookie("access_token", cached.AccessToken, int(cached.ExpiresIn), "/", "", secure, true)
			}
			response.Success(c, http.StatusOK, apidto.TokenResponse{
				AccessToken:  cached.AccessToken,
				ExpiresIn:    cached.ExpiresIn,
				RefreshToken: cached.RefreshToken,
			})
			return
		}
	}

	// ── 2. Perform actual rotation with Hydra ──
	res, err := h.refreshService.Refresh(c.Request.Context(), refreshToken)
	if err != nil {
		FlushAuthCookies(c, h.isProduction)
		response.UnauthorizedRequireLogin(c, "AUTH_SESSION_EXPIRED", "Session has expired. Please log in again.")
		return
	}

	// ── 3. Store new tokens in Redis under the old token key (TTL 10s) ──
	payloadBytes, jsonErr := json.Marshal(GracePeriodPayload{
		AccessToken:  res.AccessToken,
		RefreshToken: res.RefreshToken,
		ExpiresIn:    res.ExpiresIn,
	})
	if jsonErr == nil {
		// 10-second grace window to absorb concurrent requests
		_ = h.cacheProvider.Set(c.Request.Context(), cacheKey, string(payloadBytes), 10*time.Second)
	}

	// ── 4. Set cookies and return response ──
	secure := ShouldSecureCookie(c, h.isProduction)
	c.SetCookie("refresh_token", res.RefreshToken, 7*24*3600, "/", "", secure, true)
	if res.AccessToken != "" {
		c.SetCookie("access_token", res.AccessToken, int(res.ExpiresIn), "/", "", secure, true)
	}

	response.Success(c, http.StatusOK, apidto.TokenResponse{
		AccessToken:  res.AccessToken,
		ExpiresIn:    res.ExpiresIn,
		RefreshToken: res.RefreshToken,
	})
}
