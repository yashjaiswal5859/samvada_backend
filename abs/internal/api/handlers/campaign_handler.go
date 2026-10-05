// =============================================================================
// Samvada Platform — ABS Campaign & Audience HTTP Handler
// =============================================================================

package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/samvada/abs/internal/api/middleware"
	"github.com/samvada/abs/internal/application/campaign"
	"github.com/samvada/abs/internal/domain"
)

type CampaignHandler struct {
	campaignService campaign.CampaignService
}

func NewCampaignHandler(campaignService campaign.CampaignService) *CampaignHandler {
	return &CampaignHandler{
		campaignService: campaignService,
	}
}

func (h *CampaignHandler) CreateAudience(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	var req struct {
		Name        string  `json:"name" binding:"required"`
		Description *string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	aud, err := h.campaignService.CreateAudience(c.Request.Context(), wsID, req.Name, req.Description)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "AUDIENCE_CREATION_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusCreated, aud)
}

func (h *CampaignHandler) ListAudiences(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	audiences, err := h.campaignService.ListAudiences(c.Request.Context(), wsID)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, audiences)
}

func (h *CampaignHandler) DeleteAudience(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	audID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_AUDIENCE_ID", "Invalid audience UUID")
		return
	}

	if err := h.campaignService.DeleteAudience(c.Request.Context(), wsID, audID); err != nil {
		RespondError(c, http.StatusInternalServerError, "DELETE_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, gin.H{"deleted": true, "audience_id": audID})
}

func (h *CampaignHandler) DeleteContact(c *gin.Context) {
	audID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_AUDIENCE_ID", "Invalid audience UUID")
		return
	}

	contactID, err := uuid.Parse(c.Param("contact_id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_CONTACT_ID", "Invalid contact UUID")
		return
	}

	if err := h.campaignService.DeleteContact(c.Request.Context(), audID, contactID); err != nil {
		RespondError(c, http.StatusInternalServerError, "DELETE_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, gin.H{"deleted": true, "contact_id": contactID})
}

func (h *CampaignHandler) BatchImportCSV(c *gin.Context) {
	audID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_AUDIENCE_ID", "Invalid audience UUID")
		return
	}

	file, err := c.FormFile("csv_file")
	if err != nil {
		imported, skipped, err := h.campaignService.BatchImportCSV(c.Request.Context(), audID, c.Request.Body)
		if err != nil {
			RespondError(c, http.StatusBadRequest, "CSV_IMPORT_FAILED", err.Error())
			return
		}
		RespondSuccess(c, http.StatusOK, gin.H{"imported_count": imported, "skipped_count": skipped})
		return
	}

	src, err := file.Open()
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "FILE_OPEN_FAILED", err.Error())
		return
	}
	defer src.Close()

	imported, skipped, err := h.campaignService.BatchImportCSV(c.Request.Context(), audID, src)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "CSV_IMPORT_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, gin.H{"imported_count": imported, "skipped_count": skipped})
}

func (h *CampaignHandler) ImportContacts(c *gin.Context) {
	audID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_AUDIENCE_ID", "Invalid audience UUID")
		return
	}

	var req struct {
		Contacts []struct {
			Name    string  `json:"name" binding:"required"`
			Phone   string  `json:"phone" binding:"required"`
			Email   *string `json:"email"`
			Company *string `json:"company"`
		} `json:"contacts" binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	domainContacts := make([]*domain.Contact, 0, len(req.Contacts))
	for _, raw := range req.Contacts {
		domainContacts = append(domainContacts, &domain.Contact{
			Name:    raw.Name,
			Phone:   raw.Phone,
			Email:   raw.Email,
			Company: raw.Company,
			Status:  "active",
		})
	}

	if err := h.campaignService.ImportContacts(c.Request.Context(), audID, domainContacts); err != nil {
		RespondError(c, http.StatusInternalServerError, "IMPORT_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, gin.H{
		"imported_count": len(domainContacts),
	})
}

func (h *CampaignHandler) ListContacts(c *gin.Context) {
	audID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_AUDIENCE_ID", "Invalid audience UUID")
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	contacts, count, err := h.campaignService.ListContacts(c.Request.Context(), audID, limit, offset)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, gin.H{
		"contacts":    contacts,
		"total_count": count,
	})
}

func (h *CampaignHandler) CreateCampaign(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	var req struct {
		Name                 string     `json:"name" binding:"required"`
		AgentID              uuid.UUID  `json:"agent_id" binding:"required"`
		AudienceID           uuid.UUID  `json:"audience_id" binding:"required"`
		ConcurrentCallsLimit int        `json:"concurrent_calls_limit"`
		ScheduledAt          *time.Time `json:"scheduled_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	cmp, err := h.campaignService.CreateCampaign(c.Request.Context(), wsID, campaign.CreateCampaignRequest{
		Name:                 req.Name,
		AgentID:              req.AgentID,
		AudienceID:           req.AudienceID,
		ConcurrentCallsLimit: req.ConcurrentCallsLimit,
		ScheduledAt:          req.ScheduledAt,
	})
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "CAMPAIGN_CREATION_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusCreated, cmp)
}

func (h *CampaignHandler) ListCampaigns(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	campaigns, err := h.campaignService.ListCampaigns(c.Request.Context(), wsID)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, campaigns)
}

func (h *CampaignHandler) UpdateStatus(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	cmpID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_CAMPAIGN_ID", "Invalid campaign UUID")
		return
	}

	var req struct {
		Status string `json:"status" binding:"required,oneof=scheduled running paused completed cancelled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if err := h.campaignService.UpdateCampaignStatus(c.Request.Context(), wsID, cmpID, req.Status); err != nil {
		RespondError(c, http.StatusInternalServerError, "UPDATE_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, gin.H{"status": req.Status})
}
