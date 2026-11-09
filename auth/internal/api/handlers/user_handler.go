package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	appuser "github.com/ridesafety/auth-service/internal/application/user"
	"github.com/ridesafety/auth-service/internal/shared/response"
)

type UserHandler struct {
	userService appuser.Service
}

func NewUserHandler(service appuser.Service) *UserHandler {
	return &UserHandler{
		userService: service,
	}
}

func (h *UserHandler) DeleteUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.JSONResponse{
			Success:       false,
			CorrelationID: c.GetString("correlation_id"),
			Error: &response.ErrorDetail{
				Code:    "AUTH_400",
				Message: "Invalid UUID format",
			},
		})
		return
	}

	err = h.userService.DeleteUser(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, http.StatusOK, map[string]interface{}{
		"message": "User deleted successfully",
	})
}
