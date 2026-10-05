// =============================================================================
// Samvada Platform — Reporting Engine Calls & CDR HTTP Handler
// =============================================================================

package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/samvada/reporting/internal/application/calls"
	"github.com/samvada/reporting/internal/domain"
)

type CallsHandler struct {
	callsService calls.CallsService
}

func NewCallsHandler(callsService calls.CallsService) *CallsHandler {
	return &CallsHandler{
		callsService: callsService,
	}
}

func (h *CallsHandler) ListCalls(c *gin.Context) {
	orgID, wsID, err := parseOrgAndWorkspace(c)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ORG_ID", "Valid org_id is required")
		return
	}

	limit := 20
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	offset := 0
	if o := c.Query("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	filter := domain.CallFilter{
		OrgID:       orgID,
		WorkspaceID: wsID,
		Limit:       limit,
		Offset:      offset,
	}

	if a := c.Query("agent_id"); a != "" {
		if parsed, err := uuid.Parse(a); err == nil {
			filter.AgentID = &parsed
		}
	}
	if camp := c.Query("campaign_id"); camp != "" {
		if parsed, err := uuid.Parse(camp); err == nil {
			filter.CampaignID = &parsed
		}
	}
	if disp := c.Query("disposition"); disp != "" {
		filter.Disposition = &disp
	}
	if sent := c.Query("sentiment"); sent != "" {
		filter.Sentiment = &sent
	}
	if dir := c.Query("direction"); dir != "" {
		filter.Direction = &dir
	}
	if s := c.Query("start_date"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			filter.StartDate = &t
		}
	}
	if e := c.Query("end_date"); e != "" {
		if t, err := time.Parse(time.RFC3339, e); err == nil {
			filter.EndDate = &t
		}
	}

	callsList, total, err := h.callsService.ListCalls(c.Request.Context(), filter)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "QUERY_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, gin.H{
		"calls":  callsList,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *CallsHandler) GetCall(c *gin.Context) {
	orgID, _, err := parseOrgAndWorkspace(c)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ORG_ID", "Valid org_id is required")
		return
	}

	callID, err := uuid.Parse(c.Param("call_id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_CALL_ID", "Valid call_id UUID is required")
		return
	}

	detail, err := h.callsService.GetCallDetail(c.Request.Context(), orgID, callID)
	if err != nil {
		RespondError(c, http.StatusNotFound, "CALL_NOT_FOUND", "Call detail record not found")
		return
	}

	RespondSuccess(c, http.StatusOK, detail)
}
