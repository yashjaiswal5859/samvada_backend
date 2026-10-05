// =============================================================================
// Samvada Platform — ABS Phase 4 Billing & Enterprise Integration Tests
// =============================================================================
// Package: tests
// Covers:
//   4.1 Payment Gateway Integration (Stripe & Razorpay)
//   4.2 Subscription Lifecycle (Subscribe, Upgrade, Cancel)
//   4.2 Payment Methods (Card Tokenization, Defaults, Deletion)
//   4.2 Usage Metering & Quota Reconciliation
//   4.2 Invoice Generation, Storage PDF & Gateway Charging
//   4.1 Inbound Gateway Webhook Processing
// =============================================================================

package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/samvada/abs/configs"
	"github.com/samvada/abs/internal/api/handlers"
	"github.com/samvada/abs/internal/api/middleware"
	"github.com/samvada/abs/internal/api/routes"
	appAgent "github.com/samvada/abs/internal/application/agent"
	appBilling "github.com/samvada/abs/internal/application/billing"
	appCampaign "github.com/samvada/abs/internal/application/campaign"
	appKB "github.com/samvada/abs/internal/application/kb"
	appOrg "github.com/samvada/abs/internal/application/organization"
	appTelephony "github.com/samvada/abs/internal/application/telephony"
	appWebhook "github.com/samvada/abs/internal/application/webhook"
	appWS "github.com/samvada/abs/internal/application/workspace"
	"github.com/samvada/abs/internal/domain"
	postgresRepos "github.com/samvada/abs/internal/infrastructure/postgres/repos"
	"github.com/samvada/abs/internal/infrastructure/payment"
	"github.com/samvada/abs/internal/infrastructure/storage"
)

type billingTestHarness struct {
	router         *gin.Engine
	db             *gorm.DB
	orgID          uuid.UUID
	wsID           uuid.UUID
	token          string
	invoiceService appBilling.InvoiceService
	subService     appBilling.SubscriptionService
}

