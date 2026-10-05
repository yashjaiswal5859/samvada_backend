// =============================================================================
// Samvada Platform — ABS Workspace HTTP Handler
// =============================================================================

package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/samvada/abs/internal/api/dto"
	"github.com/samvada/abs/internal/api/middleware"
	"github.com/samvada/abs/internal/application/workspace"
)

type WorkspaceHandler struct {
	wsService workspace.WorkspaceService
}

func NewWorkspaceHandler(wsService workspace.WorkspaceService) *WorkspaceHandler {
	return &WorkspaceHandler{
		wsService: wsService,
	}
}

func (h *WorkspaceHandler) CreateWorkspace(c *gin.Context) {
	userIDStr := c.GetString(middleware.ContextUserIDKey)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Valid user session required")
		return
	}

	orgIDStr := c.GetHeader("X-Org-ID")
	if orgIDStr == "" {
		orgIDStr = c.Query("org_id")
	}
	orgID, err := uuid.Parse(orgIDStr)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ORG_ID", "X-Org-ID header or org_id query param is required")
		return
	}

	var req dto.CreateWorkspaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	ws, err := h.wsService.CreateWorkspace(c.Request.Context(), orgID, workspace.CreateWorkspaceRequest{
		Name:               req.Name,
		Slug:               req.Slug,
		Icon:               req.Icon,
		Description:        req.Description,
		MonthlyMinuteLimit: req.MonthlyMinuteLimit,
		CreatorUserID:      userID,
	})
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "WORKSPACE_CREATION_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusCreated, ws)
}

func (h *WorkspaceHandler) ListMyWorkspaces(c *gin.Context) {
	userIDStr := c.GetString(middleware.ContextUserIDKey)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Valid user session required")
		return
	}

	workspaces, err := h.wsService.ListUserWorkspaces(c.Request.Context(), userID)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	RespondSuccess(c, http.StatusOK, workspaces)
}

func (h *WorkspaceHandler) GetWorkspace(c *gin.Context) {
	param := c.Param("workspace_id")
	if param == "" {
		param = c.Param("ws_id")
	}
	wsID, err := uuid.Parse(param)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE_ID", "Invalid workspace UUID format")
		return
	}

	ws, err := h.wsService.GetWorkspace(c.Request.Context(), wsID)
	if err != nil {
		RespondError(c, http.StatusNotFound, "WORKSPACE_NOT_FOUND", err.Error())
		return
	}
	RespondSuccess(c, http.StatusOK, ws)
}

func (h *WorkspaceHandler) ListMembers(c *gin.Context) {
	param := c.Param("workspace_id")
	if param == "" {
		param = c.Param("ws_id")
	}
	wsID, err := uuid.Parse(param)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE_ID", "Invalid workspace UUID format")
		return
	}

	members, err := h.wsService.ListMembers(c.Request.Context(), wsID)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	RespondSuccess(c, http.StatusOK, members)
}
