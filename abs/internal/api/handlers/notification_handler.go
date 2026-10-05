// =============================================================================
// Samvada Platform — ABS Notification Preferences HTTP Handler
// =============================================================================

package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/api/middleware"
	"github.com/samvada/abs/internal/application/notification"
	apperrors "github.com/samvada/proto/pkg/errors"
)

type NotificationHandler struct {
	service *notification.NotificationService
	log     *zap.Logger
}

func NewNotificationHandler(service *notification.NotificationService, log *zap.Logger) *NotificationHandler {
	return &NotificationHandler{
		service: service,
		log:     log.Named("notification_handler"),
	}
}

// GetPreferences godoc
// @Summary Fetch notification preferences for workspace
// @Tags Notifications
// @Produce json
// @Success 200 {object} domain.NotificationPreferences
// @Router /api/v1/notifications/preferences [get]
func (h *NotificationHandler) GetPreferences(c *gin.Context) {
	userIDStr := middleware.GetUserID(c)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error": gin.H{"code": apperrors.CodeUnauthorized, "message": "Invalid user token"},
		})
		return
	}

	wsIDStr := middleware.GetWorkspaceID(c)
	wsID, err := uuid.Parse(wsIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{"code": apperrors.CodeInvalidInput, "message": "Valid workspace context required"},
		})
		return
	}

	prefs, err := h.service.GetPreferences(c.Request.Context(), userID, wsID)
	if err != nil {
		h.log.Error("Failed to fetch notification preferences", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{"code": apperrors.CodeInternal, "message": "Failed to fetch notification preferences"},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    prefs,
	})
}

// UpdatePreferences godoc
// @Summary Update notification preferences
// @Tags Notifications
// @Accept json
// @Produce json
// @Success 200 {object} domain.NotificationPreferences
// @Router /api/v1/notifications/preferences [put]
func (h *NotificationHandler) UpdatePreferences(c *gin.Context) {
	userIDStr := middleware.GetUserID(c)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error": gin.H{"code": apperrors.CodeUnauthorized, "message": "Invalid user token"},
		})
		return
	}

	wsIDStr := middleware.GetWorkspaceID(c)
	wsID, err := uuid.Parse(wsIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{"code": apperrors.CodeInvalidInput, "message": "Valid workspace context required"},
		})
		return
	}

	var req notification.UpdatePreferencesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{"code": apperrors.CodeInvalidInput, "message": err.Error()},
		})
		return
	}

	prefs, err := h.service.UpdatePreferences(c.Request.Context(), userID, wsID, req)
	if err != nil {
		h.log.Error("Failed to update notification preferences", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{"code": apperrors.CodeInternal, "message": "Failed to update notification preferences"},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    prefs,
	})
}

// SendTest godoc
// @Summary Send test notification
// @Tags Notifications
// @Accept json
// @Produce json
// @Success 200 {object} map[string]string
// @Router /api/v1/notifications/test [post]
func (h *NotificationHandler) SendTest(c *gin.Context) {
	userIDStr := middleware.GetUserID(c)
	userID, _ := uuid.Parse(userIDStr)
	wsIDStr := middleware.GetWorkspaceID(c)
	wsID, _ := uuid.Parse(wsIDStr)

	var body struct {
		Channel string `json:"channel"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.Channel == "" {
		body.Channel = "all"
	}

	_ = h.service.SendTestNotification(c.Request.Context(), userID, wsID, body.Channel)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Test notification dispatched across configured channels",
	})
}
