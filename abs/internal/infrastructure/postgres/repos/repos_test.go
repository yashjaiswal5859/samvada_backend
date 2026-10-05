// =============================================================================
// Samvada Platform — ABS Repository Integration Tests
// =============================================================================

package repos_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/samvada/abs/configs"
	"github.com/samvada/abs/internal/domain"
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

func TestOrgAndWorkspaceRepositories(t *testing.T) {
	db := setupTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	orgRepo := repos.NewOrgRepository(db)
	wsRepo := repos.NewWorkspaceRepository(db)

	uniqueSlug := fmt.Sprintf("test-org-%d", time.Now().UnixNano())
	org := &domain.Organization{
		Name:         "Test Enterprise Corp",
		Slug:         uniqueSlug,
		BillingEmail: "billing@testenterprise.com",
	}

	// 1. Create Organization
	err := orgRepo.Create(ctx, org)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, org.ID)

	// 2. Fetch Organization by Slug
	foundOrg, err := orgRepo.GetBySlug(ctx, uniqueSlug)
	require.NoError(t, err)
	assert.Equal(t, org.ID, foundOrg.ID)
	assert.Equal(t, "Test Enterprise Corp", foundOrg.Name)

	// 3. Create Workspace
	wsSlug := fmt.Sprintf("default-ws-%d", time.Now().UnixNano())
	ws := &domain.Workspace{
		OrgID:              org.ID,
		Name:               "Default Workspace",
		Slug:               wsSlug,
		MonthlyMinuteLimit: 10000,
		IsDefault:          true,
	}
	err = wsRepo.Create(ctx, ws)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, ws.ID)

	// 4. Verify Workspace Fetch by ID
	foundWS, err := wsRepo.GetByID(ctx, ws.ID)
	require.NoError(t, err)
	assert.Equal(t, "Default Workspace", foundWS.Name)
	assert.Equal(t, 10000, foundWS.MonthlyMinuteLimit)

	// Cleanup
	_ = wsRepo.Delete(ctx, ws.ID)
	_ = orgRepo.Delete(ctx, org.ID)
}

func TestBillingRepository_DemoCreditsAndDeduction(t *testing.T) {
	db := setupTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	orgRepo := repos.NewOrgRepository(db)
	billingRepo := repos.NewBillingRepository(db)

	// 1. Verify seeded plans exist
	plans, err := billingRepo.ListActivePlans(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(plans), 4, "Should have free, starter, pro, enterprise seeded plans")

	freePlan, err := billingRepo.GetPlanByTier(ctx, "free")
	require.NoError(t, err)
	assert.Equal(t, "Free Trial", freePlan.Name)

	// 2. Create Org and Wallet with $5.00 demo credits
	orgSlug := fmt.Sprintf("wallet-org-%d", time.Now().UnixNano())
	org := &domain.Organization{
		Name:         "Demo Credit Corp",
		Slug:         orgSlug,
		BillingEmail: "finance@democredit.com",
	}
	err = orgRepo.Create(ctx, org)
	require.NoError(t, err)

	wallet := &domain.OrganizationWallet{
		OrgID:                  org.ID,
		BalanceUSD:             5.00,
		PromotionalCreditUSD:   5.00,
		LifetimeCreditAddedUSD: 5.00,
		Currency:               "USD",
	}
	err = billingRepo.CreateWallet(ctx, wallet)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, wallet.ID)

	// 3. Verify Wallet in DB
	foundWallet, err := billingRepo.GetWalletByOrgID(ctx, org.ID)
	require.NoError(t, err)
	assert.Equal(t, 5.00, foundWallet.BalanceUSD)

	// 4. Atomic Deduct $0.05 (e.g. 1 minute call turn)
	updatedWallet, txRecord, err := billingRepo.DeductCredits(
		ctx,
		wallet.ID,
		0.05,
		"Inbound call: 1 minute processed",
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, 4.95, updatedWallet.BalanceUSD)
	assert.Equal(t, "call_deduction", txRecord.Type)
	assert.Equal(t, 4.95, txRecord.BalanceAfterUSD)

	// 5. Verify Wallet Transactions History
	txs, count, err := billingRepo.ListWalletTransactions(ctx, wallet.ID, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count, "Should have 1 signup_promo + 1 call_deduction transaction")
	assert.Equal(t, 2, len(txs))

	// Cleanup
	_ = orgRepo.Delete(ctx, org.ID)
}
