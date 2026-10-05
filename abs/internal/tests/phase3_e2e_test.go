// =============================================================================
// Samvada Platform — ABS Phase 3 End-to-End Integration Tests
// =============================================================================
// Package: tests
// Covers:
//   3.1 S2S Ephemeral Token Minting & Deduction
//   3.5 Audience & Contact Management + CSV Deduplication
//   3.6 Telephony DID Carrier Provisioning & Lifecycle (Twilio/Plivo)
//   3.7 Knowledge Base & Async RAG Processing Pipeline
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
	"github.com/samvada/abs/configs"
	"github.com/samvada/abs/internal/domain"
	postgresRepos "github.com/samvada/abs/internal/infrastructure/postgres/repos"
	"github.com/samvada/abs/internal/infrastructure/storage"
	infraTelephony "github.com/samvada/abs/internal/infrastructure/telephony"
)

func setupTestEnvironment(t *testing.T) (*gin.Engine, *gorm.DB, uuid.UUID, uuid.UUID, string) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()

	dsn := "host=localhost user=samvada password=samvada_dev_2026 dbname=samvada_abs port=5432 sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("Skipping test: PostgreSQL is not reachable on localhost:5432: %v", err)
		return nil, nil, uuid.Nil, uuid.Nil, ""
	}
	sqlDB, err := db.DB()
	if err != nil || sqlDB.Ping() != nil {
		t.Skipf("Skipping test: PostgreSQL ping failed on localhost:5432")
		return nil, nil, uuid.Nil, uuid.Nil, ""
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

	// Storage & Processors
	tempStorage, err := storage.NewLocalStorage(t.TempDir(), "http://localhost:8082", logger)
	require.NoError(t, err)

	docProcessor := appKB.NewDocumentProcessor(kbRepo, tempStorage, logger, 2)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	docProcessor.Start(ctx)

	// Telephony Sandbox Carriers
	carriers := map[string]infraTelephony.CarrierAdapter{
		"twilio": infraTelephony.NewTwilioAdapter("mock_sid", "mock_token", logger),
		"plivo":  infraTelephony.NewPlivoAdapter("mock_auth_id", "mock_token", logger),
	}

	// Services
	orgService := appOrg.NewOrgService(orgRepo, wsRepo, billingRepo, logger)
	wsService := appWS.NewWorkspaceService(wsRepo, logger)
	agentService := appAgent.NewAgentService(agentRepo, wsRepo, logger)
	walletService := appBilling.NewWalletService(billingRepo, logger)
	kbService := appKB.NewKBService(kbRepo, tempStorage, docProcessor, logger)
	campaignService := appCampaign.NewCampaignService(campaignRepo, logger)
	telephonyService := appTelephony.NewTelephonyService(telephonyRepo, carriers, logger)
	integrationService := appWebhook.NewIntegrationService(integrationRepo, logger)

	// Handlers
	jwtSecret := "phase3-test-jwt-secret-very-secure-32chars"
	cfg := &configs.Config{
		Auth: configs.AuthConfig{JWTSecret: jwtSecret},
		App:  configs.AppConfig{Env: "test"},
	}

	orgHandler := handlers.NewOrgHandler(orgService, walletService)
	wsHandler := handlers.NewWorkspaceHandler(wsService)
	agentHandler := handlers.NewAgentHandler(agentService, wsService, walletService, nil, jwtSecret)
	billingHandler := handlers.NewBillingHandler(walletService, wsService, nil, nil, nil)
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

	// Setup Test Org, Workspace, and Admin Member
	testUserID := uuid.New()
	org, ws, _, err := orgService.CreateOrganization(context.Background(), appOrg.CreateOrgRequest{
		Name:          "Phase 3 Test Org",
		Slug:          "phase3-org-" + uuid.New().String()[:8],
		WorkspaceName: "Default",
		BillingEmail:  "billing@phase3.test",
		UserID:        testUserID,
		UserEmail:     "admin@phase3.test",
	})
	require.NoError(t, err)

	// Ensure workspace membership is present
	_ = db.Exec("INSERT INTO workspace_members (id, workspace_id, user_id, workspace_role) VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING",
		uuid.New(), ws.ID, testUserID.String(), "workspace_admin").Error

	// Mint JWT token for auth header
	claims := jwt.MapClaims{
		"sub":   testUserID.String(),
		"email": "admin@phase3.test",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token, err := jwtToken.SignedString([]byte(jwtSecret))
	require.NoError(t, err)

	return router, db, org.ID, ws.ID, token
}

func TestPhase3_AudienceAndContactManagement(t *testing.T) {
	router, _, _, wsID, token := setupTestEnvironment(t)

	// 1. Create Audience
	audienceBody := `{"name":"VIP Healthcare Leads","description":"High-priority outreach"}`
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/workspaces/%s/audiences", wsID), strings.NewReader(audienceBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	var audResp struct {
		Data struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &audResp))
	audID := audResp.Data.ID
	assert.NotEmpty(t, audID)

	// 2. Batch Import CSV with Deduplication
	csvContent := `name,phone,email,company
Dr. Rajesh Sharma,+919876543210,rajesh@clinic.com,Apollo Clinic
Ananya Verma,09876543211,ananya@med.org,Max Health
Duplicate Rajesh,+919876543210,duplicate@clinic.com,Duplicate Entry
`
	reqCSV, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/workspaces/%s/audiences/%s/batch-import", wsID, audID), strings.NewReader(csvContent))
	reqCSV.Header.Set("Authorization", "Bearer "+token)
	reqCSV.Header.Set("Content-Type", "text/csv")
	wCSV := httptest.NewRecorder()
	router.ServeHTTP(wCSV, reqCSV)
	assert.Equal(t, http.StatusOK, wCSV.Code)

	var batchResp struct {
		Data struct {
			ImportedCount int `json:"imported_count"`
			SkippedCount  int `json:"skipped_count"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wCSV.Body.Bytes(), &batchResp))
	assert.Equal(t, 2, batchResp.Data.ImportedCount, "Should import 2 unique contacts")
	assert.Equal(t, 1, batchResp.Data.SkippedCount, "Should skip 1 duplicate phone")

	// 3. List Contacts
	reqList, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/workspaces/%s/audiences/%s/contacts", wsID, audID), nil)
	reqList.Header.Set("Authorization", "Bearer "+token)
	wList := httptest.NewRecorder()
	router.ServeHTTP(wList, reqList)
	assert.Equal(t, http.StatusOK, wList.Code)

	var listResp struct {
		Data struct {
			Contacts []domain.Contact `json:"contacts"`
			Total    int              `json:"total_count"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wList.Body.Bytes(), &listResp))
	assert.Equal(t, 2, listResp.Data.Total)
	require.Len(t, listResp.Data.Contacts, 2)

	// 4. Delete Single Contact
	contactID := listResp.Data.Contacts[0].ID
	reqDel, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/workspaces/%s/audiences/%s/contacts/%s", wsID, audID, contactID.String()), nil)
	reqDel.Header.Set("Authorization", "Bearer "+token)
	wDel := httptest.NewRecorder()
	router.ServeHTTP(wDel, reqDel)
	assert.Equal(t, http.StatusOK, wDel.Code)

	// 5. Delete Entire Audience
	reqDelAud, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/workspaces/%s/audiences/%s", wsID, audID), nil)
	reqDelAud.Header.Set("Authorization", "Bearer "+token)
	wDelAud := httptest.NewRecorder()
	router.ServeHTTP(wDelAud, reqDelAud)
	assert.Equal(t, http.StatusOK, wDelAud.Code)
}

