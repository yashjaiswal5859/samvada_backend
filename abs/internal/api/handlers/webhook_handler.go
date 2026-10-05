// =============================================================================
// Samvada Platform — ABS Integration & Webhook HTTP Handler
// =============================================================================

package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/samvada/abs/internal/api/middleware"
	"github.com/samvada/abs/internal/application/webhook"
)

type WebhookHandler struct {
	integrationService webhook.IntegrationService
}

func NewWebhookHandler(integrationService webhook.IntegrationService) *WebhookHandler {
	return &WebhookHandler{
		integrationService: integrationService,
	}
}

func (h *WebhookHandler) CreateAPIKey(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	rawKey, apiKey, err := h.integrationService.CreateAPIKey(c.Request.Context(), wsID, req.Name)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "API_KEY_CREATION_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusCreated, gin.H{
		"api_key": apiKey,
		"secret":  rawKey,
	})
}

func (h *WebhookHandler) ListAPIKeys(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	keys, err := h.integrationService.ListAPIKeys(c.Request.Context(), wsID)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, keys)
}

func (h *WebhookHandler) RevokeAPIKey(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	keyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_KEY_ID", "Invalid key UUID")
		return
	}

	if err := h.integrationService.RevokeAPIKey(c.Request.Context(), wsID, keyID); err != nil {
		RespondError(c, http.StatusInternalServerError, "REVOKE_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, gin.H{"revoked": true})
}

func (h *WebhookHandler) CreateWebhook(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	var req struct {
		URL string `json:"url" binding:"required,url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	wh, signingSecret, err := h.integrationService.CreateWebhook(c.Request.Context(), wsID, req.URL)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "WEBHOOK_CREATION_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusCreated, gin.H{
		"webhook":        wh,
		"signing_secret": signingSecret,
	})
}

func (h *WebhookHandler) ListWebhooks(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	webhooks, err := h.integrationService.ListWebhooks(c.Request.Context(), wsID)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, webhooks)
}

func (h *WebhookHandler) DeleteWebhook(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	whID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WEBHOOK_ID", "Invalid webhook UUID")
		return
	}

	if err := h.integrationService.DeleteWebhook(c.Request.Context(), wsID, whID); err != nil {
		RespondError(c, http.StatusInternalServerError, "DELETE_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, gin.H{"deleted": true})
}
