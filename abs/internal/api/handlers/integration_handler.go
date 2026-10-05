// =============================================================================
// Samvada Platform — ABS Third-Party Connector HTTP Handler
// =============================================================================

package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/api/middleware"
	"github.com/samvada/abs/internal/application/integration"
	apperrors "github.com/samvada/proto/pkg/errors"
)

type ConnectorHandler struct {
	service *integration.ConnectorService
	log     *zap.Logger
}

func NewConnectorHandler(service *integration.ConnectorService, log *zap.Logger) *ConnectorHandler {
	return &ConnectorHandler{
		service: service,
		log:     log.Named("connector_handler"),
	}
}

// ListIntegrations godoc
// @Summary List integration connectors for workspace
// @Tags Integrations
// @Produce json
// @Success 200 {array} domain.ConnectorIntegration
// @Router /api/v1/integrations [get]
func (h *ConnectorHandler) ListIntegrations(c *gin.Context) {
	wsIDStr := middleware.GetWorkspaceID(c)
	wsID, err := uuid.Parse(wsIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{"code": apperrors.CodeInvalidInput, "message": "Valid workspace context required"},
		})
		return
	}

	list, err := h.service.ListIntegrations(c.Request.Context(), wsID)
	if err != nil {
		h.log.Error("Failed to list integrations", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{"code": apperrors.CodeInternal, "message": "Failed to list integrations"},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    list,
	})
}

// ToggleIntegration godoc
// @Summary Toggle connector connection state
// @Tags Integrations
// @Accept json
// @Produce json
// @Success 200 {object} domain.ConnectorIntegration
// @Router /api/v1/integrations/:id/toggle [post]
func (h *ConnectorHandler) ToggleIntegration(c *gin.Context) {
	wsIDStr := middleware.GetWorkspaceID(c)
	wsID, err := uuid.Parse(wsIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{"code": apperrors.CodeInvalidInput, "message": "Valid workspace context required"},
		})
		return
	}

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{"code": apperrors.CodeInvalidInput, "message": "Invalid integration ID"},
		})
		return
	}

	var req integration.ToggleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{"code": apperrors.CodeInvalidInput, "message": err.Error()},
		})
		return
	}

	item, err := h.service.ToggleIntegration(c.Request.Context(), wsID, id, req.Connected)
	if err != nil {
		h.log.Error("Failed to toggle integration", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{"code": apperrors.CodeInternal, "message": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    item,
	})
}

// SaveCredentials godoc
// @Summary Store API keys / credentials for connector
// @Tags Integrations
// @Accept json
// @Produce json
// @Success 200 {object} domain.ConnectorIntegration
// @Router /api/v1/integrations/:id/credentials [post]
func (h *ConnectorHandler) SaveCredentials(c *gin.Context) {
	wsIDStr := middleware.GetWorkspaceID(c)
	wsID, err := uuid.Parse(wsIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{"code": apperrors.CodeInvalidInput, "message": "Valid workspace context required"},
		})
		return
	}

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{"code": apperrors.CodeInvalidInput, "message": "Invalid integration ID"},
		})
		return
	}

	var req integration.CredentialsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{"code": apperrors.CodeInvalidInput, "message": err.Error()},
		})
		return
	}

	item, err := h.service.SaveCredentials(c.Request.Context(), wsID, id, req.CredentialsJSON)
	if err != nil {
		h.log.Error("Failed to save credentials", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{"code": apperrors.CodeInternal, "message": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    item,
	})
}
