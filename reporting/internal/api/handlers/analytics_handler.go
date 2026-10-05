// =============================================================================
// Samvada Platform — Reporting Engine Analytics HTTP Handler
// =============================================================================

package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/samvada/reporting/internal/api/middleware"
	"github.com/samvada/reporting/internal/application/analytics"
)

type AnalyticsHandler struct {
	analyticsService analytics.AnalyticsService
}

func NewAnalyticsHandler(analyticsService analytics.AnalyticsService) *AnalyticsHandler {
	return &AnalyticsHandler{
		analyticsService: analyticsService,
	}
}

func parseOrgAndWorkspace(c *gin.Context) (uuid.UUID, *uuid.UUID, error) {
	orgIDStr := c.GetString(middleware.ContextOrgIDKey)
	if orgIDStr == "" {
		orgIDStr = c.GetHeader("X-Org-ID")
	}
	if orgIDStr == "" {
		orgIDStr = c.GetHeader("X-Organization-ID")
	}
	if orgIDStr == "" {
		orgIDStr = c.Query("org_id")
	}

	var wsID *uuid.UUID
	wsIDStr := c.GetHeader("X-Workspace-ID")
	if wsIDStr == "" {
		wsIDStr = c.Query("workspace_id")
	}
	if wsIDStr != "" {
		if parsed, err := uuid.Parse(wsIDStr); err == nil {
			wsID = &parsed
		}
	}

	orgID, err := uuid.Parse(orgIDStr)
	// If org_id is missing or invalid, but valid workspace_id is provided, allow scoping by workspace
	if err != nil && wsID != nil {
		return uuid.Nil, wsID, nil
	}
	if err != nil {
		return uuid.Nil, nil, fmt.Errorf("valid org_id or workspace_id is required via header, token, or query")
	}

	return orgID, wsID, nil
}

func (h *AnalyticsHandler) GetSummary(c *gin.Context) {
	orgID, wsID, err := parseOrgAndWorkspace(c)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ORG_ID", "Valid org_id is required via header, token, or query")
		return
	}

	start := time.Now().UTC().AddDate(0, 0, -30)
	if s := c.Query("start_date"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			start = t
		}
	}

	end := time.Now().UTC()
	if e := c.Query("end_date"); e != "" {
		if t, err := time.Parse(time.RFC3339, e); err == nil {
			end = t
		}
	}

	summary, err := h.analyticsService.GetKPISummary(c.Request.Context(), orgID, wsID, start, end)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "QUERY_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, summary)
}

func (h *AnalyticsHandler) GetVolumeTrend(c *gin.Context) {
	orgID, wsID, err := parseOrgAndWorkspace(c)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ORG_ID", "Valid org_id is required")
		return
	}

	days := 30
	if d := c.Query("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			days = parsed
		}
	}

	trends, err := h.analyticsService.GetDailyVolumeTrend(c.Request.Context(), orgID, wsID, days)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "QUERY_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, trends)
}

func (h *AnalyticsHandler) GetLeaderboard(c *gin.Context) {
	orgID, wsID, err := parseOrgAndWorkspace(c)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ORG_ID", "Valid org_id is required")
		return
	}

	limit := 10
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	start := time.Now().UTC().AddDate(0, 0, -30)
	end := time.Now().UTC()

	items, err := h.analyticsService.GetAgentLeaderboard(c.Request.Context(), orgID, wsID, start, end, limit)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "QUERY_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, items)
}

func (h *AnalyticsHandler) GetFunnels(c *gin.Context) {
	orgID, wsID, err := parseOrgAndWorkspace(c)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ORG_ID", "Valid org_id is required")
		return
	}

	start := time.Now().UTC().AddDate(0, 0, -30)
	end := time.Now().UTC()

	stages, err := h.analyticsService.GetConversionFunnels(c.Request.Context(), orgID, wsID, start, end)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "QUERY_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, stages)
}

func (h *AnalyticsHandler) GetLiveCalls(c *gin.Context) {
	orgID, wsID, err := parseOrgAndWorkspace(c)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ORG_ID", "Valid org_id is required")
		return
	}

	count, err := h.analyticsService.GetLiveCallCount(c.Request.Context(), orgID, wsID)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "REDIS_ERROR", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, gin.H{
		"active_calls_count": count,
	})
}
