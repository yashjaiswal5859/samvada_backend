package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ridesafety/auth-service/internal/application/onboarding"
	"github.com/ridesafety/auth-service/internal/domain/user"
	"github.com/ridesafety/auth-service/internal/platform/identity"
	"github.com/ridesafety/auth-service/internal/shared/response"
)

type MeHandler struct {
	orchestrator      identity.IdentityOrchestrator
	userRepo          user.Repository
	onboardingService onboarding.Service
	isProduction      bool
}

func NewMeHandler(orchestrator identity.IdentityOrchestrator, userRepo user.Repository, onboardingService onboarding.Service, isProduction ...bool) *MeHandler {
	prod := false
	if len(isProduction) > 0 {
		prod = isProduction[0]
	}
	return &MeHandler{
		orchestrator:      orchestrator,
		userRepo:          userRepo,
		onboardingService: onboardingService,
		isProduction:      prod,
	}
}

func (h *MeHandler) Me(c *gin.Context) {
	token, err := c.Cookie("access_token")
	if err != nil || token == "" {
		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		}
	}

	if token == "" {
		response.Unauthorized(c, "AUTH_SESSION_EXPIRED", "Unauthorized: Missing access token")
		return
	}

	active, subject, exp, err := h.orchestrator.IntrospectToken(c.Request.Context(), token)
	if err != nil || !active {
		response.Unauthorized(c, "AUTH_SESSION_EXPIRED", "Unauthorized: Invalid or expired access token")
		return
	}

	u, err := h.userRepo.FindByKratosID(c.Request.Context(), subject)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			FlushAuthCookies(c, h.isProduction)
			response.UnauthorizedRequireLogin(c, "AUTH_SESSION_EXPIRED", "Unauthorized: User session invalid or account does not exist")
			return
		}
		c.JSON(http.StatusInternalServerError, response.JSONResponse{
			Success:       false,
			CorrelationID: c.GetString("correlation_id"),
			Error: &response.ErrorDetail{
				Code:    "AUTH_500",
				Message: "Database error retrieving user details: " + err.Error(),
			},
		})
		return
	}

	// Calculate remaining lifetime in seconds
	var expiresIn int64
	if exp > 0 {
		expiresIn = exp - time.Now().Unix()
		if expiresIn < 0 {
			expiresIn = 0
		}
	}

	onboardingState, _ := h.onboardingService.GetOnboardingState(c.Request.Context(), u.ID)

	dataMap := map[string]interface{}{
		"id":                  u.ID,
		"email":               u.Email,
		"displayName":         u.DisplayName,
		"expiresIn":           expiresIn,
		"onboardingCompleted": false,
	}

	if onboardingState != nil {
		dataMap["onboardingCompleted"] = onboardingState.OnboardingCompleted
		dataMap["profile"] = onboardingState.Profile
		dataMap["organization"] = onboardingState.Organization
		dataMap["workspace"] = onboardingState.Workspace
	}

	c.JSON(http.StatusOK, response.JSONResponse{
		Success:       true,
		CorrelationID: c.GetString("correlation_id"),
		Data:          dataMap,
	})
}
