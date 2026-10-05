// =============================================================================
// Samvada Platform — ABS Agent HTTP Handler
// =============================================================================

package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/samvada/abs/internal/api/dto"
	"github.com/samvada/abs/internal/api/middleware"
	"github.com/samvada/abs/internal/application/agent"
	"github.com/samvada/abs/internal/application/billing"
	"github.com/samvada/abs/internal/application/workspace"
	infraredis "github.com/samvada/abs/internal/infrastructure/redis"
)

type AgentHandler struct {
	agentService     agent.AgentService
	workspaceService workspace.WorkspaceService
	walletService    billing.WalletService
	redisClient      *infraredis.Client
	ticketSecret     []byte
}

func NewAgentHandler(
	agentService agent.AgentService,
	workspaceService workspace.WorkspaceService,
	walletService billing.WalletService,
	redisClient *infraredis.Client,
	ticketSecret string,
) *AgentHandler {
	if ticketSecret == "" {
		ticketSecret = "samvada_ticket_secret_key_2026"
	}
	return &AgentHandler{
		agentService:     agentService,
		workspaceService: workspaceService,
		walletService:    walletService,
		redisClient:      redisClient,
		ticketSecret:     []byte(ticketSecret),
	}
}

func (h *AgentHandler) CreateAgent(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	userIDStr := c.GetString(middleware.ContextUserIDKey)
	userID, _ := uuid.Parse(userIDStr)

	var req dto.CreateAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	ag, err := h.agentService.CreateAgent(c.Request.Context(), wsID, agent.CreateAgentRequest{
		Name:            req.Name,
		Role:            req.Role,
		Description:     req.Description,
		SystemPrompt:    req.SystemPrompt,
		VoiceID:         req.VoiceID,
		VoiceProvider:   req.VoiceProvider,
		VoiceName:       req.VoiceName,
		Language:        req.Language,
		Model:           req.Model,
		AssignedPhone:   req.AssignedPhone,
		CreatedByUserID: userID,
	})
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "AGENT_CREATION_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusCreated, ag)
}

func (h *AgentHandler) ListAgents(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	agents, err := h.agentService.ListAgents(c.Request.Context(), wsID)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, agents)
}

func (h *AgentHandler) GetAgent(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	agentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_AGENT_ID", "Invalid agent UUID format")
		return
	}

	ag, err := h.agentService.GetAgent(c.Request.Context(), wsID, agentID)
	if err != nil {
		RespondError(c, http.StatusNotFound, "AGENT_NOT_FOUND", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, ag)
}

func (h *AgentHandler) PublishVersion(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	agentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_AGENT_ID", "Invalid agent UUID format")
		return
	}

	userIDStr := c.GetString(middleware.ContextUserIDKey)
	userID, _ := uuid.Parse(userIDStr)

	var req struct {
		ChangeSummary string `json:"change_summary"`
	}
	_ = c.ShouldBindJSON(&req)

	version, err := h.agentService.PublishVersion(c.Request.Context(), wsID, agentID, userID, req.ChangeSummary)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "PUBLISH_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, version)
}

func (h *AgentHandler) ListVersions(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	agentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_AGENT_ID", "Invalid agent UUID format")
		return
	}

	versions, err := h.agentService.ListVersions(c.Request.Context(), wsID, agentID)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, versions)
}

func (h *AgentHandler) RollbackVersion(c *gin.Context) {
	wsID, err := uuid.Parse(middleware.GetWorkspaceID(c))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE", "Valid workspace context required")
		return
	}

	agentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_AGENT_ID", "Invalid agent UUID format")
		return
	}

	var req struct {
		VersionNum int `json:"version_num" binding:"required,gt=0"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "Valid version_num required")
		return
	}

	ag, err := h.agentService.RollbackVersion(c.Request.Context(), wsID, agentID, req.VersionNum)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "ROLLBACK_FAILED", err.Error())
		return
	}

	RespondSuccess(c, http.StatusOK, ag)
}

// MintTestToken produces an ephemeral HMAC ticket for live playground call testing (Phase 3.2)
func (h *AgentHandler) MintTestToken(c *gin.Context) {
	wsIDStr := middleware.GetWorkspaceID(c)
	agentIDStr := c.Param("id")

	wsID, err := uuid.Parse(wsIDStr)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_WORKSPACE_ID", "Invalid workspace UUID format")
		return
	}

	remainingCreditUSD := 5.0
	var orgID uuid.UUID

	if h.workspaceService != nil && h.walletService != nil {
		ws, err := h.workspaceService.GetWorkspace(c.Request.Context(), wsID)
		if err != nil {
			RespondError(c, http.StatusNotFound, "WORKSPACE_NOT_FOUND", "Workspace not found")
			return
		}
		orgID = ws.OrgID

		wallet, err := h.walletService.GetWallet(c.Request.Context(), orgID)
		if err != nil {
			RespondError(c, http.StatusInternalServerError, "WALLET_LOOKUP_FAILED", "Failed to retrieve organization wallet")
			return
		}

		if wallet.BalanceUSD <= 0 {
			RespondError(c, http.StatusPaymentRequired, "DEMO_CREDITS_EXHAUSTED", "Demo credits exhausted. Please top up your wallet to continue testing agents.")
			return
		}
		remainingCreditUSD = wallet.BalanceUSD
	}

	now := time.Now().Unix()
	expiresAt := now + 60 // 60-second TTL as per Master Roadmap
	nonce := uuid.New().String()

	// Store single-use nonce in Redis with 60-second TTL
	if h.redisClient != nil {
		_ = h.redisClient.StoreCallTicketNonce(c.Request.Context(), nonce, 60*time.Second)
	}

	sub := c.GetString(middleware.ContextUserIDKey)
	payload := fmt.Sprintf("%s:%s:%s:%s:%d:%s", sub, orgID, wsIDStr, agentIDStr, expiresAt, nonce)
	mac := hmac.New(sha256.New, h.ticketSecret)
	mac.Write([]byte(payload))
	signature := hex.EncodeToString(mac.Sum(nil))

	ticket := fmt.Sprintf("%s.%s", payload, signature)

	RespondSuccess(c, http.StatusOK, gin.H{
		"ticket":               ticket,
		"testToken":            ticket,
		"test_token":           ticket,
		"wssUrl":               fmt.Sprintf("ws://localhost:8084/v1/audio/stream?ticket=%s", ticket),
		"wss_url":              fmt.Sprintf("ws://localhost:8084/v1/audio/stream?ticket=%s", ticket),
		"remainingCreditUSD":   remainingCreditUSD,
		"remaining_credit_usd": remainingCreditUSD,
		"maxDurationSec":       300,
		"max_duration_sec":     300,
		"expiresInSec":         60,
		"expires_in_sec":       60,
		"workspace_id":         wsIDStr,
		"agent_id":             agentIDStr,
	})
}
