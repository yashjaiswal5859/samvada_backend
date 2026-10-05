package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/IBM/sarama"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
)

type TestReport struct {
	Total   int
	Passed  int
	Failed  int
	Results []TestResult
}

type TestResult struct {
	Category string
	Name     string
	Status   string // PASS / FAIL
	Duration time.Duration
	Details  string
}

func (r *TestReport) Record(category, name string, err error, d time.Duration, details string) {
	r.Total++
	res := TestResult{
		Category: category,
		Name:     name,
		Duration: d,
		Details:  details,
	}
	if err == nil {
		r.Passed++
		res.Status = "PASS"
		fmt.Printf("  [PASS] %-15s %-40s (%v)\n", category, name, d)
	} else {
		r.Failed++
		res.Status = "FAIL"
		fmt.Printf("  [FAIL] %-15s %-40s (%v) -> ERROR: %v | %s\n", category, name, d, err, details)
	}
	r.Results = append(r.Results, res)
}

func hashOTP(otp string) string {
	h := sha256.Sum256([]byte(otp))
	return hex.EncodeToString(h[:])
}

func checkStatus(report *TestReport, category, name string, resp *http.Response, err error, d time.Duration, okCodes ...int) ([]byte, error) {
	if err != nil {
		report.Record(category, name, err, d, "")
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	for _, code := range okCodes {
		if resp.StatusCode == code {
			report.Record(category, name, nil, d, fmt.Sprintf("HTTP %d OK", resp.StatusCode))
			return body, nil
		}
	}
	failErr := fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	report.Record(category, name, failErr, d, "")
	return body, failErr
}

func main() {
	report := &TestReport{}

	fmt.Println("======================================================================")
	fmt.Println("    SAMVADA PLATFORM — EXHAUSTIVE AGGRESSIVE E2E TEST SUITE")
	fmt.Println("======================================================================")
	fmt.Println("Target Services:")
	fmt.Println("  - Auth Service      : http://localhost:8081")
	fmt.Println("  - ABS Server        : http://localhost:8082")
	fmt.Println("  - Reporting Server  : http://localhost:8083")
	fmt.Println("  - Frontend Proxy    : http://localhost:5173")
	fmt.Println("  - Kafka Brokers     : localhost:9092")
	fmt.Println("  - Redis             : localhost:6379")
	fmt.Println("  - Postgres          : localhost:5432 (samvada_auth, samvada_abs)")
	fmt.Println("  - Mailpit Relay     : localhost:8025 / localhost:1025")
	fmt.Println("======================================================================")

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	// -------------------------------------------------------------------------
	// 1. CORE INFRASTRUCTURE TESTS
	// -------------------------------------------------------------------------
	fmt.Println("\n--- 1. Testing Core Infrastructure ---")

	// Postgres samvada_auth
	start := time.Now()
	authDB, err := sql.Open("pgx", "postgres://samvada:samvada_dev_2026@localhost:5432/samvada_auth?sslmode=disable")
	if err == nil {
		err = authDB.PingContext(context.Background())
	}
	report.Record("INFRA", "PostgreSQL (samvada_auth)", err, time.Since(start), "Port 5432")

	// Postgres samvada_abs
	start = time.Now()
	absDB, err := sql.Open("pgx", "postgres://samvada:samvada_dev_2026@localhost:5432/samvada_abs?sslmode=disable")
	if err == nil {
		err = absDB.PingContext(context.Background())
	}
	report.Record("INFRA", "PostgreSQL (samvada_abs)", err, time.Since(start), "Port 5432")

	// Redis
	start = time.Now()
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379", Password: ""})
	err = rdb.Ping(context.Background()).Err()
	report.Record("INFRA", "Redis Cache & Sessions", err, time.Since(start), "Port 6379")

	// Kafka
	start = time.Now()
	kConfig := sarama.NewConfig()
	kConfig.Version = sarama.V2_8_0_0
	admin, err := sarama.NewClusterAdmin([]string{"localhost:9092"}, kConfig)
	var topics map[string]sarama.TopicDetail
	if err == nil {
		topics, err = admin.ListTopics()
		_ = admin.Close()
	}
	report.Record("INFRA", "Kafka Broker & Topics", err, time.Since(start), fmt.Sprintf("%d topics discovered", len(topics)))

	// Mailpit
	start = time.Now()
	resp, err := client.Get("http://localhost:8025/api/v1/messages")
	if err == nil && resp.StatusCode == 200 {
		_ = resp.Body.Close()
		report.Record("INFRA", "Mailpit Dev SMTP Server", nil, time.Since(start), "Port 8025 API OK")
	} else {
		report.Record("INFRA", "Mailpit Dev SMTP Server", err, time.Since(start), "Mailpit reachable")
	}

	// -------------------------------------------------------------------------
	// 2. AUTH SERVICE APIS
	// -------------------------------------------------------------------------
	fmt.Println("\n--- 2. Testing Auth Service APIs ---")

	// Health
	start = time.Now()
	resp, err = client.Get("http://localhost:8081/health")
	checkStatus(report, "AUTH", "GET /health", resp, err, time.Since(start), 200)

	// Admin Register
	testEmail := fmt.Sprintf("agg_%d@samvada.ai", time.Now().Unix())
	regBody := fmt.Sprintf(`{"email":"%s","displayName":"Aggressive Tester","phoneNumber":"+1234567890"}`, testEmail)
	start = time.Now()
	resp, err = client.Post("http://localhost:8081/api/v1/auth/admin/register", "application/json", strings.NewReader(regBody))
	var reqID string
	var regCookie string
	if err == nil {
		for _, c := range resp.Cookies() {
			if c.Name == "registration_session_id" {
				regCookie = c.Value
			}
		}
	}
	body, _ := checkStatus(report, "AUTH", "POST /api/v1/auth/admin/register", resp, err, time.Since(start), 202)
	if body != nil {
		var r struct {
			Data struct {
				RequestID string `json:"requestId"`
			} `json:"data"`
		}
		_ = json.Unmarshal(body, &r)
		reqID = r.Data.RequestID
	}

	// Update OTP hash in pending_auth
	latestOTP := "999999"
	otpHash := hashOTP(latestOTP)
	if authDB != nil && reqID != "" {
		_, _ = authDB.Exec("UPDATE pending_auth SET otp_hash = $1 WHERE id = $2", otpHash, reqID)
	}

	// Verify Admin OTP
	var accessToken string
	var refreshToken string
	start = time.Now()
	verifyBody := fmt.Sprintf(`{"requestId":"%s","otp":"%s","loginChallenge":""}`, reqID, latestOTP)
	httpReq, _ := http.NewRequest("POST", "http://localhost:8081/api/v1/auth/admin/verify", strings.NewReader(verifyBody))
	httpReq.Header.Set("Content-Type", "application/json")
	if regCookie != "" {
		httpReq.AddCookie(&http.Cookie{Name: "registration_session_id", Value: regCookie})
	}
	resp, err = client.Do(httpReq)
	body, _ = checkStatus(report, "AUTH", "POST /api/v1/auth/admin/verify", resp, err, time.Since(start), 200)
	if body != nil {
		var r struct {
			Data struct {
				AccessToken  string `json:"accessToken"`
				RefreshToken string `json:"refreshToken"`
			} `json:"data"`
		}
		_ = json.Unmarshal(body, &r)
		accessToken = r.Data.AccessToken
		refreshToken = r.Data.RefreshToken
	}

	// GET /api/v1/auth/me
	start = time.Now()
	httpReq, _ = http.NewRequest("GET", "http://localhost:8081/api/v1/auth/me", nil)
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err = client.Do(httpReq)
	checkStatus(report, "AUTH", "GET /api/v1/auth/me", resp, err, time.Since(start), 200)

	// POST /api/v1/auth/login
	var loginReqID string
	var loginSessionCookie string
	start = time.Now()
	loginBody := fmt.Sprintf(`{"email":"%s"}`, testEmail)
	resp, err = client.Post("http://localhost:8081/api/v1/auth/login", "application/json", strings.NewReader(loginBody))
	if err == nil {
		for _, c := range resp.Cookies() {
			if c.Name == "session_id" || c.Name == "login_session_id" {
				loginSessionCookie = c.Value
			}
		}
	}
	body, _ = checkStatus(report, "AUTH", "POST /api/v1/auth/login", resp, err, time.Since(start), 202)
	if body != nil {
		var r struct {
			Data struct {
				RequestID string `json:"requestId"`
			} `json:"data"`
		}
		_ = json.Unmarshal(body, &r)
		loginReqID = r.Data.RequestID
	}

	// Verify Login OTP
	start = time.Now()
	if loginReqID != "" {
		_, _ = authDB.Exec("UPDATE pending_auth SET otp_hash = $1 WHERE id = $2", otpHash, loginReqID)
		verifyLoginBody := fmt.Sprintf(`{"requestId":"%s","otp":"999999"}`, loginReqID)
		httpReq, _ = http.NewRequest("POST", "http://localhost:8081/api/v1/auth/login/verify", strings.NewReader(verifyLoginBody))
		httpReq.Header.Set("Content-Type", "application/json")
		if loginSessionCookie != "" {
			httpReq.AddCookie(&http.Cookie{Name: "session_id", Value: loginSessionCookie})
			httpReq.AddCookie(&http.Cookie{Name: "login_session_id", Value: loginSessionCookie})
		}
		resp, err = client.Do(httpReq)
		checkStatus(report, "AUTH", "POST /api/v1/auth/login/verify", resp, err, time.Since(start), 200)
	}

	// Refresh Token
	start = time.Now()
	refBody := fmt.Sprintf(`{"refreshToken":"%s"}`, refreshToken)
	resp, err = client.Post("http://localhost:8081/api/v1/auth/refresh", "application/json", strings.NewReader(refBody))
	checkStatus(report, "AUTH", "POST /api/v1/auth/refresh", resp, err, time.Since(start), 200)

	// Onboarding State
	start = time.Now()
	httpReq, _ = http.NewRequest("GET", "http://localhost:8081/api/v1/onboarding/state", nil)
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err = client.Do(httpReq)
	checkStatus(report, "AUTH", "GET /api/v1/onboarding/state", resp, err, time.Since(start), 200)

	// -------------------------------------------------------------------------
	// 3. ABS SERVICE APIS (Tested with live JWT issued by Auth Service!)
	// -------------------------------------------------------------------------
	fmt.Println("\n--- 3. Testing ABS Service APIs (with live Auth JWT) ---")

	// Complete Onboarding to create Org & Default Workspace with demo credits
	var orgID string
	var workspaceID string
	start = time.Now()
	onboardPayload := fmt.Sprintf(`{
		"company_name": "Aggressive Corp %d",
		"slug": "agg-corp-%d",
		"billing_email": "%s"
	}`, time.Now().Unix()%10000, time.Now().Unix()%10000, testEmail)
	httpReq, _ = http.NewRequest("POST", "http://localhost:8082/api/v1/onboarding", strings.NewReader(onboardPayload))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err = client.Do(httpReq)
	body, _ = checkStatus(report, "ABS", "POST /api/v1/onboarding", resp, err, time.Since(start), 200, 201)
	if body != nil {
		var r struct {
			Data struct {
				OrganizationID string `json:"organization_id"`
				WorkspaceID    string `json:"workspace_id"`
			} `json:"data"`
		}
		_ = json.Unmarshal(body, &r)
		orgID = r.Data.OrganizationID
		workspaceID = r.Data.WorkspaceID
	}

	// Create Secondary Workspace
	start = time.Now()
	wsName := fmt.Sprintf("Aggressive_WS_%d", time.Now().Unix()%10000)
	createWSBody := fmt.Sprintf(`{"name":"%s"}`, wsName)
	httpReq, _ = http.NewRequest("POST", "http://localhost:8082/api/v1/workspaces", strings.NewReader(createWSBody))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	httpReq.Header.Set("X-Org-ID", orgID)
	resp, err = client.Do(httpReq)
	checkStatus(report, "ABS", "POST /api/v1/workspaces", resp, err, time.Since(start), 200, 201)

	// List Workspaces
	start = time.Now()
	httpReq, _ = http.NewRequest("GET", "http://localhost:8082/api/v1/workspaces", nil)
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err = client.Do(httpReq)
	checkStatus(report, "ABS", "GET /api/v1/workspaces", resp, err, time.Since(start), 200)

	// Helper function to create authenticated + tenant-scoped request
	makeABSReq := func(method, path string, reqBody string) *http.Request {
		var r io.Reader
		if reqBody != "" {
			r = strings.NewReader(reqBody)
		}
		req, _ := http.NewRequest(method, "http://localhost:8082"+path, r)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("X-Workspace-ID", workspaceID)
		req.Header.Set("X-Org-ID", orgID)
		if reqBody != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		return req
	}

	// Billing Wallet
	start = time.Now()
	resp, err = client.Do(makeABSReq("GET", "/api/v1/billing/wallet", ""))
	checkStatus(report, "ABS", "GET /api/v1/billing/wallet", resp, err, time.Since(start), 200)

	// Billing Transactions
	start = time.Now()
	resp, err = client.Do(makeABSReq("GET", "/api/v1/billing/wallet/transactions", ""))
	checkStatus(report, "ABS", "GET /api/v1/billing/wallet/transactions", resp, err, time.Since(start), 200)

	// Billing Plans
	start = time.Now()
	resp, err = client.Do(makeABSReq("GET", "/api/v1/billing/plans", ""))
	checkStatus(report, "ABS", "GET /api/v1/billing/plans", resp, err, time.Since(start), 200)

	// Create Agent
	var agentID string
	start = time.Now()
	agentPayload := `{
		"name": "Aggressive Test Assistant",
		"role": "Customer Support",
		"system_prompt": "You are a professional support agent.",
		"voice_id": "alloy",
		"voice_provider": "fish_audio",
		"voice_name": "Alloy Voice",
		"language": "en-US",
		"model": "gpt-4o"
	}`
	resp, err = client.Do(makeABSReq("POST", "/api/v1/agents", agentPayload))
	body, _ = checkStatus(report, "ABS", "POST /api/v1/agents", resp, err, time.Since(start), 200, 201)
	if body != nil {
		var r struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		_ = json.Unmarshal(body, &r)
		agentID = r.Data.ID
	}

	// List Agents
	start = time.Now()
	resp, err = client.Do(makeABSReq("GET", "/api/v1/agents", ""))
	checkStatus(report, "ABS", "GET /api/v1/agents", resp, err, time.Since(start), 200)

	// Get Agent by ID
	if agentID != "" {
		start = time.Now()
		resp, err = client.Do(makeABSReq("GET", "/api/v1/agents/"+agentID, ""))
		checkStatus(report, "ABS", "GET /api/v1/agents/:id", resp, err, time.Since(start), 200)

		// Agent Versions
		start = time.Now()
		resp, err = client.Do(makeABSReq("GET", "/api/v1/agents/"+agentID+"/versions", ""))
		checkStatus(report, "ABS", "GET /api/v1/agents/:id/versions", resp, err, time.Since(start), 200)
	}

	// Create Audience
	var audienceID string
	start = time.Now()
	resp, err = client.Do(makeABSReq("POST", "/api/v1/audiences", `{"name":"VIP Customers Aggressive","description":"High priority audience"}`))
	body, _ = checkStatus(report, "ABS", "POST /api/v1/audiences", resp, err, time.Since(start), 200, 201)
	if body != nil {
		var r struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		_ = json.Unmarshal(body, &r)
		audienceID = r.Data.ID
	}

	// List Audiences
	start = time.Now()
	resp, err = client.Do(makeABSReq("GET", "/api/v1/audiences", ""))
	checkStatus(report, "ABS", "GET /api/v1/audiences", resp, err, time.Since(start), 200)

	// Create Campaign
	if agentID != "" && audienceID != "" {
		start = time.Now()
		campPayload := fmt.Sprintf(`{
			"name": "Aggressive Outbound Test",
			"agent_id": "%s",
			"audience_id": "%s",
			"type": "outbound",
			"concurrency_limit": 5
		}`, agentID, audienceID)
		resp, err = client.Do(makeABSReq("POST", "/api/v1/campaigns", campPayload))
		checkStatus(report, "ABS", "POST /api/v1/campaigns", resp, err, time.Since(start), 200, 201)
	}

	// List Campaigns
	start = time.Now()
	resp, err = client.Do(makeABSReq("GET", "/api/v1/campaigns", ""))
	checkStatus(report, "ABS", "GET /api/v1/campaigns", resp, err, time.Since(start), 200)

	// Telephony Phone Numbers
	start = time.Now()
	resp, err = client.Do(makeABSReq("GET", "/api/v1/phone-numbers", ""))
	checkStatus(report, "ABS", "GET /api/v1/phone-numbers", resp, err, time.Since(start), 200)

	// Knowledge Bases
	start = time.Now()
	resp, err = client.Do(makeABSReq("GET", "/api/v1/kb", ""))
	checkStatus(report, "ABS", "GET /api/v1/kb", resp, err, time.Since(start), 200)

	// Developer API Keys
	start = time.Now()
	resp, err = client.Do(makeABSReq("POST", "/api/v1/api-keys", `{"name":"E2E Test Key","expires_in_days":30}`))
	checkStatus(report, "ABS", "POST /api/v1/api-keys", resp, err, time.Since(start), 200, 201)

	start = time.Now()
	resp, err = client.Do(makeABSReq("GET", "/api/v1/api-keys", ""))
	checkStatus(report, "ABS", "GET /api/v1/api-keys", resp, err, time.Since(start), 200)

	// Webhooks
	start = time.Now()
	resp, err = client.Do(makeABSReq("POST", "/api/v1/webhooks", `{"url":"https://example.com/webhook","events":["call.completed"]}`))
	checkStatus(report, "ABS", "POST /api/v1/webhooks", resp, err, time.Since(start), 200, 201)

	start = time.Now()
	resp, err = client.Do(makeABSReq("GET", "/api/v1/webhooks", ""))
	checkStatus(report, "ABS", "GET /api/v1/webhooks", resp, err, time.Since(start), 200)

	// -------------------------------------------------------------------------
	// 4. REPORTING ENGINE APIS
	// -------------------------------------------------------------------------
	fmt.Println("\n--- 4. Testing Reporting Engine APIs ---")

	makeRepReq := func(method, path string) *http.Request {
		req, _ := http.NewRequest(method, "http://localhost:8083"+path, nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("X-Org-ID", orgID)
		req.Header.Set("X-Workspace-ID", workspaceID)
		return req
	}

	// Summary
	start = time.Now()
	resp, err = client.Do(makeRepReq("GET", fmt.Sprintf("/api/v1/analytics/summary?workspace_id=%s&org_id=%s", workspaceID, orgID)))
	checkStatus(report, "REPORTING", "GET /api/v1/analytics/summary", resp, err, time.Since(start), 200)

	// Volume Trend
	start = time.Now()
	resp, err = client.Do(makeRepReq("GET", fmt.Sprintf("/api/v1/analytics/volume-trend?workspace_id=%s&org_id=%s", workspaceID, orgID)))
	checkStatus(report, "REPORTING", "GET /api/v1/analytics/volume-trend", resp, err, time.Since(start), 200)

	// Leaderboard
	start = time.Now()
	resp, err = client.Do(makeRepReq("GET", fmt.Sprintf("/api/v1/analytics/agents/leaderboard?workspace_id=%s&org_id=%s", workspaceID, orgID)))
	checkStatus(report, "REPORTING", "GET /api/v1/analytics/agents/leaderboard", resp, err, time.Since(start), 200)

	// Funnels
	start = time.Now()
	resp, err = client.Do(makeRepReq("GET", fmt.Sprintf("/api/v1/analytics/funnels?workspace_id=%s&org_id=%s", workspaceID, orgID)))
	checkStatus(report, "REPORTING", "GET /api/v1/analytics/funnels", resp, err, time.Since(start), 200)

	// Live Calls
	start = time.Now()
	resp, err = client.Do(makeRepReq("GET", fmt.Sprintf("/api/v1/analytics/live-calls?workspace_id=%s&org_id=%s", workspaceID, orgID)))
	checkStatus(report, "REPORTING", "GET /api/v1/analytics/live-calls", resp, err, time.Since(start), 200)

	// Call Records
	start = time.Now()
	resp, err = client.Do(makeRepReq("GET", fmt.Sprintf("/api/v1/calls?workspace_id=%s&org_id=%s", workspaceID, orgID)))
	checkStatus(report, "REPORTING", "GET /api/v1/calls", resp, err, time.Since(start), 200)

	// -------------------------------------------------------------------------
	// 5. ASYNC KAFKA WORKER CONSUMERS
	// -------------------------------------------------------------------------
	fmt.Println("\n--- 5. Testing Async Kafka Worker Pipelines ---")

	prodConfig := sarama.NewConfig()
	prodConfig.Producer.Return.Successes = true
	prodConfig.Producer.Return.Errors = true
	prodConfig.Version = sarama.V2_8_0_0

	producer, err := sarama.NewSyncProducer([]string{"localhost:9092"}, prodConfig)
	if err == nil {
		// Test s2s.call.completed event -> consumed by abs-worker billing consumer
		start = time.Now()
		callUUID := uuid.New().String()
		callCompletedPayload := fmt.Sprintf(`{
			"call_id": "%s",
			"workspace_id": "%s",
			"agent_id": "%s",
			"duration_seconds": 65,
			"billable_seconds": 70,
			"ended_reason": "completed",
			"cost_breakdown": {
				"llm_cost": 0.005,
				"tts_cost": 0.003,
				"stt_cost": 0.002,
				"telephony_cost": 0.004,
				"total_cost": 0.014
			}
		}`, callUUID, workspaceID, agentID)

		msg := &sarama.ProducerMessage{
			Topic: "s2s.call.completed",
			Key:   sarama.StringEncoder(callUUID),
			Value: sarama.StringEncoder(callCompletedPayload),
		}
		_, _, err = producer.SendMessage(msg)
		report.Record("WORKER", "Produce s2s.call.completed event", err, time.Since(start), fmt.Sprintf("Call: %s", callUUID))

		// Test abs.campaign.dispatch -> consumed by abs-worker campaign consumer
		start = time.Now()
		campUUID := uuid.New().String()
		campEventPayload := fmt.Sprintf(`{
			"campaign_id": "%s",
			"workspace_id": "%s",
			"agent_id": "%s",
			"contact_batch": [
				{"contact_id": "%s", "phone_number": "+18005550199"}
			]
		}`, campUUID, workspaceID, agentID, uuid.New().String())

		msg = &sarama.ProducerMessage{
			Topic: "abs.campaign.dispatch",
			Key:   sarama.StringEncoder(campUUID),
			Value: sarama.StringEncoder(campEventPayload),
		}
		_, _, err = producer.SendMessage(msg)
		report.Record("WORKER", "Produce abs.campaign.dispatch event", err, time.Since(start), fmt.Sprintf("Campaign: %s", campUUID))

		// Test abs.webhook.event -> consumed by abs-worker webhook consumer
		start = time.Now()
		whUUID := uuid.New().String()
		whEventPayload := fmt.Sprintf(`{
			"event_id": "%s",
			"workspace_id": "%s",
			"event_type": "call.completed",
			"payload": {"call_id": "%s"}
		}`, whUUID, workspaceID, callUUID)

		msg = &sarama.ProducerMessage{
			Topic: "abs.webhook.event",
			Key:   sarama.StringEncoder(whUUID),
			Value: sarama.StringEncoder(whEventPayload),
		}
		_, _, err = producer.SendMessage(msg)
		report.Record("WORKER", "Produce abs.webhook.event event", err, time.Since(start), fmt.Sprintf("Webhook: %s", whUUID))

		_ = producer.Close()
	} else {
		report.Record("WORKER", "Kafka SyncProducer init", err, 0, "")
	}

	// -------------------------------------------------------------------------
	// 6. FRONTEND VITE PROXY TESTS
	// -------------------------------------------------------------------------
	fmt.Println("\n--- 6. Testing Frontend Reverse Proxy (Port 5173) ---")

	makeProxyReq := func(method, path string) *http.Request {
		req, _ := http.NewRequest(method, "http://localhost:5173"+path, nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("X-Workspace-ID", workspaceID)
		req.Header.Set("X-Org-ID", orgID)
		return req
	}

	// Proxy -> Auth /me
	start = time.Now()
	resp, err = client.Do(makeProxyReq("GET", "/api/v1/auth/me"))
	checkStatus(report, "PROXY", "5173 -> 8081 (/api/v1/auth/me)", resp, err, time.Since(start), 200)

	// Proxy -> ABS /workspaces
	start = time.Now()
	resp, err = client.Do(makeProxyReq("GET", "/api/v1/workspaces"))
	checkStatus(report, "PROXY", "5173 -> 8082 (/api/v1/workspaces)", resp, err, time.Since(start), 200)

	// Proxy -> ABS /agents
	start = time.Now()
	resp, err = client.Do(makeProxyReq("GET", "/api/v1/agents"))
	checkStatus(report, "PROXY", "5173 -> 8082 (/api/v1/agents)", resp, err, time.Since(start), 200)

	// Proxy -> Reporting /analytics/summary
	start = time.Now()
	resp, err = client.Do(makeProxyReq("GET", fmt.Sprintf("/api/v1/analytics/summary?workspace_id=%s&org_id=%s", workspaceID, orgID)))
	checkStatus(report, "PROXY", "5173 -> 8083 (/api/v1/analytics/summary)", resp, err, time.Since(start), 200)

	// -------------------------------------------------------------------------
	// SUMMARY
	// -------------------------------------------------------------------------
	fmt.Println("\n======================================================================")
	fmt.Printf("AGGRESSIVE TEST RUN COMPLETED: %d Tests Run | %d Passed | %d Failed\n", report.Total, report.Passed, report.Failed)
	fmt.Println("======================================================================")

	if report.Failed > 0 {
		fmt.Printf("\nFAILED TESTS (%d):\n", report.Failed)
		for _, res := range report.Results {
			if res.Status == "FAIL" {
				fmt.Printf("  - [%s] %s: %s\n", res.Category, res.Name, res.Details)
			}
		}
		os.Exit(1)
	} else {
		fmt.Println("\n>>> ALL TESTS PASSED SUCCESSFULLY! ZERO FAILURES. <<<")
	}
}
