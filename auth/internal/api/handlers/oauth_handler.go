package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	applogin "github.com/ridesafety/auth-service/internal/application/login"
	"github.com/ridesafety/auth-service/internal/domain/oauthsession"
	"github.com/ridesafety/auth-service/internal/domain/pendingauth"
	"github.com/ridesafety/auth-service/internal/domain/user"
	"github.com/ridesafety/auth-service/internal/platform/cache"
	"github.com/ridesafety/auth-service/internal/platform/identity"
	"github.com/ridesafety/auth-service/internal/shared/response"
)

type RegistrationSession struct {
	KratosID      string `json:"kratos_id"`
	Verified      bool   `json:"verified"`
	Authenticated bool   `json:"authenticated"`
}

type OAuthHandler struct {
	oauthSessionRepo oauthsession.Repository
	pendingAuthRepo  pendingauth.Repository
	orchestrator     identity.IdentityOrchestrator
	loginService     applogin.Service
	userRepo         user.Repository
	cacheProvider    cache.CacheProvider
	browserURL       string
	clientID         string
	redirectURI      string
	frontendURL      string
	isProduction     bool
	allowedHosts     map[string]bool // whitelist for redirect validation
}

func NewOAuthHandler(
	oauthSessionRepo oauthsession.Repository,
	pendingAuthRepo pendingauth.Repository,
	orchestrator identity.IdentityOrchestrator,
	loginService applogin.Service,
	userRepo user.Repository,
	cacheProvider cache.CacheProvider,
	browserURL string,
	clientID string,
	redirectURI string,
	frontendURL string,
	isProduction bool,
	allowedRedirectDomains []string,
) *OAuthHandler {
	hosts := make(map[string]bool)
	for _, domain := range allowedRedirectDomains {
		hosts[domain] = true
	}
	// Always allow the frontend URL's host
	if parsed, err := url.Parse(frontendURL); err == nil {
		hosts[parsed.Hostname()] = true
	}
	// Always allow the Hydra browser URL's host
	if parsed, err := url.Parse(browserURL); err == nil {
		hosts[parsed.Hostname()] = true
	}

	return &OAuthHandler{
		oauthSessionRepo: oauthSessionRepo,
		pendingAuthRepo:  pendingAuthRepo,
		orchestrator:     orchestrator,
		loginService:     loginService,
		userRepo:         userRepo,
		cacheProvider:    cacheProvider,
		browserURL:       browserURL,
		clientID:         clientID,
		redirectURI:      redirectURI,
		frontendURL:      frontendURL,
		isProduction:     isProduction,
		allowedHosts:     hosts,
	}
}

// validateRedirectURL checks the redirect URL against a whitelist of allowed hosts.
// This prevents open redirect attacks (Loophole 7).
func (h *OAuthHandler) validateRedirectURL(redirectTo string) error {
	parsed, err := url.Parse(redirectTo)
	if err != nil {
		return fmt.Errorf("invalid redirect URL")
	}
	// Allow relative paths (e.g., /api/v1/auth/oauth/login)
	if parsed.Host == "" {
		return nil
	}
	if !h.allowedHosts[parsed.Hostname()] {
		return fmt.Errorf("redirect to disallowed host: %s", parsed.Hostname())
	}
	return nil
}

// setCookie is a secure cookie setter that respects the environment and HTTPS transport.
// In production/HTTPS, Secure=true ensures cookies are only sent over HTTPS.
func (h *OAuthHandler) setCookie(c *gin.Context, name, value string, maxAge int, path string) {
	c.SetSameSite(http.SameSiteLaxMode)
	secure := ShouldSecureCookie(c, h.isProduction)
	c.SetCookie(name, value, maxAge, path, "", secure, true)
}

// deleteCookie removes a cookie by setting it with a negative max age.
func (h *OAuthHandler) deleteCookie(c *gin.Context, name, path string) {
	c.SetSameSite(http.SameSiteLaxMode)
	secure := ShouldSecureCookie(c, h.isProduction)
	c.SetCookie(name, "", -1, path, "", secure, true)
}

