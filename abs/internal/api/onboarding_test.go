// =============================================================================
// Samvada Platform — ABS Onboarding & API Routes Integration Test
// =============================================================================

package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	gormlogger "gorm.io/gorm/logger"

	"github.com/samvada/abs/configs"
	"github.com/samvada/abs/internal/api/dto"
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
	"github.com/samvada/abs/internal/infrastructure/postgres/repos"
)

const testJWTSecret = "samvada_jwt_super_secret_dev_2026_key"

func generateTestJWT(userID uuid.UUID, email, role string) (string, error) {
	claims := jwt.MapClaims{
		"sub":   userID.String(),
		"email": email,
		"role":  role,
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(1 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(testJWTSecret))
}

func setupTestApp(t *testing.T) (*gin.Engine, *gorm.DB) {
	dsn := "host=localhost user=samvada password=samvada_dev_2026 dbname=samvada_abs port=5432 sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Skipf("Skipping test: PostgreSQL is not reachable on localhost:5432: %v", err)
		return nil, nil
	}

	sqlDB, err := db.DB()
	if err != nil || sqlDB.Ping() != nil {
		t.Skipf("Skipping test: PostgreSQL ping failed on localhost:5432")
		return nil, nil
	}

	logger := zap.NewNop()

	cfg := &configs.Config{
		App: configs.AppConfig{
			Env: "test",
		},
		Auth: configs.AuthConfig{
			JWTSecret: testJWTSecret,
		},
	}

	// Repositories
	orgRepo := repos.NewOrgRepository(db)
	wsRepo := repos.NewWorkspaceRepository(db)
	agentRepo := repos.NewAgentRepository(db)
	billingRepo := repos.NewBillingRepository(db)
	kbRepo := repos.NewKBRepository(db)
	campaignRepo := repos.NewCampaignRepository(db)
	telephonyRepo := repos.NewTelephonyRepository(db)
	integrationRepo := repos.NewIntegrationRepository(db)

	// Services
	orgService := appOrg.NewOrgService(orgRepo, wsRepo, billingRepo, logger)
	wsService := appWS.NewWorkspaceService(wsRepo, logger)
	agentService := appAgent.NewAgentService(agentRepo, wsRepo, logger)
	walletService := appBilling.NewWalletService(billingRepo, logger)
	kbService := appKB.NewKBService(kbRepo, nil, nil, logger)
	campaignService := appCampaign.NewCampaignService(campaignRepo, logger)
	telephonyService := appTelephony.NewTelephonyService(telephonyRepo, nil, logger)
	integrationService := appWebhook.NewIntegrationService(integrationRepo, logger)

	// Middleware
	authMW := middleware.NewAuthMiddleware(cfg, logger)
	wsMW := middleware.NewWorkspaceMiddleware(db, nil, "test", logger)

	// Handlers
	orgHandler := handlers.NewOrgHandler(orgService, walletService)
	wsHandler := handlers.NewWorkspaceHandler(wsService)
	agentHandler := handlers.NewAgentHandler(agentService, wsService, walletService, nil, cfg.Auth.JWTSecret)
	billingHandler := handlers.NewBillingHandler(walletService, wsService, nil, nil, nil)
	kbHandler := handlers.NewKBHandler(kbService)
	campaignHandler := handlers.NewCampaignHandler(campaignService)
	telephonyHandler := handlers.NewTelephonyHandler(telephonyService)
	webhookHandler := handlers.NewWebhookHandler(integrationService)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.Recovery())

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

	return router, db
}

func TestPhase1_10_OnboardingFlow(t *testing.T) {
	router, db := setupTestApp(t)

	userID := uuid.New()
	userEmail := fmt.Sprintf("tenant_%d@example.com", time.Now().UnixNano())
	token, err := generateTestJWT(userID, userEmail, "user")
	require.NoError(t, err)

	// Step 1: POST /api/v1/onboarding/profile
	profileReq := dto.ProfileSetupRequest{
		FirstName:     "Elena",
		LastName:      &[]string{"Rostova"}[0],
		AccountType:   "business",
		OrgName:       &[]string{"NovaVoice AI"}[0],
		WorkspaceName: "Production Voice Hub",
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
	assert.True(t, profileResp.Success)

	var state dto.OnboardingStateResponse
	stateBytes, _ := json.Marshal(profileResp.Data)
	err = json.Unmarshal(stateBytes, &state)
	require.NoError(t, err)

	assert.False(t, state.OnboardingCompleted)
	assert.Equal(t, "Elena", state.Profile.FirstName)
	assert.Equal(t, "NovaVoice AI", state.Organization.Name)
	assert.Equal(t, "Production Voice Hub", state.Workspace.Name)
	assert.Equal(t, 5.0, state.WalletBalanceUSD)
	assert.Equal(t, 5.0, state.DemoCreditsUSD)
	assert.Equal(t, 83, state.RemainingMinutes)

	orgID, err := uuid.Parse(state.Organization.ID)
	require.NoError(t, err)

	// Verify Database records created in samvada_abs
	var walletCount int64
	db.Table("organization_wallets").Where("org_id = ?", orgID).Count(&walletCount)
	assert.Equal(t, int64(1), walletCount)

	var txCount int64
	db.Table("wallet_transactions").Where("type = 'signup_promo'").Count(&txCount)
	assert.True(t, txCount >= 1)

	// Step 2: POST /api/v1/onboarding/industries
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

	var indResp dto.APIResponse
	err = json.Unmarshal(w.Body.Bytes(), &indResp)
	require.NoError(t, err)
	assert.True(t, indResp.Success)

	// Step 3: GET /api/v1/onboarding/state
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/onboarding/state", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var finalStateResp dto.APIResponse
	err = json.Unmarshal(w.Body.Bytes(), &finalStateResp)
	require.NoError(t, err)
	assert.True(t, finalStateResp.Success)

	var finalState dto.OnboardingStateResponse
	finalBytes, _ := json.Marshal(finalStateResp.Data)
	err = json.Unmarshal(finalBytes, &finalState)
	require.NoError(t, err)
	assert.True(t, finalState.OnboardingCompleted)
	assert.Equal(t, "NovaVoice AI", finalState.Organization.Name)
}

func TestPhase1_10_OneShotOnboarding(t *testing.T) {
	router, _ := setupTestApp(t)

	userID := uuid.New()
	userEmail := fmt.Sprintf("oneshot_%d@example.com", time.Now().UnixNano())
	token, err := generateTestJWT(userID, userEmail, "user")
	require.NoError(t, err)

	onboardingReq := dto.OnboardingRequest{
		CompanyName:  "OmniCall AI",
		BillingEmail: userEmail,
		Industries:   []string{"Technology & SaaS"},
	}
	body, _ := json.Marshal(onboardingReq)

	req, _ := http.NewRequest(http.MethodPost, "/api/v1/onboarding", bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp dto.APIResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.True(t, resp.Success)

	var obResp dto.OnboardingResponse
	obBytes, _ := json.Marshal(resp.Data)
	err = json.Unmarshal(obBytes, &obResp)
	require.NoError(t, err)

	assert.Equal(t, "OmniCall AI", obResp.OrganizationName)
	assert.Equal(t, "owner", obResp.Role)
	assert.Equal(t, 5.0, obResp.WalletBalanceUSD)
	assert.Equal(t, 5.0, obResp.DemoCreditsUSD)
	assert.Equal(t, 83, obResp.RemainingMinutes)
	assert.Equal(t, "free", obResp.SubscriptionTier)
	assert.NotEmpty(t, obResp.WorkspaceID)
}