func setupBillingTestEnvironment(t *testing.T) *billingTestHarness {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()

	dsn := "host=localhost user=samvada password=samvada_dev_2026 dbname=samvada_abs port=5432 sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("Skipping test: PostgreSQL is not reachable on localhost:5432: %v", err)
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil || sqlDB.Ping() != nil {
		t.Skipf("Skipping test: PostgreSQL ping failed on localhost:5432")
		return nil
	}

	// Repositories
	orgRepo := postgresRepos.NewOrgRepository(db)
	wsRepo := postgresRepos.NewWorkspaceRepository(db)
	agentRepo := postgresRepos.NewAgentRepository(db)
	billingRepo := postgresRepos.NewBillingRepository(db)
	kbRepo := postgresRepos.NewKBRepository(db)
	campaignRepo := postgresRepos.NewCampaignRepository(db)
	telephonyRepo := postgresRepos.NewTelephonyRepository(db)
	integrationRepo := postgresRepos.NewIntegrationRepository(db)

	// Payment Gateways (Mock sandbox)
	stripeGW := payment.NewStripeGateway("mock_sk_test", "mock_whsec", logger)
	razorpayGW := payment.NewRazorpayGateway("mock_key_id", "mock_key_secret", "mock_rzp_whsec", logger)
	gateways := map[string]payment.PaymentGateway{
		"stripe":   stripeGW,
		"razorpay": razorpayGW,
	}

	// Storage & Processors
	tempStorage, err := storage.NewLocalStorage(t.TempDir(), "http://localhost:8082", logger)
	require.NoError(t, err)

	// Services
	orgService := appOrg.NewOrgService(orgRepo, wsRepo, billingRepo, logger)
	wsService := appWS.NewWorkspaceService(wsRepo, logger)
	agentService := appAgent.NewAgentService(agentRepo, wsRepo, logger)
	walletService := appBilling.NewWalletService(billingRepo, logger)
	subService := appBilling.NewSubscriptionService(billingRepo, orgRepo, gateways, logger)
	meteringService := appBilling.NewUsageMeteringService(billingRepo, nil, logger)
	invoiceService := appBilling.NewInvoiceService(billingRepo, orgRepo, nil, gateways, tempStorage, logger)
	kbService := appKB.NewKBService(kbRepo, tempStorage, nil, logger)
	campaignService := appCampaign.NewCampaignService(campaignRepo, logger)
	telephonyService := appTelephony.NewTelephonyService(telephonyRepo, nil, logger)
	integrationService := appWebhook.NewIntegrationService(integrationRepo, logger)

	// Auth Config & Handlers
	jwtSecret := "phase4-test-jwt-secret-very-secure-32chars"
	cfg := &configs.Config{
		Auth: configs.AuthConfig{JWTSecret: jwtSecret},
		App:  configs.AppConfig{Env: "test"},
	}

	orgHandler := handlers.NewOrgHandler(orgService, walletService)
	wsHandler := handlers.NewWorkspaceHandler(wsService)
	agentHandler := handlers.NewAgentHandler(agentService, wsService, walletService, nil, jwtSecret)
	billingHandler := handlers.NewBillingHandler(walletService, wsService, subService, meteringService, invoiceService)
	kbHandler := handlers.NewKBHandler(kbService)
	campaignHandler := handlers.NewCampaignHandler(campaignService)
	telephonyHandler := handlers.NewTelephonyHandler(telephonyService)
	webhookHandler := handlers.NewWebhookHandler(integrationService)

	router := gin.New()
	authMW := middleware.NewAuthMiddleware(cfg, logger)
	wsMW := middleware.NewWorkspaceMiddleware(db, nil, "test", logger)

	routes.RegisterAPIRoutes(router, routes.RouterDependencies{
		AuthMW:           authMW,
		WorkspaceMW:      wsMW,
		OrgHandler:       orgHandler,
		WorkspaceHandler: wsHandler,
		AgentHandler:     agentHandler,
		BillingHandler:   billingHandler,
		KBHandler:        kbHandler,
		CampaignHandler:  campaignHandler,
		TelephonyHandler: telephonyHandler,
		WebhookHandler:   webhookHandler,
	})

	// Seed Standard Pricing Plans if not present
	plans := []*domain.Plan{
		{Tier: "starter", Name: "Starter", MonthlyPriceUSD: 49.00, MonthlyPriceINR: 3999.00, IncludedMinutes: 250, IncludedAgents: 3, IncludedWorkspaces: 1, OveragePerMinuteUSD: 0.12, OveragePerMinuteINR: 10.0, IsActive: true},
		{Tier: "pro", Name: "Pro Growth", MonthlyPriceUSD: 199.00, MonthlyPriceINR: 15999.00, IncludedMinutes: 1200, IncludedAgents: 10, IncludedWorkspaces: 5, OveragePerMinuteUSD: 0.10, OveragePerMinuteINR: 8.0, IsActive: true},
		{Tier: "enterprise", Name: "Enterprise Scale", MonthlyPriceUSD: 799.00, MonthlyPriceINR: 64999.00, IncludedMinutes: 6000, IncludedAgents: 50, IncludedWorkspaces: 20, OveragePerMinuteUSD: 0.08, OveragePerMinuteINR: 6.5, IsActive: true},
	}
	for _, p := range plans {
		_ = db.Exec("INSERT INTO plans (id, tier, name, monthly_price_usd, monthly_price_inr, included_minutes, included_agents, included_workspaces, overage_per_minute_usd, overage_per_minute_inr, is_active) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (tier) DO NOTHING",
			uuid.New(), p.Tier, p.Name, p.MonthlyPriceUSD, p.MonthlyPriceINR, p.IncludedMinutes, p.IncludedAgents, p.IncludedWorkspaces, p.OveragePerMinuteUSD, p.OveragePerMinuteINR, p.IsActive).Error
	}

	// Create test org and workspace
	testUserID := uuid.New()
	org, ws, _, err := orgService.CreateOrganization(context.Background(), appOrg.CreateOrgRequest{
		Name:          "Phase 4 Enterprise AI Inc",
		Slug:          "phase4-ent-" + uuid.New().String()[:8],
		WorkspaceName: "Production",
		BillingEmail:  "billing@phase4enterprise.ai",
		UserID:        testUserID,
		UserEmail:     "cfo@phase4enterprise.ai",
	})
	require.NoError(t, err)

	_ = db.Exec("INSERT INTO workspace_members (id, workspace_id, user_id, workspace_role) VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING",
		uuid.New(), ws.ID, testUserID.String(), "workspace_admin").Error

	claims := jwt.MapClaims{
		"sub":   testUserID.String(),
		"email": "cfo@phase4enterprise.ai",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token, err := jwtToken.SignedString([]byte(jwtSecret))
	require.NoError(t, err)

	return &billingTestHarness{
		router:         router,
		db:             db,
		orgID:          org.ID,
		wsID:           ws.ID,
		token:          token,
		invoiceService: invoiceService,
		subService:     subService,
	}
}

func TestPhase4_PricingPlansAndSubscriptionLifecycle(t *testing.T) {
	h := setupBillingTestEnvironment(t)

	// 1. List Available Pricing Plans
	reqPlans, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/workspaces/%s/billing/plans", h.wsID), nil)
	reqPlans.Header.Set("Authorization", "Bearer "+h.token)
	wPlans := httptest.NewRecorder()
	h.router.ServeHTTP(wPlans, reqPlans)
	assert.Equal(t, http.StatusOK, wPlans.Code)

	var plansResp struct {
		Data []domain.Plan `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wPlans.Body.Bytes(), &plansResp))
	assert.GreaterOrEqual(t, len(plansResp.Data), 3, "Should have starter, pro, enterprise tiers")

	// 2. Subscribe to 'starter' plan
	subBody := `{"plan_tier":"starter","gateway":"stripe","billing_cycle":"monthly","payment_token":"pm_mock_starter"}`
	reqSub, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/workspaces/%s/billing/subscribe", h.wsID), strings.NewReader(subBody))
	reqSub.Header.Set("Authorization", "Bearer "+h.token)
	reqSub.Header.Set("Content-Type", "application/json")
	wSub := httptest.NewRecorder()
	h.router.ServeHTTP(wSub, reqSub)
	assert.Equal(t, http.StatusOK, wSub.Code)

	var subResp struct {
		Data domain.Subscription `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wSub.Body.Bytes(), &subResp))
	assert.Equal(t, "active", subResp.Data.Status)
	assert.Equal(t, "stripe", subResp.Data.PaymentGateway)
	assert.NotEmpty(t, subResp.Data.GatewayCustomerID)

	// 3. Get Active Subscription
	reqGetSub, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/workspaces/%s/billing/subscription", h.wsID), nil)
	reqGetSub.Header.Set("Authorization", "Bearer "+h.token)
	wGetSub := httptest.NewRecorder()
	h.router.ServeHTTP(wGetSub, reqGetSub)
	assert.Equal(t, http.StatusOK, wGetSub.Code)

	// 4. Upgrade Subscription to 'pro' tier
	upgradeBody := `{"plan_tier":"pro","gateway":"stripe","billing_cycle":"annual"}`
	reqUp, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/workspaces/%s/billing/subscribe", h.wsID), strings.NewReader(upgradeBody))
	reqUp.Header.Set("Authorization", "Bearer "+h.token)
	reqUp.Header.Set("Content-Type", "application/json")
	wUp := httptest.NewRecorder()
	h.router.ServeHTTP(wUp, reqUp)
	assert.Equal(t, http.StatusOK, wUp.Code)

	var upResp struct {
		Data domain.Subscription `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wUp.Body.Bytes(), &upResp))
	assert.Equal(t, "annual", upResp.Data.BillingCycle)

	// 5. Cancel Subscription (at period end)
	reqCancel, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/workspaces/%s/billing/subscription/cancel", h.wsID), nil)
	reqCancel.Header.Set("Authorization", "Bearer "+h.token)
	wCancel := httptest.NewRecorder()
	h.router.ServeHTTP(wCancel, reqCancel)
	assert.Equal(t, http.StatusOK, wCancel.Code)
}

func TestPhase4_PaymentMethodsLifecycle(t *testing.T) {
	h := setupBillingTestEnvironment(t)

	// 1. Add Stripe Payment Method
	pmBody1 := `{"gateway":"stripe","token":"pm_card_visa_4242","make_default":true}`
	reqPM1, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/workspaces/%s/billing/payment-methods", h.wsID), strings.NewReader(pmBody1))
	reqPM1.Header.Set("Authorization", "Bearer "+h.token)
	reqPM1.Header.Set("Content-Type", "application/json")
	wPM1 := httptest.NewRecorder()
	h.router.ServeHTTP(wPM1, reqPM1)
	assert.Equal(t, http.StatusCreated, wPM1.Code)

	var pm1Resp struct {
		Data domain.PaymentMethod `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wPM1.Body.Bytes(), &pm1Resp))
	pm1ID := pm1Resp.Data.ID
	assert.True(t, pm1Resp.Data.IsDefault)

	// 2. Add Razorpay Payment Method
	pmBody2 := `{"gateway":"razorpay","token":"token_rzp_upi_test","make_default":false}`
	reqPM2, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/workspaces/%s/billing/payment-methods", h.wsID), strings.NewReader(pmBody2))
	reqPM2.Header.Set("Authorization", "Bearer "+h.token)
	reqPM2.Header.Set("Content-Type", "application/json")
	wPM2 := httptest.NewRecorder()
	h.router.ServeHTTP(wPM2, reqPM2)
	assert.Equal(t, http.StatusCreated, wPM2.Code)

	var pm2Resp struct {
		Data domain.PaymentMethod `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wPM2.Body.Bytes(), &pm2Resp))
	pm2ID := pm2Resp.Data.ID

	// 3. List Payment Methods
	reqList, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/workspaces/%s/billing/payment-methods", h.wsID), nil)
	reqList.Header.Set("Authorization", "Bearer "+h.token)
	wList := httptest.NewRecorder()
	h.router.ServeHTTP(wList, reqList)
	assert.Equal(t, http.StatusOK, wList.Code)

	var listResp struct {
		Data []domain.PaymentMethod `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wList.Body.Bytes(), &listResp))
	require.Len(t, listResp.Data, 2)

	// 4. Set PM2 as Default
	reqDef, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/workspaces/%s/billing/payment-methods/%s/default", h.wsID, pm2ID), nil)
	reqDef.Header.Set("Authorization", "Bearer "+h.token)
	wDef := httptest.NewRecorder()
	h.router.ServeHTTP(wDef, reqDef)
	assert.Equal(t, http.StatusOK, wDef.Code)

	// 5. Delete PM1
	reqDel, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/workspaces/%s/billing/payment-methods/%s", h.wsID, pm1ID), nil)
	reqDel.Header.Set("Authorization", "Bearer "+h.token)
	wDel := httptest.NewRecorder()
	h.router.ServeHTTP(wDel, reqDel)
	assert.Equal(t, http.StatusOK, wDel.Code)
}

