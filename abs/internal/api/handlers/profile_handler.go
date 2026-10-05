// =============================================================================
// Samvada Platform — ABS User Profile HTTP Handler
// =============================================================================

package handlers

import (
	"io"
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/api/middleware"
	"github.com/samvada/abs/internal/application/profile"
	apperrors "github.com/samvada/proto/pkg/errors"
)

type ProfileHandler struct {
	service *profile.ProfileService
	log     *zap.Logger
}

func NewProfileHandler(service *profile.ProfileService, log *zap.Logger) *ProfileHandler {
	return &ProfileHandler{
		service: service,
		log:     log.Named("profile_handler"),
	}
}

// GetMe godoc
// @Summary Fetch current authenticated user session, profile, and tenant context directly from ABS
// @Tags User Profile
// @Produce json
// @Success 200 {object} profile.UserMeResponse
// @Router /api/v1/user/me [get]
func (h *ProfileHandler) GetMe(c *gin.Context) {
	userIDStr := middleware.GetUserID(c)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error": gin.H{"code": apperrors.CodeUnauthorized, "message": "Invalid user token"},
		})
		return
	}
	userEmail := c.GetString(middleware.ContextUserEmailKey)

	me, err := h.service.GetMe(c.Request.Context(), userID, userEmail)
	if err != nil {
		h.log.Error("Failed to fetch user me", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{"code": apperrors.CodeInternal, "message": "Failed to fetch user session"},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    me,
	})
}

// GetProfile godoc
// @Summary Fetch personal user profile
// @Tags User Profile
// @Produce json
// @Success 200 {object} domain.UserProfile
// @Router /api/v1/user/profile [get]
func (h *ProfileHandler) GetProfile(c *gin.Context) {
	userIDStr := middleware.GetUserID(c)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error": gin.H{"code": apperrors.CodeUnauthorized, "message": "Invalid user token"},
		})
		return
	}

	p, err := h.service.GetProfile(c.Request.Context(), userID)
	if err != nil {
		h.log.Error("Failed to fetch user profile", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{"code": apperrors.CodeInternal, "message": "Failed to fetch user profile"},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    p,
	})
}

// UpdateProfile godoc
// @Summary Update personal user profile
// @Tags User Profile
// @Accept json
// @Produce json
// @Success 200 {object} domain.UserProfile
// @Router /api/v1/user/profile [patch]
func (h *ProfileHandler) UpdateProfile(c *gin.Context) {
	userIDStr := middleware.GetUserID(c)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error": gin.H{"code": apperrors.CodeUnauthorized, "message": "Invalid user token"},
		})
		return
	}

	var req profile.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{"code": apperrors.CodeInvalidInput, "message": err.Error()},
		})
		return
	}

	p, err := h.service.UpdateProfile(c.Request.Context(), userID, req)
	if err != nil {
		h.log.Error("Failed to update user profile", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{"code": apperrors.CodeInternal, "message": "Failed to update user profile"},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    p,
	})
}

// UploadAvatar godoc
// @Summary Upload user profile photo
// @Tags User Profile
// @Accept multipart/form-data
// @Produce json
// @Success 200 {object} map[string]string
// @Router /api/v1/user/profile/avatar [post]
func (h *ProfileHandler) UploadAvatar(c *gin.Context) {
	userIDStr := middleware.GetUserID(c)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error": gin.H{"code": apperrors.CodeUnauthorized, "message": "Invalid user token"},
		})
		return
	}

	file, header, err := c.Request.FormFile("avatar")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{"code": apperrors.CodeInvalidInput, "message": "Missing avatar form file field"},
		})
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{"code": apperrors.CodeInternal, "message": "Failed to read file"},
		})
		return
	}

	ext := filepath.Ext(header.Filename)
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}

	avatarURL, err := h.service.UploadAvatar(c.Request.Context(), userID, data, ext, contentType)
	if err != nil {
		h.log.Error("Failed to upload avatar", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{"code": apperrors.CodeInternal, "message": "Failed to upload avatar"},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"avatar_url": avatarURL,
		},
	})
}

// ExportData godoc
// @Summary Export all user data under GDPR Article 20
// @Tags User Profile
// @Produce json
// @Success 200 {object} domain.GDPRDataExport
// @Router /api/v1/user/data-export [get]
func (h *ProfileHandler) ExportData(c *gin.Context) {
	userIDStr := middleware.GetUserID(c)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   gin.H{"code": apperrors.CodeUnauthorized, "message": "Invalid user token"},
		})
		return
	}

	data, err := h.service.ExportData(c.Request.Context(), userID)
	if err != nil {
		h.log.Error("Failed to generate GDPR data export", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   gin.H{"code": apperrors.CodeInternal, "message": "Failed to generate GDPR data export"},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
	})
}

// DeleteData godoc
// @Summary Request full erasure of user data under GDPR Article 17
// @Tags User Profile
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/user/data-deletion [delete]
func (h *ProfileHandler) DeleteData(c *gin.Context) {
	userIDStr := middleware.GetUserID(c)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   gin.H{"code": apperrors.CodeUnauthorized, "message": "Invalid user token"},
		})
		return
	}

	if err := h.service.DeleteData(c.Request.Context(), userID); err != nil {
		h.log.Error("Failed to execute GDPR data erasure", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   gin.H{"code": apperrors.CodeInternal, "message": "Failed to process data erasure"},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "User personal data successfully purged in accordance with GDPR Article 17 (Right to Erasure)",
	})
}
