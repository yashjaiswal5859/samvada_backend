// =============================================================================
// Samvada Platform — ABS Admin REST API Handler
// =============================================================================
// Package: handlers
//
// Responsibilities:
//   1. Exposes POST /api/v1/admin/login for curl/API-based administrative authentication.
//   2. Exposes GET /api/v1/admin/me for session verification by human operators.
// =============================================================================

package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/samvada/abs/internal/api/middleware"
	"github.com/samvada/abs/internal/application/admin"
)

// AdminHandler handles HTTP requests for admin authentication and administration.
type AdminHandler struct {
	adminService admin.AdminService
}

// NewAdminHandler creates an instance of AdminHandler.
func NewAdminHandler(adminService admin.AdminService) *AdminHandler {
	return &AdminHandler{adminService: adminService}
}

// AdminLoginRequest defines the JSON payload expected on POST /api/v1/admin/login.
type AdminLoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login handles POST /api/v1/admin/login.
//
// Expects:
//   POST /api/v1/admin/login
//   Content-Type: application/json
//   {"username": "admin", "password": "..."}
//
// Returns:
//   {"success": true, "access_token": "...", "refresh_token": "...", "token_type": "Bearer", "expires_in": 3600}
func (h *AdminHandler) Login(c *gin.Context) {
	var req AdminLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": "Both 'username' and 'password' are required in JSON payload.",
			},
		})
		return
	}

	result, err := h.adminService.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, admin.ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "INVALID_CREDENTIALS",
					"message": "Invalid admin username or password.",
				},
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": "Failed to complete admin login: " + err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"access_token":  result.AccessToken,
		"refresh_token": result.RefreshToken,
		"token_type":    result.TokenType,
		"expires_in":    result.ExpiresIn,
	})
}

// Me handles GET /api/v1/admin/me (Protected by RequireAdminKeto middleware).
func (h *AdminHandler) Me(c *gin.Context) {
	userID := middleware.GetUserID(c)
	claims, _ := c.Get(middleware.ContextClaimsKey)

	c.JSON(http.StatusOK, gin.H{
		"success":            true,
		"user_id":            userID,
		"role":               "admin",
		"keto_role_verified": true,
		"claims":             claims,
	})
}
