// =============================================================================
// Samvada Platform — ABS Agent HTTP Handler
// =============================================================================

package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"gorm.io/datatypes"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/samvada/abs/internal/api/dto"
	"github.com/samvada/abs/internal/api/middleware"
	"github.com/samvada/abs/internal/application/agent"
	"github.com/samvada/abs/internal/application/billing"
	"github.com/samvada/abs/internal/application/catalog"
	"github.com/samvada/abs/internal/application/integration"
	"github.com/samvada/abs/internal/application/kb"
	"github.com/samvada/abs/internal/application/telephony"
	"github.com/samvada/abs/internal/application/workspace"
	"github.com/samvada/abs/internal/domain"
	infraredis "github.com/samvada/abs/internal/infrastructure/redis"
)

type AgentHandler struct {
	agentService     agent.AgentService
	workspaceService workspace.WorkspaceService
	walletService    billing.WalletService
	catalogService   catalog.CatalogService
	kbService        kb.KBService
	connectorService *integration.ConnectorService
	telephonyService telephony.TelephonyService
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

func (h *AgentHandler) SetBuilderDependencies(
	catalogService catalog.CatalogService,
	kbService kb.KBService,
	connectorService *integration.ConnectorService,
	telephonyService telephony.TelephonyService,
) *AgentHandler {
	h.catalogService = catalogService
	h.kbService = kbService
	h.connectorService = connectorService
	h.telephonyService = telephonyService
	return h
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

	// 1. Intelligent defaults
	if req.Name == "" {
		req.Name = "AI Voice Assistant"
	}
	if req.Role == "" {
		req.Role = "Customer Support Specialist"
	}
	if req.SystemPrompt == "" {
		desc := ""
		if req.Description != nil && *req.Description != "" {
			desc = " " + *req.Description
		}
		req.SystemPrompt = fmt.Sprintf("You are %s, a warm and efficient %s.%s Respond to caller requests clearly, empathetically, and concisely.", req.Name, req.Role, desc)
	}
	if req.VoiceID == "" {
		req.VoiceID = "fish-swara-hi-en"
	}
	if req.VoiceProvider == "" {
		req.VoiceProvider = "fish-audio"
	}
	if req.Language == "" {
		req.Language = "en-IN"
	}
	if req.Model == "" {
		req.Model = "gpt-4o-mini"
	}

	// 2. Resolve Phone Number from ID if passed
	if req.AssignedPhone == nil && req.PhoneNumberID != nil && *req.PhoneNumberID != "" {
		if pnID, parseErr := uuid.Parse(*req.PhoneNumberID); parseErr == nil && h.telephonyService != nil {
			if pn, pnErr := h.telephonyService.GetNumber(c.Request.Context(), wsID, pnID); pnErr == nil && pn != nil {
				req.AssignedPhone = &pn.PhoneNumber
			}
		}
	}

	reqJSON, _ := json.Marshal(req)
	var toolsCfg datatypes.JSON
	if len(reqJSON) > 0 {
		toolsCfg = datatypes.JSON(reqJSON)
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
		Status:          req.Status,
		AssignedPhone:   req.AssignedPhone,
		ToolsConfig:     toolsCfg,
		CreatedByUserID: userID,
	})
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "AGENT_CREATION_FAILED", err.Error())
		return
	}

	// 4. Bind phone number to agent in telephony repository if requested
	if req.PhoneNumberID != nil && *req.PhoneNumberID != "" && h.telephonyService != nil {
		if pnID, parseErr := uuid.Parse(*req.PhoneNumberID); parseErr == nil {
			_ = h.telephonyService.AssignAgent(c.Request.Context(), wsID, pnID, ag.ID)
		}
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

func (h *AgentHandler) UpdateAgent(c *gin.Context) {
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

	var req dto.UpdateAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	// 1. Resolve phone number from ID if provided
	if req.PhoneNumberID != nil && *req.PhoneNumberID != "" && h.telephonyService != nil {
		if pnID, parseErr := uuid.Parse(*req.PhoneNumberID); parseErr == nil {
			if pn, pnErr := h.telephonyService.GetNumber(c.Request.Context(), wsID, pnID); pnErr == nil && pn != nil {
				req.AssignedPhone = &pn.PhoneNumber
			}
			_ = h.telephonyService.AssignAgent(c.Request.Context(), wsID, pnID, agentID)
		}
	}

	// 3. Serialize update payload for tools_config in latest version
	reqJSON, _ := json.Marshal(req)
	var toolsCfg *datatypes.JSON
	if len(reqJSON) > 0 {
		tc := datatypes.JSON(reqJSON)
		toolsCfg = &tc
	}

	ag, err := h.agentService.UpdateAgent(c.Request.Context(), wsID, agentID, agent.UpdateAgentRequest{
		Name:          req.Name,
		Role:          req.Role,
		Description:   req.Description,
		AvatarURL:     req.AvatarURL,
		SystemPrompt:  req.SystemPrompt,
		VoiceID:       req.VoiceID,
		VoiceProvider: req.VoiceProvider,
		VoiceName:     req.VoiceName,
		Language:      req.Language,
		Model:         req.Model,
		AssignedPhone: req.AssignedPhone,
		Status:        req.Status,
		ToolsConfig:   toolsCfg,
	})
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "UPDATE_FAILED", err.Error())
		return
	}

	updatedAg, gErr := h.agentService.GetAgent(c.Request.Context(), wsID, agentID)
	if gErr == nil && updatedAg != nil {
		RespondSuccess(c, http.StatusOK, updatedAg)
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

	wssHost := os.Getenv("S2S_PUBLIC_WSS_URL")
	if wssHost == "" {
		host := c.Request.Host
		if host != "" {
			hostName := strings.Split(host, ":")[0]
			wssHost = fmt.Sprintf("ws://%s:8084", hostName)
		} else {
			wssHost = "ws://localhost:8084"
		}
	}
	resolvedWSS := fmt.Sprintf("%s/v1/audio/stream?ticket=%s", strings.TrimRight(wssHost, "/"), ticket)

	RespondSuccess(c, http.StatusOK, gin.H{
		"ticket":               ticket,
		"testToken":            ticket,
		"test_token":           ticket,
		"wssUrl":               resolvedWSS,
		"wss_url":              resolvedWSS,
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

// GetBuilderResources handles GET /api/v1/agents/builder-resources
// Consolidates dynamic models, available phone numbers, workspace knowledge bases,
// and workspace integrations into a single atomic GET response.
func (h *AgentHandler) GetBuilderResources(c *gin.Context) {
	wsIDStr := middleware.GetWorkspaceID(c)
	if wsIDStr == "" {
		wsIDStr = c.GetHeader("X-Workspace-ID")
	}
	if wsIDStr == "" {
		wsIDStr = c.Param("workspace_id")
	}
	if wsIDStr == "" {
		wsIDStr = c.Query("workspace_id")
	}

	var wsID uuid.UUID
	if wsIDStr != "" {
		wsID, _ = uuid.Parse(wsIDStr)
	}

	ctx := c.Request.Context()

	res := dto.AgentBuilderResourcesResponse{
		PhoneNumbers: dto.AgentBuilderPhoneNumbers{
			Available: make([]*domain.PhoneNumber, 0),
			Assigned:  make([]*domain.PhoneNumber, 0),
		},
		STT:            make([]*domain.AIModel, 0),
		TTS:            make([]*domain.AIModel, 0),
		LLM:            make([]*domain.AIModel, 0),
		KnowledgeBases: make([]*domain.KnowledgeBase, 0),
		Integrations:   make([]domain.ConnectorIntegration, 0),
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	// 1. Fetch AI Models (partitioned into separate STT, TTS, LLM)
	if h.catalogService != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			models, err := h.catalogService.ListModelsGrouped(ctx, "", true)
			if err == nil && models != nil {
				mu.Lock()
				if stt, ok := models["stt"]; ok && stt != nil {
					res.STT = stt
				}
				if tts, ok := models["tts"]; ok && tts != nil {
					res.TTS = tts
				}
				if llm, ok := models["llm"]; ok && llm != nil {
					res.LLM = llm
				}
				mu.Unlock()
			}
		}()
	}

	// 2. Fetch Phone Numbers (partitioned into available and assigned)
	wg.Add(1)
	go func() {
		defer wg.Done()
		availableMap := make(map[uuid.UUID]*domain.PhoneNumber)
		assignedMap := make(map[uuid.UUID]*domain.PhoneNumber)

		// 2.1 Workspace numbers (assigned vs unassigned)
		if h.telephonyService != nil && wsID != uuid.Nil {
			if wsNumbers, err := h.telephonyService.ListWorkspaceNumbers(ctx, wsID, ""); err == nil {
				for _, pn := range wsNumbers {
					if pn == nil {
						continue
					}
					if pn.AssignedAgentID != nil || pn.Status == "assigned" {
						assignedMap[pn.ID] = pn
					} else {
						availableMap[pn.ID] = pn
					}
				}
			}
		}

		// 2.2 Platform inventory available numbers
		if h.catalogService != nil {
			if catalogNumbers, err := h.catalogService.ListAvailablePhoneNumbers(ctx, "", 100); err == nil {
				for _, pn := range catalogNumbers {
					if pn == nil {
						continue
					}
					if pn.AssignedAgentID != nil || pn.Status == "assigned" {
						assignedMap[pn.ID] = pn
					} else {
						if _, alreadyAssigned := assignedMap[pn.ID]; !alreadyAssigned {
							availableMap[pn.ID] = pn
						}
					}
				}
			}
		}

		availableList := make([]*domain.PhoneNumber, 0, len(availableMap))
		for _, pn := range availableMap {
			availableList = append(availableList, pn)
		}

		assignedList := make([]*domain.PhoneNumber, 0, len(assignedMap))
		for _, pn := range assignedMap {
			assignedList = append(assignedList, pn)
		}

		mu.Lock()
		res.PhoneNumbers.Available = availableList
		res.PhoneNumbers.Assigned = assignedList
		mu.Unlock()
	}()

	// 3. Fetch Knowledge Bases for workspace
	if h.kbService != nil && wsID != uuid.Nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			kbs, err := h.kbService.ListKBs(ctx, wsID)
			if err == nil && kbs != nil {
				mu.Lock()
				res.KnowledgeBases = kbs
				mu.Unlock()
			}
		}()
	}

	// 4. Fetch Integrations for workspace
	if h.connectorService != nil && wsID != uuid.Nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ints, err := h.connectorService.ListIntegrations(ctx, wsID)
			if err == nil && ints != nil {
				mu.Lock()
				res.Integrations = ints
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	RespondSuccess(c, http.StatusOK, res)
}