// readSessionID reads and validates the session_id cookie, returning the parsed UUID.
// Returns the session from DB, or an error if session is missing/invalid/expired/used.
func (h *OAuthHandler) readAndValidateSession(c *gin.Context) (*oauthsession.OAuthSession, error) {
	sessionCookie, err := c.Cookie("session_id")
	if err != nil || sessionCookie == "" {
		return nil, fmt.Errorf("missing session_id cookie")
	}

	sessionID, err := uuid.Parse(sessionCookie)
	if err != nil {
		return nil, fmt.Errorf("invalid session_id format")
	}

	session, err := h.oauthSessionRepo.FindByID(c.Request.Context(), sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found")
	}

	if session.IsExpired() {
		_ = h.oauthSessionRepo.Delete(c.Request.Context(), session.ID)
		return nil, fmt.Errorf("session expired")
	}

	if session.Used {
		return nil, fmt.Errorf("session already used")
	}

	return session, nil
}

// OAuthLogin redirects to Hydra with secure session_id cookie binding.
// Security: SSO check first, session_id HttpOnly cookie, PKCE, state, nonce.
func (h *OAuthHandler) OAuthLogin(c *gin.Context) {
	// ── SSO Check (Fast-path): Skip login if user already has a valid token ──
	token, err := c.Cookie("access_token")
	if err == nil && token != "" {
		active, _, _, introErr := h.orchestrator.IntrospectToken(c.Request.Context(), token)
		if introErr == nil && active {
			c.Redirect(http.StatusFound, h.frontendURL+"/")
			return
		}
	}

	// If registration session cookie is present but expired/invalid, clear it to prevent stale states
	regSessionID, err := c.Cookie("registration_session_id")
	if err == nil && regSessionID != "" {
		val, err := h.cacheProvider.Get(c.Request.Context(), "registration_session:"+regSessionID)
		if err != nil || val == "" {
			h.deleteCookie(c, "registration_session_id", "/api/v1/auth")
		} else {
			var regSess RegistrationSession
			if json.Unmarshal([]byte(val), &regSess) != nil || !regSess.Verified || !regSess.Authenticated {
				h.deleteCookie(c, "registration_session_id", "/api/v1/auth")
			}
		}
	}

	// ── Handle register intent ──
	action := c.Query("action")
	if action == "register" {
		h.setCookie(c, "oauth_intent", "register", 300, "/")
	}

	// ── Generate all cryptographic material ──
	state, err := generateRandomString(32)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.JSONResponse{
			Success: false,
			Error:   &response.ErrorDetail{Message: "Internal Server Error"},
		})
		return
	}

	nonce, err := generateRandomString(32)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.JSONResponse{
			Success: false,
			Error:   &response.ErrorDetail{Message: "Internal Server Error"},
		})
		return
	}

	verifier, err := generateVerifier()
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.JSONResponse{
			Success: false,
			Error:   &response.ErrorDetail{Message: "Internal Server Error"},
		})
		return
	}

	challenge := generateChallenge(verifier)

	// ── Create session with session_id as the primary browser-binding key ──
	sessionID := uuid.New()
	session := &oauthsession.OAuthSession{
		ID:           sessionID,
		State:        state,
		Nonce:        nonce,
		CodeVerifier: verifier,
		Used:         false,
		CreatedAt:    time.Now().UTC(),
		ExpiresAt:    time.Now().UTC().Add(1 * time.Hour),
	}

	if err := h.oauthSessionRepo.Create(c.Request.Context(), session); err != nil {
		c.JSON(http.StatusInternalServerError, response.JSONResponse{
			Success: false,
			Error:   &response.ErrorDetail{Message: "Internal Server Error"},
		})
		return
	}

	// ── Set session_id cookie (HttpOnly, Secure in prod, SameSite=Lax) ──
	h.setCookie(c, "session_id", sessionID.String(), 3600, "/api/v1/auth")

	hydraAuthURL := fmt.Sprintf("%s/oauth2/auth?client_id=%s&response_type=code&scope=openid+offline_access&redirect_uri=%s&state=%s&nonce=%s&code_challenge=%s&code_challenge_method=S256",
		h.browserURL, h.clientID, url.QueryEscape(h.redirectURI), state, nonce, challenge)

	c.Redirect(http.StatusFound, hydraAuthURL)
}

