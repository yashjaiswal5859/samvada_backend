// =============================================================================
// Samvada Platform — ABS Agent Runtime gRPC Server Test
// =============================================================================

package server_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	appAgent "github.com/samvada/abs/internal/application/agent"
	appBilling "github.com/samvada/abs/internal/application/billing"
	appOrg "github.com/samvada/abs/internal/application/organization"
	appWS "github.com/samvada/abs/internal/application/workspace"
	"github.com/samvada/abs/internal/grpc/server"
	"github.com/samvada/abs/internal/infrastructure/postgres/repos"
	absv1 "github.com/samvada/proto/gen/go/abs/v1"
)

func TestAgentRuntimeServer_gRPC(t *testing.T) {
	dsn := "host=localhost user=samvada password=samvada_dev_2026 dbname=samvada_abs port=5432 sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Skipf("Skipping test: PostgreSQL is not reachable on localhost:5432: %v", err)
		return
	}
	sqlDB, err := db.DB()
	if err != nil || sqlDB.Ping() != nil {
		t.Skipf("Skipping test: PostgreSQL ping failed on localhost:5432")
		return
	}

	logger := zap.NewNop()

	orgRepo := repos.NewOrgRepository(db)
	wsRepo := repos.NewWorkspaceRepository(db)
	agentRepo := repos.NewAgentRepository(db)
	billingRepo := repos.NewBillingRepository(db)

	orgService := appOrg.NewOrgService(orgRepo, wsRepo, billingRepo, logger)
	wsService := appWS.NewWorkspaceService(wsRepo, logger)
	agentService := appAgent.NewAgentService(agentRepo, wsRepo, logger)
	walletService := appBilling.NewWalletService(billingRepo, logger)

	grpcServer := server.NewAgentRuntimeServer(agentService, wsService, walletService, logger)

	ctx := context.Background()

	// 1. Provision a tenant organization & workspace
	userID := uuid.New()
	userEmail := fmt.Sprintf("grpc_test_%d@example.com", time.Now().UnixNano())
	org, ws, wallet, err := orgService.CreateOrganization(ctx, appOrg.CreateOrgRequest{
		Name:          "Nexus Voice Solutions",
		WorkspaceName: "Main Center",
		BillingEmail:  userEmail,
		UserID:        userID,
		UserEmail:     userEmail,
	})
	require.NoError(t, err)
	_ = org
	assert.Equal(t, 5.0, wallet.BalanceUSD)

	// 2. Create an agent in this workspace
	desc := "Enterprise Sales AI Agent"
	ag, err := agentService.CreateAgent(ctx, ws.ID, appAgent.CreateAgentRequest{
		Name:          "Samantha",
		Role:          "Inbound Sales Specialist",
		Description:   &desc,
		SystemPrompt:  "You are Samantha, an empathetic voice AI sales representative.",
		VoiceID:       "21m00Tcm4TlvDq8ikWAM",
		VoiceProvider: "elevenlabs",
		VoiceName:     "Rachel",
		Language:      "en-US",
		Model:         "gemini-2.5-flash",
	})
	require.NoError(t, err)

	// 3. Test gRPC GetAgentRuntimeConfig
	configResp, err := grpcServer.GetAgentRuntimeConfig(ctx, &absv1.GetAgentRuntimeConfigRequest{
		AgentId:     ag.ID.String(),
		WorkspaceId: ws.ID.String(),
	})
	require.NoError(t, err)
	assert.Equal(t, ag.ID.String(), configResp.GetAgentId())
	assert.Equal(t, "Samantha", configResp.GetName())
	assert.Equal(t, "You are Samantha, an empathetic voice AI sales representative.", configResp.GetSystemPrompt())
	assert.Equal(t, "elevenlabs", configResp.GetVoiceProvider())
	assert.Equal(t, "21m00Tcm4TlvDq8ikWAM", configResp.GetVoiceId())
	assert.Equal(t, "gemini-2.5-flash", configResp.GetModel())

	// 4. Test gRPC CheckCallAuthorization (Authorized with $5.00 promo demo credits)
	authResp, err := grpcServer.CheckCallAuthorization(ctx, &absv1.CheckCallAuthorizationRequest{
		WorkspaceId: ws.ID.String(),
		PhoneNumber: "+18005550199",
		Direction:   "inbound",
	})
	require.NoError(t, err)
	assert.True(t, authResp.GetIsAuthorized())
	assert.Empty(t, authResp.GetDenialReason())
	assert.Equal(t, 5.0, authResp.GetCurrentWalletBalanceUsd())
	assert.Equal(t, int64(83), authResp.GetRemainingMinutes())

	// 5. Test gRPC CheckCallAuthorization with non-existent workspace
	fakeWSID := uuid.New().String()
	authFailResp, err := grpcServer.CheckCallAuthorization(ctx, &absv1.CheckCallAuthorizationRequest{
		WorkspaceId: fakeWSID,
		PhoneNumber: "+18005550199",
		Direction:   "inbound",
	})
	require.NoError(t, err)
	assert.False(t, authFailResp.GetIsAuthorized())
	assert.Equal(t, "WORKSPACE_NOT_FOUND", authFailResp.GetDenialReason())
}
