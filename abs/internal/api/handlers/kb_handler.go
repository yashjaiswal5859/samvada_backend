// =============================================================================
// Samvada Platform — ABS Knowledge Base HTTP Handler
// =============================================================================

package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/samvada/abs/internal/api/middleware"
	"github.com/samvada/abs/internal/application/kb"
)

type KBHandler struct {
	kbService kb.KBService
}

func NewKBHandler(kbService kb.KBService) *KBHandler {
	return &KBHandler{
		kbService: kbService,
	}
}

func (h *KBHandler) CreateKB(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	var req struct {
		Name        string  `json:"name" binding:"required"`
		Description *string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	knowledgeBase, err := h.kbService.CreateKB(c.Request.Context(), wsID, req.Name, req.Description)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "KB_CREATION_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusCreated, knowledgeBase)
}

func (h *KBHandler) ListKBs(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	kbs, err := h.kbService.ListKBs(c.Request.Context(), wsID)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, kbs)
}

func (h *KBHandler) GetKB(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	kbID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_KB_ID", "Invalid KB UUID")
		return
	}

	knowledgeBase, err := h.kbService.GetKB(c.Request.Context(), wsID, kbID)
	if err != nil {
		RespondError(c, http.StatusNotFound, "KB_NOT_FOUND", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, knowledgeBase)
}

func (h *KBHandler) DeleteKB(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	kbID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_KB_ID", "Invalid KB UUID")
		return
	}

	if err := h.kbService.DeleteKB(c.Request.Context(), wsID, kbID); err != nil {
		RespondError(c, http.StatusInternalServerError, "DELETE_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, gin.H{"deleted": true, "kb_id": kbID})
}

func (h *KBHandler) AddDocument(c *gin.Context) {
	kbID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_KB_ID", "Invalid KB UUID")
		return
	}

	var req struct {
		Name       string `json:"name" binding:"required"`
		DocType    string `json:"doc_type" binding:"required,oneof=file url text"`
		SourcePath string `json:"source_path" binding:"required"`
		SizeBytes  int64  `json:"size_bytes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	doc, err := h.kbService.AddDocument(c.Request.Context(), kbID, req.Name, req.DocType, req.SourcePath, req.SizeBytes)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "DOC_ADD_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusCreated, doc)
}

func (h *KBHandler) UploadDocument(c *gin.Context) {
	kbID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_KB_ID", "Invalid KB UUID")
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_FILE", "Missing 'file' field in multipart form")
		return
	}

	src, err := fileHeader.Open()
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "FILE_READ_FAILED", err.Error())
		return
	}
	defer src.Close()

	doc, err := h.kbService.UploadAndProcessDocument(
		c.Request.Context(),
		kbID,
		fileHeader.Filename,
		fileHeader.Header.Get("Content-Type"),
		src,
		fileHeader.Size,
	)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "UPLOAD_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusCreated, doc)
}

func (h *KBHandler) ListDocuments(c *gin.Context) {
	kbID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_KB_ID", "Invalid KB UUID")
		return
	}

	docs, err := h.kbService.ListDocuments(c.Request.Context(), kbID)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, docs)
}

func (h *KBHandler) DeleteDocument(c *gin.Context) {
	docID, err := uuid.Parse(c.Param("doc_id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_DOC_ID", "Invalid document UUID")
		return
	}

	if err := h.kbService.DeleteDocument(c.Request.Context(), docID); err != nil {
		RespondError(c, http.StatusInternalServerError, "DELETE_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, gin.H{"deleted": true, "doc_id": docID})
}

func (h *KBHandler) SyncKB(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	kbID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_KB_ID", "Invalid KB UUID")
		return
	}

	if err := h.kbService.SyncKB(c.Request.Context(), wsID, kbID); err != nil {
		RespondError(c, http.StatusInternalServerError, "SYNC_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, gin.H{"synced": true, "kb_id": kbID})
}
