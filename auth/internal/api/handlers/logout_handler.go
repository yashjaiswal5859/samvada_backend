package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	applogout "github.com/ridesafety/auth-service/internal/application/logout"
	"github.com/ridesafety/auth-service/internal/shared/response"
)

type LogoutHandler struct {
	logoutService applogout.Service
	isProduction  bool
}

func NewLogoutHandler(service applogout.Service, isProduction bool) *LogoutHandler {
	return &LogoutHandler{
		logoutService: service,
		isProduction:  isProduction,
	}
}

func (h *LogoutHandler) Logout(c *gin.Context) {
	var body struct {
		RefreshToken string `json:"refreshToken"`
	}
	_ = c.ShouldBindJSON(&body)

	refreshToken, _ := c.Cookie("refresh_token")
	if refreshToken == "" {
		refreshToken = body.RefreshToken
	}

	kratosID := c.GetHeader("X-User-Id")
	if kratosID == "" {
		kratosID = c.GetString("user_id")
	}

	corrID := c.GetString("correlation_id")

	err := h.logoutService.Logout(c.Request.Context(), refreshToken, kratosID, corrID)
	// Server flushes all auth cookies regardless of backend downstream status
	FlushAuthCookies(c, h.isProduction)
	if err != nil {
		response.Error(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}
