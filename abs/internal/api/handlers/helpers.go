// =============================================================================
// Samvada Platform — ABS HTTP Response Helpers
// =============================================================================

package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/samvada/abs/internal/api/dto"
	"github.com/samvada/abs/internal/api/middleware"
)

func RespondSuccess(c *gin.Context, status int, data interface{}) {
	corrID := c.GetHeader(middleware.HeaderCorrelationID)
	c.JSON(status, dto.APIResponse{
		Success:       true,
		Data:          data,
		CorrelationID: corrID,
	})
}

func RespondError(c *gin.Context, status int, code, message string) {
	corrID := c.GetHeader(middleware.HeaderCorrelationID)
	c.AbortWithStatusJSON(status, dto.APIResponse{
		Success: false,
		Error: &dto.APIError{
			Code:    code,
			Message: message,
		},
		CorrelationID: corrID,
	})
}
