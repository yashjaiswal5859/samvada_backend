// =============================================================================
// Samvada Platform — ABS Telephony HTTP Handler
// =============================================================================

package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/samvada/abs/internal/api/middleware"
	"github.com/samvada/abs/internal/application/telephony"
)

type TelephonyHandler struct {
	telephonyService telephony.TelephonyService
}

func NewTelephonyHandler(telephonyService telephony.TelephonyService) *TelephonyHandler {
	return &TelephonyHandler{
		telephonyService: telephonyService,
	}
}

func (h *TelephonyHandler) ListNumbers(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	numbers, err := h.telephonyService.ListWorkspaceNumbers(c.Request.Context(), wsID)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, numbers)
}

func (h *TelephonyHandler) SearchAvailableNumbers(c *gin.Context) {
	provider := c.DefaultQuery("provider", "twilio")
	country := c.DefaultQuery("country", "US")
	areaCode := c.Query("area_code")
	numType := c.DefaultQuery("type", "local")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	numbers, err := h.telephonyService.SearchAvailableNumbers(c.Request.Context(), provider, country, areaCode, numType, limit)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "SEARCH_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, gin.H{
		"provider": provider,
		"numbers":  numbers,
	})
}

func (h *TelephonyHandler) PurchaseNumber(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	var req struct {
		PhoneNumber string `json:"phone_number" binding:"required"`
		Provider    string `json:"provider"`
		WebhookURL  string `json:"webhook_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	pn, err := h.telephonyService.PurchaseNumber(c.Request.Context(), wsID, req.Provider, req.PhoneNumber, req.WebhookURL)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "PURCHASE_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusCreated, pn)
}

func (h *TelephonyHandler) ProvisionNumber(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	var req struct {
		PhoneNumber string `json:"phone_number" binding:"required"`
		Provider    string `json:"provider"`
		Type        string `json:"type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	pn, err := h.telephonyService.ProvisionNumber(c.Request.Context(), wsID, req.PhoneNumber, req.Provider, req.Type)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "PROVISION_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusCreated, pn)
}

func (h *TelephonyHandler) ReleaseNumber(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	numberID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_NUMBER_ID", "Invalid phone number UUID")
		return
	}

	if err := h.telephonyService.ReleaseNumber(c.Request.Context(), wsID, numberID); err != nil {
		RespondError(c, http.StatusInternalServerError, "RELEASE_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, gin.H{"released": true, "number_id": numberID})
}

func (h *TelephonyHandler) AssignAgent(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	numberID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_NUMBER_ID", "Invalid phone number UUID")
		return
	}

	var req struct {
		AgentID uuid.UUID `json:"agent_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if err := h.telephonyService.AssignAgent(c.Request.Context(), wsID, numberID, req.AgentID); err != nil {
		RespondError(c, http.StatusInternalServerError, "ASSIGN_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, gin.H{"assigned": true})
}

func (h *TelephonyHandler) UnassignAgent(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	numberID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_NUMBER_ID", "Invalid phone number UUID")
		return
	}

	if err := h.telephonyService.UnassignAgent(c.Request.Context(), wsID, numberID); err != nil {
		RespondError(c, http.StatusInternalServerError, "UNASSIGN_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, gin.H{"unassigned": true})
}
