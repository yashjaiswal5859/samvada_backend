// =============================================================================
// Samvada Platform — ABS Audit Log HTTP Handler
// =============================================================================

package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/api/middleware"
	"github.com/samvada/abs/internal/application/audit"
	"github.com/samvada/abs/internal/domain"
	apperrors "github.com/samvada/proto/pkg/errors"
)

type AuditHandler struct {
	service *audit.AuditService
	log     *zap.Logger
}

func NewAuditHandler(service *audit.AuditService, log *zap.Logger) *AuditHandler {
	return &AuditHandler{
		service: service,
		log:     log.Named("audit_handler"),
	}
}

// ListLogs godoc
// @Summary List audit logs for the current workspace
// @Tags Audit
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/audit-logs [get]
func (h *AuditHandler) ListLogs(c *gin.Context) {
	wsIDStr := middleware.GetWorkspaceID(c)
	wsID, err := uuid.Parse(wsIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"code": apperrors.CodeInvalidInput, "message": "Invalid workspace context"},
		})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	action := c.Query("action")
	resourceType := c.Query("resource_type")

	filter := domain.AuditLogFilter{
		WorkspaceID:  &wsID,
		Action:       action,
		ResourceType: resourceType,
		Limit:        limit,
		Offset:       offset,
	}

	logs, total, err := h.service.List(c.Request.Context(), filter)
	if err != nil {
		h.log.Error("Failed to list audit logs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   gin.H{"code": apperrors.CodeInternal, "message": "Failed to fetch audit records"},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"total":  total,
			"limit":  limit,
			"offset": offset,
			"logs":   logs,
		},
	})
}
