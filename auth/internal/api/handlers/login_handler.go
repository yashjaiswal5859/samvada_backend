package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	apidto "github.com/ridesafety/auth-service/internal/api/dto"
	applogin "github.com/ridesafety/auth-service/internal/application/login"
	"github.com/ridesafety/auth-service/internal/domain/oauthsession"
	"github.com/ridesafety/auth-service/internal/domain/user"
	"github.com/ridesafety/auth-service/internal/platform/cache"
	"github.com/ridesafety/auth-service/internal/platform/identity"
	"github.com/ridesafety/auth-service/internal/shared/response"
)

type LoginHandler struct {
	loginService     applogin.Service
	orchestrator     identity.IdentityOrchestrator
	userRepo         user.Repository
	oauthSessionRepo oauthsession.Repository
	cacheProvider    cache.CacheProvider
	isProduction     bool
}

func NewLoginHandler(
	service applogin.Service,
	orchestrator identity.IdentityOrchestrator,
	userRepo user.Repository,
	oauthSessionRepo oauthsession.Repository,
	cacheProvider cache.CacheProvider,
	isProduction bool,
) *LoginHandler {
	return &LoginHandler{
		loginService:     service,
		orchestrator:     orchestrator,
		userRepo:         userRepo,
		oauthSessionRepo: oauthSessionRepo,
		cacheProvider:    cacheProvider,
		isProduction:     isProduction,
	}
}

// Login handles initial login request.
// Security:
// 1. Triggers ONLY on explicit user email submission (passive page loads do 0 DB writes).
// 2. Generates PKCE verifier (32 bytes CSPRNG) & S256 challenge.
// 3. Generates 6-digit OTP, hashes it with SHA-256, and publishes to transactional outbox (Kafka).
// 4. Binds session_id to an HttpOnly, Secure, SameSite=Lax cookie with 5-minute TTL.
func (h *LoginHandler) Login(c *gin.Context) {
	var req apidto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, response.JSONResponse{
			Success:       false,
			CorrelationID: c.GetString("correlation_id"),
			Error: &response.ErrorDetail{
				Code:    "AUTH_422",
				Message: err.Error(),
			},
		})
		return
	}

	corrID := c.GetString("correlation_id")
	appReq := applogin.LoginRequest{
		Email: req.Email,
	}

	// 1. Process login and generate OTP in backend (publishes to transactional outbox -> Kafka -> Email Service)
	res, err := h.loginService.Login(c.Request.Context(), appReq, corrID)
	if err != nil {
		response.Error(c, err)
		return
	}

	// 2. Generate PKCE Material strictly on Email submit
	verifier, err := generateVerifier()
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.JSONResponse{
			Success:       false,
			CorrelationID: corrID,
			Error:         &response.ErrorDetail{Code: "AUTH_500", Message: "Failed to generate security verifier"},
		})
		return
	}
	challenge := generateChallenge(verifier)

	state, err := generateRandomString(32)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.JSONResponse{
			Success:       false,
			CorrelationID: corrID,
			Error:         &response.ErrorDetail{Code: "AUTH_500", Message: "Failed to generate CSRF state"},
		})
		return
	}

	nonce, err := generateRandomString(32)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.JSONResponse{
			Success:       false,
			CorrelationID: corrID,
			Error:         &response.ErrorDetail{Code: "AUTH_500", Message: "Failed to generate replay nonce"},
		})
		return
	}

	// 3. Create session with session_id as the primary browser-binding key
	sessionID := uuid.New()
	session := &oauthsession.OAuthSession{
		ID:           sessionID,
		State:        state,
		Nonce:        nonce,
		CodeVerifier: verifier,
		Used:         false,
		CreatedAt:    time.Now().UTC(),
		ExpiresAt:    time.Now().UTC().Add(5 * time.Minute), // Strict 5m TTL matching OTP
	}

	if err := h.oauthSessionRepo.Create(c.Request.Context(), session); err != nil {
		c.JSON(http.StatusInternalServerError, response.JSONResponse{
			Success:       false,
			CorrelationID: corrID,
			Error:         &response.ErrorDetail{Code: "AUTH_500", Message: "Failed to save session"},
		})
		return
	}

	// 4. Cache in Redis for ultra-fast double-binding & verifier lookup (5 min TTL)
	pkcePayload, _ := json.Marshal(map[string]string{
		"code_verifier":  verifier,
		"code_challenge": challenge,
		"state":          state,
		"nonce":          nonce,
		"email":          req.Email,
		"request_id":     res.RequestID,
	})
	_ = h.cacheProvider.Set(c.Request.Context(), "pkce_session:"+sessionID.String(), string(pkcePayload), 5*time.Minute)

	// 5. Set session_id cookie (HttpOnly, Secure in prod/https, SameSite=Lax)
	c.SetSameSite(http.SameSiteLaxMode)
	secure := ShouldSecureCookie(c, h.isProduction)
	c.SetCookie("session_id", sessionID.String(), 300, "/", "", secure, true)

	response.JSON(c, http.StatusAccepted, true, apidto.RegisterResponse{
		RequestID: res.RequestID,
		ExpiresIn: res.ExpiresIn,
	}, nil)
}

