package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/ridesafety/auth-service/internal/domain/events"
	"github.com/ridesafety/auth-service/internal/domain/user"
	"github.com/ridesafety/auth-service/internal/platform/eventbus"
	"github.com/ridesafety/auth-service/internal/platform/identity"
	"github.com/ridesafety/auth-service/internal/shared/constants"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"gorm.io/gorm"
)

type GoogleHandler struct {
	oauthConfig  *oauth2.Config
	orchestrator identity.IdentityOrchestrator
	userRepo     user.Repository
	db           *gorm.DB
	eventBus     eventbus.EventBus
	isProduction bool
}

func NewGoogleHandler(
	clientID string,
	clientSecret string,
	redirectURL string,
	orchestrator identity.IdentityOrchestrator,
	userRepo user.Repository,
	db *gorm.DB,
	eventBus eventbus.EventBus,
	isProduction bool,
) *GoogleHandler {
	conf := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes: []string{
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		},
		Endpoint: google.Endpoint,
	}

	return &GoogleHandler{
		oauthConfig:  conf,
		orchestrator: orchestrator,
		userRepo:     userRepo,
		db:           db,
		eventBus:     eventBus,
		isProduction: isProduction,
	}
}

// generateStateOauthCookie generates a cryptographically random CSRF state token,
// stores it in an HttpOnly, short-lived cookie, and returns it for the OAuth URL.
func (h *GoogleHandler) generateStateOauthCookie(c *gin.Context) string {
	b := make([]byte, 32) // 256-bit random state
	if _, err := rand.Read(b); err != nil {
		// Fallback should never happen with crypto/rand, but fail closed
		c.String(http.StatusInternalServerError, "Failed to generate secure state")
		return ""
	}
	state := base64.URLEncoding.EncodeToString(b)
	// Short-lived cookie (10 minutes), HttpOnly, SameSite=Lax to prevent CSRF
	c.SetSameSite(http.SameSiteLaxMode)
	secure := ShouldSecureCookie(c, h.isProduction)
	c.SetCookie("oauthstate", state, 600, "/api/v1/auth/google", "", secure, true)
	return state
}

func (h *GoogleHandler) GoogleLogin(c *gin.Context) {
	oauthState := h.generateStateOauthCookie(c)
	if oauthState == "" {
		return // error already written
	}
	url := h.oauthConfig.AuthCodeURL(oauthState, oauth2.AccessTypeOffline)
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func (h *GoogleHandler) GoogleCallback(c *gin.Context) {
	// 1. CSRF state validation
	oauthState, err := c.Cookie("oauthstate")
	if err != nil || oauthState == "" {
		c.String(http.StatusBadRequest, "Missing or expired state cookie")
		return
	}

	// Immediately clear the state cookie (one-time use)
	c.SetSameSite(http.SameSiteLaxMode)
	secure := ShouldSecureCookie(c, h.isProduction)
	c.SetCookie("oauthstate", "", -1, "/api/v1/auth/google", "", secure, true)

	// Constant-time comparison to prevent timing attacks
	if !secureCompare(c.Query("state"), oauthState) {
		c.String(http.StatusBadRequest, "Invalid state parameter")
		return
	}

	// 2. Check for error response from Google
	if errParam := c.Query("error"); errParam != "" {
		c.String(http.StatusBadRequest, "Google authentication failed")
		return
	}

	// 3. Exchange authorization code for token
	code := c.Query("code")
	if code == "" {
		c.String(http.StatusBadRequest, "Missing authorization code")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	token, err := h.oauthConfig.Exchange(ctx, code)
	if err != nil {
		// Never leak internal error details to client
		c.String(http.StatusInternalServerError, "Authentication failed")
		return
	}

	// 4. Fetch user info from Google using the OAuth2 HTTP client (uses token automatically)
	client := h.oauthConfig.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to fetch user information")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.String(http.StatusInternalServerError, "Google returned an error")
		return
	}

	// Limit response body to 1MB to prevent memory exhaustion
	contents, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to read user information")
		return
	}

	var googleUser struct {
		Email         string `json:"email"`
		Name          string `json:"name"`
		EmailVerified bool   `json:"verified_email"`
	}
	if err := json.Unmarshal(contents, &googleUser); err != nil {
		c.String(http.StatusInternalServerError, "Failed to parse user information")
		return
	}

	// 5. Validate email
	email := strings.TrimSpace(strings.ToLower(googleUser.Email))
	if email == "" {
		c.String(http.StatusBadRequest, "Email not provided by Google")
		return
	}
	if !googleUser.EmailVerified {
		c.String(http.StatusForbidden, "Google email is not verified")
		return
	}

	name := strings.TrimSpace(googleUser.Name)

	// 6. Determine if user exists — find or auto-register
	existingUser, err := h.userRepo.FindByEmail(c.Request.Context(), email)
	var kratosID string

	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			kratosID, err = h.autoRegisterUser(c, email, name)
			if err != nil {
				return // error already written by autoRegisterUser
			}
		} else {
			c.String(http.StatusInternalServerError, "Authentication failed")
			return
		}
	} else {
		kratosID = existingUser.KratosID
	}

	customClaims := map[string]interface{}{
		"email": email,
	}
	if existingUser != nil {
		customClaims["user_id"] = existingUser.ID.String()
	} else if u, err := h.userRepo.FindByKratosID(c.Request.Context(), kratosID); err == nil && u != nil {
		customClaims["user_id"] = u.ID.String()
	}

	// 7. Generate tokens in Hydra
	tokens, err := h.orchestrator.GenerateTokens(c.Request.Context(), kratosID, customClaims)
	if err != nil {
		c.String(http.StatusInternalServerError, "Authentication failed")
		return
	}

	// Set SSO cookies — HttpOnly, scoped paths
	c.SetSameSite(http.SameSiteLaxMode)
	secureSSO := ShouldSecureCookie(c, h.isProduction)
	if tokens.RefreshToken != "" {
		c.SetCookie("refresh_token", tokens.RefreshToken, 7*24*3600, "/", "", secureSSO, true)
	}
	if tokens.AccessToken != "" {
		c.SetCookie("access_token", tokens.AccessToken, int(tokens.ExpiresIn), "/", "", secureSSO, true)
	}

	// Seamless SSO redirect
	c.Redirect(http.StatusFound, "/api/v1/auth/oauth/login")
}