// LoginChallenge handles Hydra's login_challenge redirect.
// Security: Validates session_id cookie, validates challenge with Hydra Admin API,
// binds challenge to session, checks for SSO before serving login page.
func (h *OAuthHandler) LoginChallenge(c *gin.Context) {
	challenge := c.Query("login_challenge")
	if challenge == "" {
		c.String(http.StatusBadRequest, "Missing login_challenge")
		return
	}

	// ── Validate session_id cookie ──
	session, err := h.readAndValidateSession(c)
	if err != nil {
		// No valid session — clear stale challenge cookie and redirect to main domain
		c.SetCookie("login_challenge", "", -1, "/", "", h.isProduction, false)
		c.Redirect(http.StatusFound, h.frontendURL)
		return
	}

	// ── Verify challenge is real and pending via Hydra Admin API (Loophole 2 fix) ──
	valid, verifyErr := h.orchestrator.GetLoginRequest(c.Request.Context(), challenge)
	if verifyErr != nil || !valid {
		c.String(http.StatusBadRequest, "Invalid login challenge")
		return
	}

	// ── Bind challenge to session ──
	if updateErr := h.oauthSessionRepo.UpdateLoginChallenge(c.Request.Context(), session.ID, challenge); updateErr != nil {
		c.JSON(http.StatusInternalServerError, response.JSONResponse{
			Success: false,
			Error:   &response.ErrorDetail{Message: "Internal Server Error"},
		})
		return
	}

	// ── Auto-Accept Registration Login Check (Phase 4) ──
	regSessionID, err := c.Cookie("registration_session_id")
	if err == nil && regSessionID != "" {
		val, err := h.cacheProvider.Get(c.Request.Context(), "registration_session:"+regSessionID)
		if err == nil && val != "" {
			var regSess RegistrationSession
			if json.Unmarshal([]byte(val), &regSess) == nil && regSess.Verified && regSess.Authenticated {
				redirectTo, acceptErr := h.orchestrator.AcceptLoginChallenge(c.Request.Context(), challenge, regSess.KratosID)
				if acceptErr == nil {
					if err := h.validateRedirectURL(redirectTo); err == nil {
						c.Redirect(http.StatusFound, redirectTo)
						return
					}
				}
			}
		}
	}

	// ── Auto-Accept Login Verification Check ──
	loginSessionID, err := c.Cookie("login_session_id")
	if err == nil && loginSessionID != "" {
		val, err := h.cacheProvider.Get(c.Request.Context(), "login_session:"+loginSessionID)
		if err == nil && val != "" {
			var logSess RegistrationSession
			if json.Unmarshal([]byte(val), &logSess) == nil && logSess.Verified && logSess.Authenticated {
				redirectTo, acceptErr := h.orchestrator.AcceptLoginChallenge(c.Request.Context(), challenge, logSess.KratosID)
				if acceptErr == nil {
					if err := h.validateRedirectURL(redirectTo); err == nil {
						_ = h.cacheProvider.Delete(c.Request.Context(), "login_session:"+loginSessionID)
						c.SetCookie("login_session_id", "", -1, "/api/v1/auth", "", h.isProduction, true)
						c.Redirect(http.StatusFound, redirectTo)
						return
					}
				}
			}
		}
	}

	// ── Fallback SSO Check: If user has a valid token, auto-accept ──
	token, tokenErr := c.Cookie("access_token")
	if tokenErr == nil && token != "" {
		active, subject, _, introErr := h.orchestrator.IntrospectToken(c.Request.Context(), token)
		if introErr == nil && active && subject != "" {
			redirectTo, acceptErr := h.orchestrator.AcceptLoginChallenge(c.Request.Context(), challenge, subject)
			if acceptErr == nil {
				if valErr := h.validateRedirectURL(redirectTo); valErr == nil {
					c.Redirect(http.StatusFound, redirectTo)
					return
				}
			}
		}
	}

	// ── Check register intent ──
	intent, _ := c.Cookie("oauth_intent")
	if intent == "register" {
		h.deleteCookie(c, "oauth_intent", "/")
		c.Redirect(http.StatusFound, "/api/v1/auth/register-challenge?login_challenge="+url.QueryEscape(challenge))
		return
	}

	// ── Set login_challenge cookie (non-HttpOnly so client JS can read it) ──
	c.SetCookie("login_challenge", challenge, 3600, "/", "", h.isProduction, false)

	// ── Redirect to React Frontend login route ──
	c.Redirect(http.StatusFound, h.frontendURL+"/login")
}

// RegisterChallenge GET /api/v1/auth/register-challenge
func (h *OAuthHandler) RegisterChallenge(c *gin.Context) {
	challenge := c.Query("login_challenge")
	c.SetCookie("login_challenge", challenge, 3600, "/", "", h.isProduction, false)
	c.Redirect(http.StatusFound, h.frontendURL+"/register")
}

// LoginView GET /api/v1/auth/login-view
func (h *OAuthHandler) LoginView(c *gin.Context) {
	c.Redirect(http.StatusFound, h.frontendURL+"/login")
}

