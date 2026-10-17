// =============================================================================
// Samvada Platform — ABS Application Services Integration Tests
// =============================================================================

package application_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/samvada/abs/configs"
	"github.com/samvada/abs/internal/application/agent"
	"github.com/samvada/abs/internal/application/billing"
	"github.com/samvada/abs/internal/application/organization"
	"github.com/samvada/abs/internal/application/workspace"
	"github.com/samvada/abs/internal/infrastructure/postgres"
	"github.com/samvada/abs/internal/infrastructure/postgres/repos"
)

func setupTestDB(t *testing.T) *gorm.DB {
	cfg := &configs.DatabaseConfig{
		Host:               "localhost",
		Port:               5432,
		User:               "samvada",
		Password:           "samvada_dev_2026",
		DBName:             "samvada_abs",
		SSLMode:            "disable",
		MaxOpenConns:       5,
		MaxIdleConns:       5,
		ConnMaxLifetimeMin: 5,
	}

	logger := zap.NewNop()
	db, err := postgres.NewConnection(cfg, logger)
	if err != nil {
		t.Skipf("Skipping test: PostgreSQL is not reachable on localhost:5432: %v", err)
		return nil
	}
	return db
}

func TestCompleteEnterpriseOnboardingAndBillingLifecycle(t *testing.T) {
	db := setupTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	log := zap.NewNop()

	orgRepo := repos.NewOrgRepository(db)
	wsRepo := repos.NewWorkspaceRepository(db)
	agentRepo := repos.NewAgentRepository(db)
	billingRepo := repos.NewBillingRepository(db)

	orgService := organization.NewOrgService(orgRepo, wsRepo, billingRepo, log)
	wsService := workspace.NewWorkspaceService(wsRepo, log)
	walletService := billing.NewWalletService(billingRepo, log)
	agentService := agent.NewAgentService(agentRepo, wsRepo, log)

	userID := uuid.New()
	userEmail := fmt.Sprintf("founder-%d@acmeai.io", time.Now().UnixNano())

	// =========================================================================
	// 1. Organization Onboarding (creates Org, default WS, $5 demo wallet, sub)
	// =========================================================================
	orgName := fmt.Sprintf("Acme AI %d", time.Now().UnixNano()%100000)
	org, defaultWS, wallet, err := orgService.CreateOrganization(ctx, organization.CreateOrgRequest{
		Name:         orgName,
		BillingEmail: userEmail,
		UserID:       userID,
		UserEmail:    userEmail,
	})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, org.ID)
	assert.NotEqual(t, uuid.Nil, defaultWS.ID)
	assert.Equal(t, 5.00, wallet.BalanceUSD, "Wallet must be auto-provisioned with $5.00 demo credit")

	// Verify wallet can be retrieved by OrgID
	fetchedWallet, err := walletService.GetWallet(ctx, org.ID)
	require.NoError(t, err)
	assert.Equal(t, 5.00, fetchedWallet.BalanceUSD)

	// =========================================================================
	// 2. Call Authorization Check
	// =========================================================================
	authResult, err := walletService.CheckCallAuthorization(ctx, org.ID)
	require.NoError(t, err)
	assert.True(t, authResult.IsAuthorized)
	assert.Equal(t, 5.00, authResult.CurrentBalanceUSD)
	assert.Equal(t, 83, authResult.EstimatedRemainingMinutes, "5.00 / 0.06 free plan rate = 83 minutes")

	// =========================================================================
	// 3. Atomic Call Cost Deduction
	// =========================================================================
	updatedWallet, txRecord, err := walletService.DeductCallCost(
		ctx,
		org.ID,
		0.05,
		"Live inbound turn: 1 min",
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, 4.95, updatedWallet.BalanceUSD)
	assert.Equal(t, "call_deduction", txRecord.Type)
	assert.Equal(t, 4.95, txRecord.BalanceAfterUSD)

	// =========================================================================
	// 4. Custom Workspace Provisioning
	// =========================================================================
	salesWS, err := wsService.CreateWorkspace(ctx, org.ID, workspace.CreateWorkspaceRequest{
		Name:               "Outbound Sales",
		MonthlyMinuteLimit: 25000,
		CreatorUserID:      userID,
	})
	require.NoError(t, err)
	assert.Equal(t, "Outbound Sales", salesWS.Name)
	assert.Equal(t, 25000, salesWS.MonthlyMinuteLimit)

	// =========================================================================
	// 5. Agent Lifecycle: Create -> Publish -> Rollback
	// =========================================================================
	desc := "Enterprise front-desk receptionist"
	ag, err := agentService.CreateAgent(ctx, salesWS.ID, agent.CreateAgentRequest{
		Name:            "Priya Sharma",
		Role:            "Customer Support Voice Agent",
		Description:     &desc,
		SystemPrompt:    "You are Priya, a courteous and helpful voice agent for Acme AI.",
		VoiceID:         "sarvam-priya",
		VoiceProvider:   "sarvam",
		VoiceName:       "Sarvam Priya",
		Language:        "hi-IN",
		Model:           "gemini-2.5-flash",
		ToolsConfig:     datatypes.JSON(`[]`),
		CreatedByUserID: userID,
	})
	require.NoError(t, err)
	assert.Equal(t, "active", ag.Status)
	assert.Equal(t, 1, ag.ActiveVersionNum)

	// Publish Version 2
	publishedV2, err := agentService.PublishVersion(ctx, salesWS.ID, ag.ID, userID, "Added Hindi language greeting")
	require.NoError(t, err)
	assert.Equal(t, 2, publishedV2.VersionNum)

	// Verify agent status updated to active
	reloadedAgent, err := agentService.GetAgent(ctx, salesWS.ID, ag.ID)
	require.NoError(t, err)
	assert.Equal(t, "active", reloadedAgent.Status)
	assert.Equal(t, 2, reloadedAgent.ActiveVersionNum)

	// Rollback to Version 1
	rolledBackAgent, err := agentService.RollbackVersion(ctx, salesWS.ID, ag.ID, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, rolledBackAgent.ActiveVersionNum)

	// =========================================================================
	// 6. Cleanup
	// =========================================================================
	_ = agentService.DeleteAgent(ctx, salesWS.ID, ag.ID)
	_ = wsService.DeleteWorkspace(ctx, salesWS.ID)
	_ = orgRepo.Delete(ctx, org.ID)
}