func TestPhase3_TelephonyCarrierLifecycle(t *testing.T) {
	router, db, _, wsID, token := setupTestEnvironment(t)

	// 1. Search Available Phone Numbers
	reqSearch, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/workspaces/%s/phone-numbers/available?provider=twilio&country=US&area_code=415&limit=5", wsID), nil)
	reqSearch.Header.Set("Authorization", "Bearer "+token)
	wSearch := httptest.NewRecorder()
	router.ServeHTTP(wSearch, reqSearch)
	assert.Equal(t, http.StatusOK, wSearch.Code)

	var searchResp struct {
		Data struct {
			Provider string                          `json:"provider"`
			Numbers  []infraTelephony.AvailableNumber `json:"numbers"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wSearch.Body.Bytes(), &searchResp))
	assert.Equal(t, "twilio", searchResp.Data.Provider)
	require.NotEmpty(t, searchResp.Data.Numbers)
	chosenNumber := searchResp.Data.Numbers[0].PhoneNumber

	// 2. Purchase Chosen Number
	purchaseBody := fmt.Sprintf(`{"provider":"twilio","phone_number":"%s","webhook_url":"https://api.samvada.ai/webhook/voice"}`, chosenNumber)
	reqPurchase, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/workspaces/%s/phone-numbers/purchase", wsID), strings.NewReader(purchaseBody))
	reqPurchase.Header.Set("Authorization", "Bearer "+token)
	reqPurchase.Header.Set("Content-Type", "application/json")
	wPurchase := httptest.NewRecorder()
	router.ServeHTTP(wPurchase, reqPurchase)
	assert.Equal(t, http.StatusCreated, wPurchase.Code)

	var purchaseResp struct {
		Data struct {
			ID          string `json:"id"`
			PhoneNumber string `json:"phone_number"`
			Provider    string `json:"provider"`
			Status      string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wPurchase.Body.Bytes(), &purchaseResp))
	numID := purchaseResp.Data.ID
	assert.Equal(t, chosenNumber, purchaseResp.Data.PhoneNumber)
	assert.Equal(t, "active", purchaseResp.Data.Status)

	// 3. Assign Number to Agent
	testAgent := &domain.Agent{
		ID:           uuid.New(),
		WorkspaceID:  wsID,
		Name:         "Call Routing Agent",
		Role:         "Specialist",
		SystemPrompt: "You are a test call receptionist",
		VoiceID:      "mock-voice",
		VoiceName:    "Rachel",
		Language:     "en-US",
		Model:        "gemini-2.5-flash",
		Status:       "active",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	require.NoError(t, db.Create(testAgent).Error)

	assignBody := fmt.Sprintf(`{"agent_id":"%s"}`, testAgent.ID.String())
	reqAssign, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/workspaces/%s/phone-numbers/%s/assign", wsID, numID), strings.NewReader(assignBody))
	reqAssign.Header.Set("Authorization", "Bearer "+token)
	reqAssign.Header.Set("Content-Type", "application/json")
	wAssign := httptest.NewRecorder()
	router.ServeHTTP(wAssign, reqAssign)
	assert.Equal(t, http.StatusOK, wAssign.Code)

	// 4. Unassign Number
	reqUnassign, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/workspaces/%s/phone-numbers/%s/unassign", wsID, numID), nil)
	reqUnassign.Header.Set("Authorization", "Bearer "+token)
	wUnassign := httptest.NewRecorder()
	router.ServeHTTP(wUnassign, reqUnassign)
	assert.Equal(t, http.StatusOK, wUnassign.Code)

	// 5. Release Number
	reqRelease, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/workspaces/%s/phone-numbers/%s", wsID, numID), nil)
	reqRelease.Header.Set("Authorization", "Bearer "+token)
	wRelease := httptest.NewRecorder()
	router.ServeHTTP(wRelease, reqRelease)
	assert.Equal(t, http.StatusOK, wRelease.Code)
}

func TestPhase3_KnowledgeBaseAndRAGLifecycle(t *testing.T) {
	router, _, _, wsID, token := setupTestEnvironment(t)

	// 1. Create Knowledge Base
	kbBody := `{"name":"Customer FAQs & Product Manual","description":"Knowledge base for Voice AI agent"}`
	reqKB, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/workspaces/%s/kb", wsID), strings.NewReader(kbBody))
	reqKB.Header.Set("Authorization", "Bearer "+token)
	reqKB.Header.Set("Content-Type", "application/json")
	wKB := httptest.NewRecorder()
	router.ServeHTTP(wKB, reqKB)
	assert.Equal(t, http.StatusCreated, wKB.Code)

	var kbResp struct {
		Data struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wKB.Body.Bytes(), &kbResp))
	kbID := kbResp.Data.ID
	assert.NotEmpty(t, kbID)

	// 2. Add Direct Text Document
	docBody := `{"name":"Cancellation Policy","doc_type":"text","source_path":"Orders can be cancelled within 24 hours of placement for a 100% full refund.\n\nAfter 24 hours, a 10% restocking fee applies to hardware returns.\n\nDigital subscriptions may be cancelled anytime with pro-rated credit.","size_bytes":210}`
	reqDoc, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/workspaces/%s/kb/%s/documents", wsID, kbID), strings.NewReader(docBody))
	reqDoc.Header.Set("Authorization", "Bearer "+token)
	reqDoc.Header.Set("Content-Type", "application/json")
	wDoc := httptest.NewRecorder()
	router.ServeHTTP(wDoc, reqDoc)
	assert.Equal(t, http.StatusCreated, wDoc.Code)

	var docResp struct {
		Data struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wDoc.Body.Bytes(), &docResp))
	docID := docResp.Data.ID
	assert.NotEmpty(t, docID)

	// Allow async background processor to run chunking and indexing
	time.Sleep(300 * time.Millisecond)

	// 3. List Documents and check indexed status
	reqDocs, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/workspaces/%s/kb/%s/documents", wsID, kbID), nil)
	reqDocs.Header.Set("Authorization", "Bearer "+token)
	wDocs := httptest.NewRecorder()
	router.ServeHTTP(wDocs, reqDocs)
	assert.Equal(t, http.StatusOK, wDocs.Code)

	var docsListResp struct {
		Data []domain.KBDocument `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wDocs.Body.Bytes(), &docsListResp))
	require.NotEmpty(t, docsListResp.Data)
	assert.Equal(t, "indexed", docsListResp.Data[0].Status)
	assert.GreaterOrEqual(t, docsListResp.Data[0].ChunkCount, 1)

	// 4. Trigger Sync
	reqSync, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/workspaces/%s/kb/%s/sync", wsID, kbID), nil)
	reqSync.Header.Set("Authorization", "Bearer "+token)
	wSync := httptest.NewRecorder()
	router.ServeHTTP(wSync, reqSync)
	assert.Equal(t, http.StatusOK, wSync.Code)

	// 5. Delete Document
	reqDelDoc, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/workspaces/%s/kb/%s/documents/%s", wsID, kbID, docID), nil)
	reqDelDoc.Header.Set("Authorization", "Bearer "+token)
	wDelDoc := httptest.NewRecorder()
	router.ServeHTTP(wDelDoc, reqDelDoc)
	assert.Equal(t, http.StatusOK, wDelDoc.Code)

	// 6. Delete Knowledge Base
	reqDelKB, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/workspaces/%s/kb/%s", wsID, kbID), nil)
	reqDelKB.Header.Set("Authorization", "Bearer "+token)
	wDelKB := httptest.NewRecorder()
	router.ServeHTTP(wDelKB, reqDelKB)
	assert.Equal(t, http.StatusOK, wDelKB.Code)
}