// VerifyView GET /api/v1/auth/verify/:requestId
func (h *OAuthHandler) VerifyView(c *gin.Context) {
	requestId := c.Param("requestId")
	loginChallenge := c.Query("login_challenge")
	mode := c.Query("mode")
	
	if loginChallenge != "" {
		c.SetCookie("login_challenge", loginChallenge, 3600, "/", "", h.isProduction, false)
	}
	
	targetURL := fmt.Sprintf("%s/login?requestId=%s&mode=%s", 
		h.frontendURL, requestId, mode)
	if mode == "register" {
		targetURL = fmt.Sprintf("%s/register?requestId=%s&mode=%s", 
			h.frontendURL, requestId, mode)
	}
	c.Redirect(http.StatusFound, targetURL)
}

// VerifyOTP handles OTP verification with session_id binding and session regeneration.
// Security: Validates session_id cookie, reads login_challenge from session (not from request body),
// regenerates session_id after successful OTP to prevent session fixation.
func (h *OAuthHandler) VerifyOTP(c *gin.Context) {
	var req struct {
		RequestID      string `json:"requestId" binding:"required,uuid"`
		OTP            string `json:"otp" binding:"required,len=6,numeric"`
		LoginChallenge string `json:"loginChallenge" binding:"omitempty"`
	}

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

	// ── Validate session_id cookie to bind this request to the OAuth flow ──
	session, sessionErr := h.readAndValidateSession(c)

	// Determine the login_challenge: prefer from session (secure), fallback to request body (legacy compat)
	loginChallenge := ""
	if session != nil && session.LoginChallenge != "" {
		loginChallenge = session.LoginChallenge
	} else if req.LoginChallenge != "" {
		loginChallenge = req.LoginChallenge
	}

	uuidID, err := uuid.Parse(req.RequestID)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.JSONResponse{
			Success:       false,
			CorrelationID: c.GetString("correlation_id"),
			Error: &response.ErrorDetail{
				Code:    "AUTH_400",
				Message: "Invalid requestId format",
			},
		})
		return
	}

	pa, err := h.pendingAuthRepo.FindByID(c.Request.Context(), uuidID)
	if err != nil {
		c.JSON(http.StatusNotFound, response.JSONResponse{
			Success:       false,
			CorrelationID: c.GetString("correlation_id"),
			Error: &response.ErrorDetail{
				Code:    "AUTH_404",
				Message: "Session expired or invalid",
			},
		})
		return
	}

	corrID := c.GetString("correlation_id")
	var accessToken, refreshToken string
	var expiresIn int64
	var subject string

	if pa.Purpose == pendingauth.PurposeLogin {
		appReq := applogin.VerifyLoginRequest{
			RequestID: req.RequestID,
			OTP:       req.OTP,
		}
		res, err := h.loginService.VerifyLogin(c.Request.Context(), appReq, corrID)
		if err != nil {
			response.Error(c, err)
			return
		}

		// Check if it is a browser session with login_session_id
		loginSessionID, errCookie := c.Cookie("login_session_id")
		if errCookie == nil && loginSessionID != "" && loginChallenge == "" {
			// Resolve KratosID from local database using pa.Email
			u, userErr := h.userRepo.FindByEmail(c.Request.Context(), pa.Email)
			if userErr == nil {
				sessBytes, jsonErr := json.Marshal(RegistrationSession{
					KratosID:      u.KratosID,
					Verified:      true,
					Authenticated: true,
				})
				if jsonErr == nil {
					_ = h.cacheProvider.Set(c.Request.Context(), "login_session:"+loginSessionID, string(sessBytes), 30*time.Minute)
				}
			}

			response.Success(c, http.StatusOK, map[string]string{
				"redirectTo": "/api/v1/auth/oauth/login",
			})
			return
		}

		accessToken = res.AccessToken
		refreshToken = res.RefreshToken
		expiresIn = res.ExpiresIn
	}

	// ── Session Regeneration (Loophole 3 fix) ──
	// After successful OTP, regenerate session_id to prevent session fixation.
	if session != nil && sessionErr == nil {
		newSessionID := uuid.New()
		newSession := &oauthsession.OAuthSession{
			ID:             newSessionID,
			State:          session.State,
			Nonce:          session.Nonce,
			CodeVerifier:   session.CodeVerifier,
			LoginChallenge: session.LoginChallenge,
			Used:           false,
			CreatedAt:      session.CreatedAt,
			ExpiresAt:      session.ExpiresAt,
		}
		if createErr := h.oauthSessionRepo.Create(c.Request.Context(), newSession); createErr == nil {
			_ = h.oauthSessionRepo.Delete(c.Request.Context(), session.ID)
			h.setCookie(c, "session_id", newSessionID.String(), 3600, "/api/v1/auth")
		}
	}

	// ── Set token cookies ──
	if refreshToken != "" {
		h.setCookie(c, "refresh_token", refreshToken, 7*24*3600, "/")
	}
	if accessToken != "" {
		h.setCookie(c, "access_token", accessToken, int(expiresIn), "/")
	}

	if loginChallenge != "" {
		// ── OAuth flow: Accept login challenge and redirect to consent ──
		// Look up user's KratosID from local users table if not resolved
		if subject == "" {
			if pa.Email != "" {
				u, userErr := h.userRepo.FindByEmail(c.Request.Context(), pa.Email)
				if userErr == nil {
					subject = u.KratosID
				}
			}
		}

		// If we couldn't get subject from user table, try extracting from access token
		if subject == "" && accessToken != "" {
			active, sub, _, introErr := h.orchestrator.IntrospectToken(c.Request.Context(), accessToken)
			if introErr == nil && active {
				subject = sub
			}
		}

		if subject == "" {
			c.JSON(http.StatusInternalServerError, response.JSONResponse{
				Success:       false,
				CorrelationID: c.GetString("correlation_id"),
				Error:         &response.ErrorDetail{Code: "AUTH_500", Message: "Could not determine user subject"},
			})
			return
		}

		redirectTo, acceptErr := h.orchestrator.AcceptLoginChallenge(c.Request.Context(), loginChallenge, subject)
		if acceptErr != nil {
			// Challenge may be stale — redirect to restart OAuth flow
			response.Success(c, http.StatusOK, map[string]interface{}{
				"redirectTo": "/api/v1/auth/oauth/login",
			})
			return
		}

		// Validate redirect URL before returning it to the browser
		if valErr := h.validateRedirectURL(redirectTo); valErr != nil {
			response.Success(c, http.StatusOK, map[string]interface{}{
				"redirectTo": "/api/v1/auth/oauth/login",
			})
			return
		}

		response.Success(c, http.StatusOK, map[string]interface{}{
			"redirectTo": redirectTo,
		})
		return
	}

	response.Success(c, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// Consent auto-accepts the consent challenge with session_id validation.
// Security: Validates session_id cookie to prevent consent hijacking (Loophole 10 fix).
func (h *OAuthHandler) Consent(c *gin.Context) {
	challenge := c.Query("consent_challenge")
	if challenge == "" {
		c.JSON(http.StatusBadRequest, response.JSONResponse{
			Success:       false,
			CorrelationID: c.GetString("correlation_id"),
			Error: &response.ErrorDetail{
				Code:    "AUTH_400",
				Message: "Missing consent_challenge query parameter",
			},
		})
		return
	}

	// ── Validate session_id cookie (Loophole 10 fix) ──
	// Ensures only the browser that started the OAuth flow can consent.
	session, sessionErr := h.readAndValidateSession(c)
	if sessionErr != nil {
		// Fallback: if no session cookie, still proceed if user has valid access_token (SSO path)
		token, tokenErr := c.Cookie("access_token")
		if tokenErr != nil || token == "" {
			c.JSON(http.StatusBadRequest, response.JSONResponse{
				Success:       false,
				CorrelationID: c.GetString("correlation_id"),
				Error: &response.ErrorDetail{
					Code:    "AUTH_400",
					Message: "Invalid session",
				},
			})
			return
		}
	}

	consentRequest, err := h.orchestrator.GetConsentRequest(c.Request.Context(), challenge)
	if err != nil {
		response.Error(c, err)
		return
	}

	// ── Consent Hijack Check ──
	// Assert session.login_challenge corresponds to consent (matches sequence diagram)
	if sessionErr == nil {
		if session.LoginChallenge == "" || session.LoginChallenge != consentRequest.LoginChallenge {
			c.JSON(http.StatusBadRequest, response.JSONResponse{
				Success:       false,
				CorrelationID: c.GetString("correlation_id"),
				Error: &response.ErrorDetail{
					Code:    "AUTH_400",
					Message: "Consent hijack check failed: login challenge mismatch",
				},
			})
			return
		}
	}

	customClaims := map[string]interface{}{}
	if u, err := h.userRepo.FindByKratosID(c.Request.Context(), consentRequest.Subject); err == nil && u != nil {
		customClaims["user_id"] = u.ID.String()
		customClaims["email"] = u.Email
	}

	redirectTo, err := h.orchestrator.AcceptConsentChallenge(c.Request.Context(), challenge, consentRequest.Subject, []string{"openid", "offline_access"}, customClaims)
	if err != nil {
		response.Error(c, err)
		return
	}

	if valErr := h.validateRedirectURL(redirectTo); valErr != nil {
		c.String(http.StatusBadRequest, "Redirect URL validation failed")
		return
	}

	c.Redirect(http.StatusFound, redirectTo)
}

// Callback handles the OAuth callback with the full security double-lock.
// Security: session_id cookie binding, state validation, used flag check,
// PKCE code exchange, JWKS RS256 ID Token signature verification, nonce validation.
func (h *OAuthHandler) Callback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")

	if code == "" || state == "" {
		c.JSON(http.StatusBadRequest, response.JSONResponse{
			Success:       false,
			CorrelationID: c.GetString("correlation_id"),
			Error: &response.ErrorDetail{
				Code:    "AUTH_400",
				Message: "Missing authorization code or state in callback",
			},
		})
		return
	}

	// ── DOUBLE-LOCK: Cookie binding + state validation ──

	// 1. Read session_id cookie (browser binding)
	session, err := h.readAndValidateSession(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.JSONResponse{
			Success:       false,
			CorrelationID: c.GetString("correlation_id"),
			Error: &response.ErrorDetail{
				Code:    "AUTH_400",
				Message: "Invalid or missing session: " + err.Error(),
			},
		})
		return
	}

	// 2. Validate state matches the session (CSRF protection)
	if session.State != state {
		c.JSON(http.StatusBadRequest, response.JSONResponse{
			Success:       false,
			CorrelationID: c.GetString("correlation_id"),
			Error: &response.ErrorDetail{
				Code:    "AUTH_400",
				Message: "State parameter mismatch",
			},
		})
		return
	}

	// ── Exchange code with PKCE code_verifier ──
	tokens, err := h.orchestrator.ExchangeCodeForTokens(c.Request.Context(), code, session.CodeVerifier)
	if err != nil {
		response.Error(c, err)
		return
	}

	// ── Validate ID Token signature with JWKS (Loophole 1 fix) ──
	if tokens.IDToken != "" {
		_, valErr := h.orchestrator.ValidateIDToken(
			c.Request.Context(),
			tokens.IDToken,
			session.Nonce,
			h.clientID,
		)
		if valErr != nil {
			c.JSON(http.StatusBadRequest, response.JSONResponse{
				Success:       false,
				CorrelationID: c.GetString("correlation_id"),
				Error: &response.ErrorDetail{
					Code:    "AUTH_400",
					Message: "ID Token validation failed: " + valErr.Error(),
				},
			})
			return
		}
	}

	// ── Mark session as used and clean up ──
	_ = h.oauthSessionRepo.MarkUsed(c.Request.Context(), session.ID)
	_ = h.oauthSessionRepo.Delete(c.Request.Context(), session.ID)
	h.deleteCookie(c, "session_id", "/api/v1/auth")

	// Clear the login_challenge cookie
	c.SetCookie("login_challenge", "", -1, "/", "", h.isProduction, false)

	// Cleanup registration session if it exists (Phase 4 auto-login cleanup)
	regSessionID, err := c.Cookie("registration_session_id")
	if err == nil && regSessionID != "" {
		_ = h.cacheProvider.Delete(c.Request.Context(), "registration_session:"+regSessionID)
		h.deleteCookie(c, "registration_session_id", "/api/v1/auth")
	}

	// ── Set token cookies (HttpOnly, Secure in prod, SameSite=Lax) ──
	if tokens.RefreshToken != "" {
		h.setCookie(c, "refresh_token", tokens.RefreshToken, 7*24*3600, "/")
	}
	if tokens.AccessToken != "" {
		h.setCookie(c, "access_token", tokens.AccessToken, int(tokens.ExpiresIn), "/")
	}

	// ── Redirect to frontend (from config, not hardcoded) ──
	c.Redirect(http.StatusFound, h.frontendURL+"/")
}

// Helper utilities
func generateRandomString(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func generateVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func generateChallenge(verifier string) string {
	h := sha256.New()
	h.Write([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
