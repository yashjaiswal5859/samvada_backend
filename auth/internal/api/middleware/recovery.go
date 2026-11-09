package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ridesafety/auth-service/internal/shared/response"
	"go.uber.org/zap"
)

func Recovery(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				corrID := c.GetString("correlation_id")
				logger.Error("Recovery from panic",
					zap.String("correlation_id", corrID),
					zap.Any("error", err),
				)

				response.JSON(c, http.StatusInternalServerError, false, nil, &response.ErrorDetail{
					Code:    "AUTH_500",
					Message: "An internal server error occurred",
				})
				c.Abort()
			}
		}()
		c.Next()
	}
}
