package response

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	apperrors "github.com/ridesafety/auth-service/internal/shared/errors"
)

type JSONResponse struct {
	Success       bool         `json:"success"`
	CorrelationID string       `json:"correlation_id,omitempty"`
	Data          interface{}  `json:"data,omitempty"`
	Error         *ErrorDetail `json:"error,omitempty"`
}

type ErrorDetail struct {
	Code         string `json:"code,omitempty"`
	Message      string `json:"message"`
	RequireLogin bool   `json:"require_login,omitempty"`
}

func UnauthorizedRequireLogin(c *gin.Context, code string, message string) {
	corrID := c.GetString("correlation_id")
	if code == "" {
		code = "AUTH_401"
	}
	c.JSON(http.StatusUnauthorized, JSONResponse{
		Success:       false,
		CorrelationID: corrID,
		Error: &ErrorDetail{
			Code:         code,
			Message:      message,
			RequireLogin: true,
		},
	})
}

func Unauthorized(c *gin.Context, code string, message string) {
	corrID := c.GetString("correlation_id")
	if code == "" {
		code = "AUTH_401"
	}
	c.JSON(http.StatusUnauthorized, JSONResponse{
		Success:       false,
		CorrelationID: corrID,
		Error: &ErrorDetail{
			Code:         code,
			Message:      message,
			RequireLogin: false,
		},
	})
}

func JSON(c *gin.Context, statusCode int, success bool, data interface{}, err *ErrorDetail) {
	corrID := c.GetString("correlation_id")
	c.JSON(statusCode, JSONResponse{
		Success:       success,
		CorrelationID: corrID,
		Data:          data,
		Error:         err,
	})
}

func Success(c *gin.Context, statusCode int, data interface{}) {
	JSON(c, statusCode, true, data, nil)
}

func Error(c *gin.Context, err error) {
	corrID := c.GetString("correlation_id")
	
	var appErr *apperrors.AppError
	var statusCode = http.StatusInternalServerError
	var message = "Internal Server Error"
	
	if errors.As(err, &appErr) {
		statusCode = appErr.Code
		message = appErr.Message
	}
	
	c.JSON(statusCode, JSONResponse{
		Success:       false,
		CorrelationID: corrID,
		Error: &ErrorDetail{
			Code:    fmt.Sprintf("AUTH_%d", statusCode),
			Message: message,
		},
	})
}
