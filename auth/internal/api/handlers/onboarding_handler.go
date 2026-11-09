package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	appOnboarding "github.com/ridesafety/auth-service/internal/application/onboarding"
	"github.com/ridesafety/auth-service/internal/domain/user"
	"github.com/ridesafety/auth-service/internal/platform/identity"
	"github.com/ridesafety/auth-service/internal/shared/response"
)

type OnboardingHandler struct {
	service      appOnboarding.Service
	orchestrator identity.IdentityOrchestrator
	userRepo     user.Repository
	isProduction bool
}

func NewOnboardingHandler(
	service appOnboarding.Service,
	orchestrator identity.IdentityOrchestrator,
	userRepo user.Repository,
	isProduction ...bool,
) *OnboardingHandler {
	prod := false
	if len(isProduction) > 0 {
		prod = isProduction[0]
	}
	return &OnboardingHandler{
		service:      service,
		orchestrator: orchestrator,
		userRepo:     userRepo,
		isProduction: prod,
	}
}

func (h *OnboardingHandler) authenticateUser(c *gin.Context) (*user.User, bool) {
	token, err := c.Cookie("access_token")
	if err != nil || token == "" {
		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		}
	}

	if token == "" {
		FlushAuthCookies(c, h.isProduction)
		c.JSON(http.StatusUnauthorized, response.JSONResponse{
			Success:       false,
			CorrelationID: c.GetString("correlation_id"),
			Error: &response.ErrorDetail{
				Code:    "AUTH_401",
				Message: "Unauthorized: Missing access token",
			},
		})
		return nil, false
	}

	active, subject, _, err := h.orchestrator.IntrospectToken(c.Request.Context(), token)
	if err != nil || !active {
		FlushAuthCookies(c, h.isProduction)
		c.JSON(http.StatusUnauthorized, response.JSONResponse{
			Success:       false,
			CorrelationID: c.GetString("correlation_id"),
			Error: &response.ErrorDetail{
				Code:    "AUTH_401",
				Message: "Unauthorized: Invalid or expired access token",
			},
		})
		return nil, false
	}

	u, err := h.userRepo.FindByKratosID(c.Request.Context(), subject)
	if err != nil {
		FlushAuthCookies(c, h.isProduction)
		c.JSON(http.StatusUnauthorized, response.JSONResponse{
			Success:       false,
			CorrelationID: c.GetString("correlation_id"),
			Error: &response.ErrorDetail{
				Code:    "AUTH_401",
				Message: "Unauthorized: User record not found",
			},
		})
		return nil, false
	}

	return u, true
}

func (h *OnboardingHandler) SaveProfile(c *gin.Context) {
	u, ok := h.authenticateUser(c)
	if !ok {
		return
	}

	var req appOnboarding.ProfileRequest
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

	state, err := h.service.SaveProfile(c.Request.Context(), u.ID, u.Email, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.JSONResponse{
			Success:       false,
			CorrelationID: c.GetString("correlation_id"),
			Error: &response.ErrorDetail{
				Code:    "AUTH_400",
				Message: err.Error(),
			},
		})
		return
	}

	response.Success(c, http.StatusOK, state)
}

func (h *OnboardingHandler) SaveIndustries(c *gin.Context) {
	u, ok := h.authenticateUser(c)
	if !ok {
		return
	}

	var req appOnboarding.IndustriesRequest
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

	state, err := h.service.SaveIndustries(c.Request.Context(), u.ID, u.Email, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.JSONResponse{
			Success:       false,
			CorrelationID: c.GetString("correlation_id"),
			Error: &response.ErrorDetail{
				Code:    "AUTH_400",
				Message: err.Error(),
			},
		})
		return
	}

	response.Success(c, http.StatusOK, state)
}

func (h *OnboardingHandler) GetState(c *gin.Context) {
	u, ok := h.authenticateUser(c)
	if !ok {
		return
	}

	state, err := h.service.GetOnboardingState(c.Request.Context(), u.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.JSONResponse{
			Success:       false,
			CorrelationID: c.GetString("correlation_id"),
			Error: &response.ErrorDetail{
				Code:    "AUTH_500",
				Message: err.Error(),
			},
		})
		return
	}

	response.Success(c, http.StatusOK, state)
}