func TestPhase3_EphemeralTicketAndWalletDeduction(t *testing.T) {
	router, db, orgID, wsID, token := setupTestEnvironment(t)

	// 1. Create Agent
	agentBody := `{"name":"Customer Support Voice Agent","role":"Support Specialist","language":"en-US","voice_id":"voice-rachel-elevenlabs","system_prompt":"You are a helpful customer support agent."}`
	reqAgent, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/workspaces/%s/agents", wsID), strings.NewReader(agentBody))
	reqAgent.Header.Set("Authorization", "Bearer "+token)
	reqAgent.Header.Set("Content-Type", "application/json")
	wAgent := httptest.NewRecorder()
	router.ServeHTTP(wAgent, reqAgent)
	assert.Equal(t, http.StatusCreated, wAgent.Code)

	var agentResp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wAgent.Body.Bytes(), &agentResp))
	agentID := agentResp.Data.ID

	// 2. Request Ephemeral Test Token
	reqToken, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/workspaces/%s/agents/%s/test-token", wsID, agentID), nil)
	reqToken.Header.Set("Authorization", "Bearer "+token)
	wToken := httptest.NewRecorder()
	router.ServeHTTP(wToken, reqToken)
	assert.Equal(t, http.StatusOK, wToken.Code)

	var tokenResp struct {
		Data struct {
			Ticket             string  `json:"ticket"`
			WSSURL             string  `json:"wss_url"`
			RemainingCreditUSD float64 `json:"remaining_credit_usd"`
			ExpiresInSec       int     `json:"expires_in_sec"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wToken.Body.Bytes(), &tokenResp))
	assert.NotEmpty(t, tokenResp.Data.Ticket)
	assert.Equal(t, 60, tokenResp.Data.ExpiresInSec)
	assert.GreaterOrEqual(t, tokenResp.Data.RemainingCreditUSD, 0.0)

	// 3. Verify Atomic Wallet Balance Deduction
	var initialWallet domain.OrganizationWallet
	err := db.First(&initialWallet, "org_id = ?", orgID).Error
	require.NoError(t, err)

	deductionAmount := 0.25
	err = db.Exec("UPDATE organization_wallets SET balance_usd = GREATEST(0, balance_usd - ?), updated_at = NOW() WHERE org_id = ?",
		deductionAmount, orgID).Error
	require.NoError(t, err)

	var updatedWallet domain.OrganizationWallet
	err = db.First(&updatedWallet, "org_id = ?", orgID).Error
	require.NoError(t, err)
	assert.InDelta(t, initialWallet.BalanceUSD-deductionAmount, updatedWallet.BalanceUSD, 0.0001)
}
