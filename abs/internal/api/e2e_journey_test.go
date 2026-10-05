// =============================================================================
// Samvada Platform — Full End-to-End Enterprise Journey Test
// =============================================================================

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/api/dto"
	appAgent "github.com/samvada/abs/internal/application/agent"
	appBilling "github.com/samvada/abs/internal/application/billing"
	appOrg "github.com/samvada/abs/internal/application/organization"
	appWS "github.com/samvada/abs/internal/application/workspace"
	"github.com/samvada/abs/internal/grpc/server"
	"github.com/samvada/abs/internal/infrastructure/postgres/repos"
	absv1 "github.com/samvada/proto/gen/go/abs/v1"
)

func TestEndToEnd_OnboardingToAgentAndCallAuthorization(t *testing.T) {
	router, db := setupTestApp(t)

	// Step 1: Simulated Authenticated User
	userID := uuid.New()
	userEmail := fmt.Sprintf("enterprise_%d@acme.ai", time.Now().UnixNano())
	token, err := generateTestJWT(userID, userEmail, "user")
	require.NoError(t, err)

	// Step 2: Onboarding - Save Profile
	profileReq := dto.ProfileSetupRequest{
		FirstName:     "Alexander",
		LastName:      &[]string{"Pierce"}[0],
		AccountType:   "business",
		OrgName:       &[]string{"Apex Voice Dynamics"}[0],
		WorkspaceName: "San Francisco HQ",
	}
	body, _ := json.Marshal(profileReq)

	req, _ := http.NewRequest(http.MethodPost, "/api/v1/onboarding/profile", bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var profileResp dto.APIResponse
	err = json.Unmarshal(w.Body.Bytes(), &profileResp)
	require.NoError(t, err)
	var state dto.OnboardingStateResponse
	stateBytes, _ := json.Marshal(profileResp.Data)
	err = json.Unmarshal(stateBytes, &state)
	require.NoError(t, err)

	workspaceID := state.Workspace.ID
	assert.Equal(t, 5.0, state.WalletBalanceUSD)
	assert.Equal(t, 5.0, state.DemoCreditsUSD)
	assert.Equal(t, 83, state.RemainingMinutes)

	// Step 3: Onboarding - Save Industries
	indReq := dto.IndustrySelectionRequest{
		Industries: []string{"Technology & SaaS", "Healthcare & Medical"},
	}
	body, _ = json.Marshal(indReq)
	req, _ = http.NewRequest(http.MethodPost, "/api/v1/onboarding/industries", bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Step 4: Create Agent in Tenant Workspace
	agentReq := dto.CreateAgentRequest{
		Name:          "Apex Concierge",
		Role:          "Customer Support Specialist",
		Description:   &[]string{"Automated 24/7 Voice Support"}[0],
		SystemPrompt:  "You are Apex Concierge, delivering world-class enterprise voice assistance.",
		VoiceID:       "21m00Tcm4TlvDq8ikWAM",
		VoiceProvider: "elevenlabs",
		VoiceName:     "Rachel",
		Language:      "en-US",
		Model:         "gemini-2.5-flash",
	}
	body, _ = json.Marshal(agentReq)
	agentURL := fmt.Sprintf("/api/v1/workspaces/%s/agents", workspaceID)
	req, _ = http.NewRequest(http.MethodPost, agentURL, bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	var agentResp dto.APIResponse
	err = json.Unmarshal(w.Body.Bytes(), &agentResp)
	require.NoError(t, err)
	agentMap := agentResp.Data.(map[string]interface{})
	agentIDStr := agentMap["id"].(string)
	assert.NotEmpty(t, agentIDStr)

	// Step 5: Mint S2S Test WebRTC Ticket Token
	ticketURL := fmt.Sprintf("/api/v1/workspaces/%s/agents/%s/test-token", workspaceID, agentIDStr)
	req, _ = http.NewRequest(http.MethodPost, ticketURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var ticketResp dto.APIResponse
	err = json.Unmarshal(w.Body.Bytes(), &ticketResp)
	require.NoError(t, err)
	ticketMap := ticketResp.Data.(map[string]interface{})
	assert.NotEmpty(t, ticketMap["test_token"])
	assert.Equal(t, agentIDStr, ticketMap["agent_id"])

	// Step 6: gRPC Runtime Authorization Check by S2S
	orgRepo := repos.NewOrgRepository(db)
	wsRepo := repos.NewWorkspaceRepository(db)
	agentRepo := repos.NewAgentRepository(db)
	billingRepo := repos.NewBillingRepository(db)

	agentService := appAgent.NewAgentService(agentRepo, wsRepo, zap.NewNop())
	wsService := appWS.NewWorkspaceService(wsRepo, zap.NewNop())
	walletService := appBilling.NewWalletService(billingRepo, zap.NewNop())
	orgService := appOrg.NewOrgService(orgRepo, wsRepo, billingRepo, zap.NewNop())
	_ = orgService

	grpcRuntime := server.NewAgentRuntimeServer(agentService, wsService, walletService, zap.NewNop())

	authCheck, err := grpcRuntime.CheckCallAuthorization(context.Background(), &absv1.CheckCallAuthorizationRequest{
		WorkspaceId: workspaceID,
		PhoneNumber: "+14155550100",
		Direction:   "inbound",
	})
	require.NoError(t, err)
	assert.True(t, authCheck.GetIsAuthorized())
	assert.Equal(t, 5.0, authCheck.GetCurrentWalletBalanceUsd())
	assert.Equal(t, int64(83), authCheck.GetRemainingMinutes())

	// Step 7: gRPC Runtime Instructions Fetch by S2S
	cfgCheck, err := grpcRuntime.GetAgentRuntimeConfig(context.Background(), &absv1.GetAgentRuntimeConfigRequest{
		AgentId:     agentIDStr,
		WorkspaceId: workspaceID,
	})
	require.NoError(t, err)
	assert.Equal(t, "Apex Concierge", cfgCheck.GetName())
	assert.Equal(t, "You are Apex Concierge, delivering world-class enterprise voice assistance.", cfgCheck.GetSystemPrompt())
	assert.Equal(t, "elevenlabs", cfgCheck.GetVoiceProvider())
	assert.Equal(t, "gemini-2.5-flash", cfgCheck.GetModel())
}
