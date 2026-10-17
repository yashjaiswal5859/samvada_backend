// =============================================================================
// Samvada Platform — ABS AI Catalog & Phone Inventory HTTP Handlers
// =============================================================================

package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/samvada/abs/internal/application/catalog"
	"github.com/samvada/abs/internal/domain"
)

type CatalogHandler struct {
	service catalog.CatalogService
}

func NewCatalogHandler(service catalog.CatalogService) *CatalogHandler {
	return &CatalogHandler{
		service: service,
	}
}

// UploadPhoneNumbers handles POST /api/v1/admin/catalog/phone-numbers
// Supports both raw list format: [ { "phone_number": "..." }, ... ]
// and wrapped object: { "items": [ ... ] } or { "phone_numbers": [ ... ] }
func (h *CatalogHandler) UploadPhoneNumbers(c *gin.Context) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil || len(bodyBytes) == 0 {
		RespondError(c, http.StatusBadRequest, "EMPTY_PAYLOAD", "Request body cannot be empty")
		return
	}

	var inputs []domain.PhoneNumberInput

	// 1. Try parsing as direct JSON list/array
	if err := json.Unmarshal(bodyBytes, &inputs); err != nil {
		// 2. Try parsing as wrapped object
		var wrapped struct {
			PhoneNumbers []domain.PhoneNumberInput `json:"phone_numbers"`
			Items        []domain.PhoneNumberInput `json:"items"`
			Numbers      []domain.PhoneNumberInput `json:"numbers"`
		}
		if wrapErr := json.Unmarshal(bodyBytes, &wrapped); wrapErr != nil {
			// 3. Try parsing as a single object
			var single domain.PhoneNumberInput
			if singleErr := json.Unmarshal(bodyBytes, &single); singleErr == nil && single.PhoneNumber != "" {
				inputs = []domain.PhoneNumberInput{single}
			} else {
				RespondError(c, http.StatusBadRequest, "INVALID_FORMAT", "Payload must be a JSON array of phone numbers: [{...}] or {\"items\": [...]}")
				return
			}
		} else {
			if len(wrapped.PhoneNumbers) > 0 {
				inputs = wrapped.PhoneNumbers
			} else if len(wrapped.Items) > 0 {
				inputs = wrapped.Items
			} else {
				inputs = wrapped.Numbers
			}
		}
	}

	if len(inputs) == 0 {
		RespondError(c, http.StatusBadRequest, "NO_ITEMS", "No phone numbers provided in array")
		return
	}

	res, err := h.service.BulkUploadPhoneNumbers(c.Request.Context(), inputs)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "UPLOAD_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusCreated, res)
}

// UploadModels handles POST /api/v1/admin/catalog/models (general endpoint)
func (h *CatalogHandler) UploadModels(c *gin.Context) {
	h.processModelUpload(c, "")
}

// UploadSTTModels handles POST /api/v1/admin/catalog/stt
func (h *CatalogHandler) UploadSTTModels(c *gin.Context) {
	h.processModelUpload(c, "stt")
}

// UploadTTSModels handles POST /api/v1/admin/catalog/tts
func (h *CatalogHandler) UploadTTSModels(c *gin.Context) {
	h.processModelUpload(c, "tts")
}

// UploadLLMModels handles POST /api/v1/admin/catalog/llm
func (h *CatalogHandler) UploadLLMModels(c *gin.Context) {
	h.processModelUpload(c, "llm")
}

func (h *CatalogHandler) processModelUpload(c *gin.Context, categoryOverride string) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil || len(bodyBytes) == 0 {
		RespondError(c, http.StatusBadRequest, "EMPTY_PAYLOAD", "Request body cannot be empty")
		return
	}

	var inputs []domain.AIModelInput

	// 1. Try direct JSON array
	if err := json.Unmarshal(bodyBytes, &inputs); err != nil {
		// 2. Try wrapped object
		var wrapped struct {
			Models []domain.AIModelInput `json:"models"`
			Items  []domain.AIModelInput `json:"items"`
			STT    []domain.AIModelInput `json:"stt"`
			TTS    []domain.AIModelInput `json:"tts"`
			LLM    []domain.AIModelInput `json:"llm"`
		}
		if wrapErr := json.Unmarshal(bodyBytes, &wrapped); wrapErr != nil {
			// 3. Try single model object
			var single domain.AIModelInput
			if singleErr := json.Unmarshal(bodyBytes, &single); singleErr == nil && single.ID != "" {
				inputs = []domain.AIModelInput{single}
			} else {
				RespondError(c, http.StatusBadRequest, "INVALID_FORMAT", "Payload must be a JSON array of models: [{...}] or {\"items\": [...]}")
				return
			}
		} else {
			if len(wrapped.Models) > 0 {
				inputs = wrapped.Models
			} else if len(wrapped.Items) > 0 {
				inputs = wrapped.Items
			} else if len(wrapped.STT) > 0 && categoryOverride == "stt" {
				inputs = wrapped.STT
			} else if len(wrapped.TTS) > 0 && categoryOverride == "tts" {
				inputs = wrapped.TTS
			} else if len(wrapped.LLM) > 0 && categoryOverride == "llm" {
				inputs = wrapped.LLM
			}
		}
	}

	if len(inputs) == 0 {
		RespondError(c, http.StatusBadRequest, "NO_ITEMS", "No models provided in array")
		return
	}

	res, err := h.service.BulkUploadModels(c.Request.Context(), categoryOverride, inputs)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "UPLOAD_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusCreated, res)
}

// ListModels handles GET /api/v1/catalog/models
func (h *CatalogHandler) ListModels(c *gin.Context) {
	category := c.Query("category")
	activeOnly := c.DefaultQuery("active_only", "true") == "true"
	flat := c.Query("flat") == "true"

	if flat || category != "" {
		list, err := h.service.ListModelsFlat(c.Request.Context(), category, activeOnly)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		RespondSuccess(c, http.StatusOK, list)
		return
	}

	grouped, err := h.service.ListModelsGrouped(c.Request.Context(), category, activeOnly)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, grouped)
}

// ListPhoneNumbers handles GET /api/v1/catalog/phone-numbers
func (h *CatalogHandler) ListPhoneNumbers(c *gin.Context) {
	search := c.Query("search")
	limitStr := c.DefaultQuery("limit", "100")
	limit, _ := strconv.Atoi(limitStr)

	numbers, err := h.service.ListAvailablePhoneNumbers(c.Request.Context(), search, limit)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, numbers)
}

// DeleteModel handles DELETE /api/v1/admin/catalog/models/:id
func (h *CatalogHandler) DeleteModel(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		RespondError(c, http.StatusBadRequest, "MISSING_ID", "Model ID is required")
		return
	}

	if err := h.service.DeleteModel(c.Request.Context(), id); err != nil {
		RespondError(c, http.StatusInternalServerError, "DELETE_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, gin.H{"deleted": true, "id": id})
}

// DeletePhoneNumber handles DELETE /api/v1/admin/catalog/phone-numbers/:id
func (h *CatalogHandler) DeletePhoneNumber(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "Valid UUID is required")
		return
	}

	if err := h.service.DeletePhoneNumber(c.Request.Context(), id); err != nil {
		RespondError(c, http.StatusInternalServerError, "DELETE_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, gin.H{"deleted": true, "id": id})
}