func TestPhase4_UsageMeteringAndInvoicing(t *testing.T) {
	h := setupBillingTestEnvironment(t)

	// 1. Subscribe to 'starter' plan first
	_, err := h.subService.Subscribe(context.Background(), h.orgID, appBilling.SubscribeRequest{
		PlanTier:     "starter",
		Gateway:      "stripe",
		BillingCycle: "monthly",
	})
	require.NoError(t, err)

	// 2. Query Usage & Quota Summary
	reqUsage, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/workspaces/%s/billing/usage", h.wsID), nil)
	reqUsage.Header.Set("Authorization", "Bearer "+h.token)
	wUsage := httptest.NewRecorder()
	h.router.ServeHTTP(wUsage, reqUsage)
	assert.Equal(t, http.StatusOK, wUsage.Code)

	var usageResp struct {
		Data appBilling.QuotaUsageSummary `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wUsage.Body.Bytes(), &usageResp))
	assert.Equal(t, "starter", usageResp.Data.PlanTier)
	assert.Equal(t, 250, usageResp.Data.IncludedMinutes)
	assert.Equal(t, 49.00, usageResp.Data.BasePlanPriceUSD)

	// 3. Generate Monthly Invoice
	periodStart := time.Now().UTC().AddDate(0, -1, 0)
	periodEnd := time.Now().UTC()
	invoice, err := h.invoiceService.GenerateMonthlyInvoice(context.Background(), h.orgID, periodStart, periodEnd)
	require.NoError(t, err)
	assert.NotEmpty(t, invoice.InvoiceNumber)
	assert.Greater(t, invoice.TotalAmount, 0.0)

	// 4. List Historical Invoices via HTTP
	reqInvs, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/workspaces/%s/billing/invoices", h.wsID), nil)
	reqInvs.Header.Set("Authorization", "Bearer "+h.token)
	wInvs := httptest.NewRecorder()
	h.router.ServeHTTP(wInvs, reqInvs)
	assert.Equal(t, http.StatusOK, wInvs.Code)

	var invsResp struct {
		Data []domain.Invoice `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wInvs.Body.Bytes(), &invsResp))
	require.NotEmpty(t, invsResp.Data)
	assert.Equal(t, invoice.InvoiceNumber, invsResp.Data[0].InvoiceNumber)
}