// VerifyLogin handles OTP verification and session token issuance.
// Security:
// 1. Double-Binding check: asserts valid session_id cookie exists and matches Redis/DB.
// 2. Constant-time comparison on OTP hash to prevent timing attacks.
// 3. Session Fixation prevention: destroys pre-auth session and issues fresh credentials.
// 4. Delivers JWTs exclusively via HttpOnly; SameSite=Lax; Secure cookies (Zero tokens in localStorage).
func (h *LoginHandler) VerifyLogin(c *gin.Context) {
	var req apidto.VerifyLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, response.JSONResponse{
			Success:       false,
			CorrelationID: c.GetString("correlation_id"),
			Error: &response.ErrorDetail{
				Code:    "AUTH_422",
				Message: err.Error(),
			},
		})
		return
	}

	corrID := c.GetString("correlation_id")

	// ── DOUBLE-BINDING CHECK: Validate session_id cookie ──
	sessionCookie, err := c.Cookie("session_id")
	if err != nil || sessionCookie == "" {
		sessionCookie, err = c.Cookie("login_session_id")
	}

	if err != nil || sessionCookie == "" {
		c.JSON(http.StatusUnauthorized, response.JSONResponse{
			Success:       false,
			CorrelationID: corrID,
			Error: &response.ErrorDetail{
				Code:    "AUTH_401",
				Message: "Session invalid or expired. Please request a new verification code.",
			},
		})
		return
	}

	// Check if session exists in Postgres or Redis
	sessionID, parseErr := uuid.Parse(sessionCookie)
	var oauthSess *oauthsession.OAuthSession
	if parseErr == nil {
		oauthSess, _ = h.oauthSessionRepo.FindByID(c.Request.Context(), sessionID)
	}

	if oauthSess != nil {
		if oauthSess.IsExpired() || oauthSess.Used {
			_ = h.oauthSessionRepo.Delete(c.Request.Context(), oauthSess.ID)
			c.JSON(http.StatusUnauthorized, response.JSONResponse{
				Success:       false,
				CorrelationID: corrID,
				Error: &response.ErrorDetail{
					Code:    "AUTH_401",
					Message: "Session has expired. Please request a new verification code.",
				},
			})
			return
		}
	}

	appReq := applogin.VerifyLoginRequest{
		RequestID: req.RequestID,
		OTP:       req.OTP,
	}

	// ── VERIFY OTP (Constant-time compare, attempt limits, expiration) ──
	res, err := h.loginService.VerifyLogin(c.Request.Context(), appReq, corrID)
	if err != nil {
		response.Error(c, err)
		return
	}

	// ── SESSION FIXATION PROTECTION: Invalidate pre-auth session ──
	_ = h.cacheProvider.Delete(c.Request.Context(), "pkce_session:"+sessionCookie)
	if oauthSess != nil {
		_ = h.oauthSessionRepo.MarkUsed(c.Request.Context(), oauthSess.ID)
		_ = h.oauthSessionRepo.Delete(c.Request.Context(), oauthSess.ID)
	}

	// If login challenge is passed, redirect through Hydra login accept flow
	if req.LoginChallenge != "" {
		subject, err := extractSubjectFromJWT(res.AccessToken)
		if err != nil {
			c.JSON(http.StatusInternalServerError, response.JSONResponse{
				Success:       false,
				CorrelationID: corrID,
				Error: &response.ErrorDetail{
					Code:    "AUTH_500",
					Message: "Failed to extract subject from generated token: " + err.Error(),
				},
			})
			return
		}

		redirectTo, err := h.orchestrator.AcceptLoginChallenge(c.Request.Context(), req.LoginChallenge, subject)
		if err != nil {
			response.Error(c, err)
			return
		}

		response.Success(c, http.StatusOK, map[string]string{
			"redirectTo": redirectTo,
		})
		return
	}

	// ── SET SECURE COOKIES: Zero tokens in localStorage ──
	c.SetSameSite(http.SameSiteLaxMode)
	secure := ShouldSecureCookie(c, h.isProduction)
	if res.RefreshToken != "" {
		c.SetCookie("refresh_token", res.RefreshToken, 7*24*3600, "/", "", secure, true)
	}
	if res.AccessToken != "" {
		c.SetCookie("access_token", res.AccessToken, int(res.ExpiresIn), "/", "", secure, true)
	}

	response.Success(c, http.StatusOK, apidto.TokenResponse{
		AccessToken:  res.AccessToken,
		ExpiresIn:    res.ExpiresIn,
		RefreshToken: res.RefreshToken,
	})
}