// autoRegisterUser creates a new Kratos identity and local DB user for a Google sign-in.
// Returns the kratosID on success, or writes an error to the response and returns an error.
func (h *GoogleHandler) autoRegisterUser(c *gin.Context, email, name string) (string, error) {
	kratosID, err := h.orchestrator.CreateIdentity(c.Request.Context(), email)
	if err != nil {
		if errors.Is(err, identity.ErrIdentityAlreadyExists) {
			c.String(http.StatusConflict, "Account already exists. Please use email login.")
		} else {
			c.String(http.StatusInternalServerError, "Authentication failed")
		}
		return "", err
	}

	tx := h.db.Begin()
	if tx.Error != nil {
		_ = h.orchestrator.DeleteIdentity(c.Request.Context(), kratosID)
		c.String(http.StatusInternalServerError, "Authentication failed")
		return "", tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			_ = h.orchestrator.DeleteIdentity(c.Request.Context(), kratosID)
		}
	}()

	ctxTx := eventbus.WithTx(c.Request.Context(), tx)

	u := &user.User{
		ID:          uuid.New(),
		KratosID:    kratosID,
		Email:       email,
		DisplayName: name,
		IsActive:    true,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	if err := h.userRepo.Create(ctxTx, u); err != nil {
		tx.Rollback()
		_ = h.orchestrator.DeleteIdentity(c.Request.Context(), kratosID)
		c.String(http.StatusInternalServerError, "Authentication failed")
		return "", err
	}

	type userRegisteredPayload struct {
		KratosID    string `json:"kratos_id"`
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
	}
	evt, err := events.NewDomainEvent(
		events.UserRegistered,
		kratosID,
		"User",
		constants.EventVersion,
		"google-sso",
		userRegisteredPayload{KratosID: kratosID, Email: email, DisplayName: name},
	)
	if err != nil {
		tx.Rollback()
		_ = h.orchestrator.DeleteIdentity(c.Request.Context(), kratosID)
		c.String(http.StatusInternalServerError, "Authentication failed")
		return "", err
	}

	if err := h.eventBus.Publish(ctxTx, evt); err != nil {
		tx.Rollback()
		_ = h.orchestrator.DeleteIdentity(c.Request.Context(), kratosID)
		c.String(http.StatusInternalServerError, "Authentication failed")
		return "", err
	}

	if err := tx.Commit().Error; err != nil {
		_ = h.orchestrator.DeleteIdentity(c.Request.Context(), kratosID)
		c.String(http.StatusInternalServerError, "Authentication failed")
		return "", err
	}

	return kratosID, nil
}

// secureCompare performs a constant-time string comparison to prevent timing attacks.
func secureCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var result byte
	for i := 0; i < len(a); i++ {
		result |= a[i] ^ b[i]
	}
	return result == 0
}