func TestPhase4_PaymentGatewayWebhooks(t *testing.T) {
	h := setupBillingTestEnvironment(t)

	// 1. Stripe Webhook (customer.subscription.updated)
	stripePayload := `{"id":"evt_test_stripe_123","type":"customer.subscription.updated","data":{"object":{"id":"sub_mock_123","status":"active"}}}`
	reqStripe, _ := http.NewRequest(http.MethodPost, "/api/v1/billing/webhooks/stripe", strings.NewReader(stripePayload))
	reqStripe.Header.Set("Content-Type", "application/json")
	wStripe := httptest.NewRecorder()
	h.router.ServeHTTP(wStripe, reqStripe)
	assert.Equal(t, http.StatusOK, wStripe.Code)

	var stripeResp struct {
		Received bool   `json:"received"`
		EventID  string `json:"event_id"`
	}
	require.NoError(t, json.Unmarshal(wStripe.Body.Bytes(), &stripeResp))
	assert.True(t, stripeResp.Received)
	assert.Equal(t, "evt_test_stripe_123", stripeResp.EventID)

	// 2. Razorpay Webhook (subscription.charged)
	rzpPayload := `{"event":"subscription.charged","payload":{"subscription":{"entity":{"id":"sub_rzp_mock_123","status":"active"}}}}`
	reqRzp, _ := http.NewRequest(http.MethodPost, "/api/v1/billing/webhooks/razorpay", strings.NewReader(rzpPayload))
	reqRzp.Header.Set("Content-Type", "application/json")
	wRzp := httptest.NewRecorder()
	h.router.ServeHTTP(wRzp, reqRzp)
	assert.Equal(t, http.StatusOK, wRzp.Code)

	var rzpResp struct {
		Received bool `json:"received"`
	}
	require.NoError(t, json.Unmarshal(wRzp.Body.Bytes(), &rzpResp))
	assert.True(t, rzpResp.Received)
}
