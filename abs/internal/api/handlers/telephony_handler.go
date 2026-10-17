// =============================================================================
// Samvada Platform — ABS Telephony HTTP Handler
// =============================================================================

package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/samvada/abs/internal/api/middleware"
	"github.com/samvada/abs/internal/application/telephony"
	"github.com/samvada/abs/internal/domain"
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
	wsIDStr := middleware.GetWorkspaceID(c)
	if wsIDStr == "" {
		wsIDStr = c.GetHeader("X-Workspace-ID")
	}
	if wsIDStr == "" {
		wsIDStr = c.Query("workspace_id")
	}

	var wsID uuid.UUID
	var err error
	if wsIDStr != "" {
		wsID, err = uuid.Parse(wsIDStr)
		if err != nil {
			RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
			return
		}
	}

	search := c.Query("search")
	if search == "" {
		search = c.Query("query")
	}
	if search == "" {
		search = c.Query("q")
	}

	numbers, err := h.telephonyService.ListWorkspaceNumbers(c.Request.Context(), wsID, search)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, numbers)
}

func (h *TelephonyHandler) SearchAvailableNumbers(c *gin.Context) {
	wsIDStr := middleware.GetWorkspaceID(c)
	if wsIDStr == "" {
		wsIDStr = c.GetHeader("X-Workspace-ID")
	}
	if wsIDStr == "" {
		wsIDStr = c.Query("workspace_id")
	}
	var wsIDPtr *uuid.UUID
	if wsID, err := uuid.Parse(wsIDStr); err == nil {
		wsIDPtr = &wsID
	}

	search := c.Query("search")
	if search == "" {
		search = c.Query("query")
	}
	if search == "" {
		search = c.Query("q")
	}

	numbers, err := h.telephonyService.ListAvailableNumbersFromDB(c.Request.Context(), wsIDPtr, search)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "DB_SEARCH_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, numbers)
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
	wsIDStr := middleware.GetWorkspaceID(c)
	if wsIDStr == "" {
		wsIDStr = c.GetHeader("X-Workspace-ID")
	}
	if wsIDStr == "" {
		wsIDStr = c.Query("workspace_id")
	}
	wsID, err := uuid.Parse(wsIDStr)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	var req struct {
		PhoneNumberID   *uuid.UUID `json:"phone_number_id"`
		PhoneNumber     string     `json:"phone_number"`
		Provider        string     `json:"provider"`
		Type            string     `json:"type"`
		AssignedAgentID *uuid.UUID `json:"assigned_agent_id"`
		AgentID         *uuid.UUID `json:"agent_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	agentID := req.AssignedAgentID
	if agentID == nil {
		agentID = req.AgentID
	}

	var pn *domain.PhoneNumber
	if req.PhoneNumberID != nil && *req.PhoneNumberID != uuid.Nil {
		pn, err = h.telephonyService.AllocateNumberFromDB(c.Request.Context(), wsID, *req.PhoneNumberID, agentID)
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ALLOCATION_FAILED", "Selected phone number could not be allocated from database: "+err.Error())
			return
		}
	} else if req.PhoneNumber != "" {
		existing, err := h.telephonyService.GetByNumber(c.Request.Context(), req.PhoneNumber)
		if err != nil || existing == nil {
			RespondError(c, http.StatusBadRequest, "NUMBER_NOT_IN_DB", "User cannot write random numbers. You must select an existing number from our database inventory.")
			return
		}
		pn, err = h.telephonyService.AllocateNumberFromDB(c.Request.Context(), wsID, existing.ID, agentID)
		if err != nil {
			RespondError(c, http.StatusBadRequest, "ALLOCATION_FAILED", "Failed to allocate number from database: "+err.Error())
			return
		}
	} else {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "Please select a phone number from the database")
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
