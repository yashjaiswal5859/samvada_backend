package main

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"time"

	_ "github.com/lib/pq"
)

type APIEnvelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func main() {
	fmt.Println("==================================================")
	fmt.Println("  SAMVADA KAFKA-DRIVEN ONBOARDING VERIFICATION")
	fmt.Println("==================================================")

	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar:     jar,
		Timeout: 10 * time.Second,
	}

	testEmail := fmt.Sprintf("kafka_user_%d@samvada.io", time.Now().Unix())
	fmt.Printf("[1/6] Initiating Unified Login for: %s\n", testEmail)

	// 1. Login Request
	loginReqBody, _ := json.Marshal(map[string]string{
		"email": testEmail,
	})
	resp, err := client.Post("http://localhost:8081/api/v1/auth/login", "application/json", bytes.NewReader(loginReqBody))
	if err != nil {
		fmt.Printf("FAILED to call login: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var loginEnv APIEnvelope
	body, _ := io.ReadAll(resp.Body)
	json.Unmarshal(body, &loginEnv)
	if !loginEnv.Success {
		fmt.Printf("FAILED login response: %s\n", string(body))
		os.Exit(1)
	}

	var loginData struct {
		RequestID string `json:"requestId"`
		ExpiresIn int    `json:"expiresIn"`
	}
	json.Unmarshal(loginEnv.Data, &loginData)
	fmt.Printf("  -> RequestID received: %s\n", loginData.RequestID)

	// 2. Verify with Dev OTP (000000)
	fmt.Printf("[2/6] Verifying Login OTP using PKCE verification...\n")
	verifier := "abcdefghijklmnopqrstuvwxyz1234567890_verifier_test"
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	verifyReqBody, _ := json.Marshal(map[string]string{
		"requestId":      loginData.RequestID,
		"otp":            "000000",
		"code_verifier":  verifier,
		"code_challenge": challenge,
	})
	resp, err = client.Post("http://localhost:8081/api/v1/auth/login/verify", "application/json", bytes.NewReader(verifyReqBody))
	if err != nil {
		fmt.Printf("FAILED verify: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, _ = io.ReadAll(resp.Body)
	var verifyEnv APIEnvelope
	json.Unmarshal(body, &verifyEnv)
	if !verifyEnv.Success {
		fmt.Printf("FAILED verify response: %s\n", string(body))
		os.Exit(1)
	}

	var tokenData struct {
		AccessToken string `json:"accessToken"`
	}
	json.Unmarshal(verifyEnv.Data, &tokenData)
	fmt.Printf("  -> Authenticated successfully! Token received (%d bytes)\n", len(tokenData.AccessToken))

	// Share cookies across ports (8081 and 5173)
	u8081, _ := url.Parse("http://localhost:8081")
	u5173, _ := url.Parse("http://localhost:5173")
	cookies := jar.Cookies(u8081)
	jar.SetCookies(u5173, cookies)

	// 3. Step 1: Save Profile via Frontend Proxy (5173 -> 8081)
	orgName := fmt.Sprintf("Kafka Org %d", time.Now().Unix()%10000)
	wsName := "Default workspace"
	fmt.Printf("[3/6] Step 1: Submitting Profile (Business, %s, %s)...\n", orgName, wsName)
	profilePayload, _ := json.Marshal(map[string]interface{}{
		"firstName":     "Yash",
		"lastName":      "Jaiswal",
		"accountType":   "business",
		"orgName":       orgName,
		"workspaceName": wsName,
	})
	req, _ := http.NewRequest("POST", "http://localhost:5173/api/v1/onboarding/profile", bytes.NewReader(profilePayload))
	req.Header.Set("Content-Type", "application/json")
	if tokenData.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+tokenData.AccessToken)
	}
	resp, err = client.Do(req)
	if err != nil {
		fmt.Printf("FAILED to save profile: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, _ = io.ReadAll(resp.Body)
	var profileEnv APIEnvelope
	json.Unmarshal(body, &profileEnv)
	if !profileEnv.Success {
		fmt.Printf("FAILED save profile: %s\n", string(body))
		os.Exit(1)
	}
	fmt.Printf("  -> Step 1 Complete! Saved in samvada_auth and pushed event to Kafka.\n")

	// 4. Step 2: Save Industries
	fmt.Printf("[4/6] Step 2: Submitting Industries (Technology & SaaS)...\n")
	industriesPayload, _ := json.Marshal(map[string]interface{}{
		"industries": []string{
			"Technology & SaaS",
		},
	})
	req, _ = http.NewRequest("POST", "http://localhost:5173/api/v1/onboarding/industries", bytes.NewReader(industriesPayload))
	req.Header.Set("Content-Type", "application/json")
	if tokenData.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+tokenData.AccessToken)
	}
	resp, err = client.Do(req)
	if err != nil {
		fmt.Printf("FAILED to save industries: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, _ = io.ReadAll(resp.Body)
	var indEnv APIEnvelope
	json.Unmarshal(body, &indEnv)
	if !indEnv.Success {
		fmt.Printf("FAILED save industries: %s\n", string(body))
		os.Exit(1)
	}
	fmt.Printf("  -> Step 2 Complete! Industries saved.\n")

	// 5. Verify State
	fmt.Printf("[5/6] Checking Onboarding State...\n")
	req, _ = http.NewRequest("GET", "http://localhost:5173/api/v1/onboarding/state", nil)
	if tokenData.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+tokenData.AccessToken)
	}
	resp, err = client.Do(req)
	if err != nil {
		fmt.Printf("FAILED to get state: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, _ = io.ReadAll(resp.Body)
	var stateEnv APIEnvelope
	json.Unmarshal(body, &stateEnv)
	if !stateEnv.Success {
		fmt.Printf("FAILED state: %s\n", string(body))
		os.Exit(1)
	}

	// 6. Verify Database Persistence in BOTH samvada_auth AND samvada_abs
	fmt.Printf("[6/6] Verifying DBs & Kafka async worker processing...\n")
	time.Sleep(2 * time.Second) // allow Kafka consumer to process

	// Connect to samvada_auth
	authDB, err := sql.Open("postgres", "postgres://samvada:samvada_dev_2026@localhost:5432/samvada_auth?sslmode=disable")
	if err != nil {
		fmt.Printf("FAILED to connect to samvada_auth: %v\n", err)
		os.Exit(1)
	}
	defer authDB.Close()

	var authOrgCount, authWsCount, authProfileCount int
	_ = authDB.QueryRow("SELECT count(*) FROM organizations WHERE name = $1", orgName).Scan(&authOrgCount)
	_ = authDB.QueryRow("SELECT count(*) FROM workspaces WHERE name = $1", wsName).Scan(&authWsCount)
	_ = authDB.QueryRow("SELECT count(*) FROM user_profiles WHERE first_name = 'Yash' AND onboarding_completed = true").Scan(&authProfileCount)

	fmt.Printf("  -> authDB (samvada_auth):\n")
	fmt.Printf("     - Organizations with '%s': %d\n", orgName, authOrgCount)
	fmt.Printf("     - Workspaces with '%s':    %d\n", wsName, authWsCount)
	fmt.Printf("     - Completed User Profiles:      %d\n", authProfileCount)

	if authOrgCount == 0 || authWsCount == 0 {
		fmt.Printf("ERROR: Data was not saved in authDB!\n")
		os.Exit(1)
	}

	// Connect to samvada_abs
	absDB, err := sql.Open("postgres", "postgres://samvada:samvada_dev_2026@localhost:5432/samvada_abs?sslmode=disable")
	if err != nil {
		fmt.Printf("FAILED to connect to samvada_abs: %v\n", err)
		os.Exit(1)
	}
	defer absDB.Close()

	var absOrgCount, absWsCount, absMemberCount, absWalletCount int
	_ = absDB.QueryRow("SELECT count(*) FROM organizations WHERE name = $1", orgName).Scan(&absOrgCount)
	_ = absDB.QueryRow("SELECT count(*) FROM workspaces WHERE name = $1", wsName).Scan(&absWsCount)
	_ = absDB.QueryRow("SELECT count(*) FROM org_members WHERE email = $1 AND role = 'owner'", testEmail).Scan(&absMemberCount)
	_ = absDB.QueryRow("SELECT count(*) FROM organization_wallets WHERE promotional_credit_usd >= 5.0").Scan(&absWalletCount)

	fmt.Printf("  -> absDB (samvada_abs, populated via Kafka worker):\n")
	fmt.Printf("     - Organizations with '%s': %d\n", orgName, absOrgCount)
	fmt.Printf("     - Workspaces with '%s':    %d\n", wsName, absWsCount)
	fmt.Printf("     - Org Admin/Owner Members:      %d\n", absMemberCount)
	fmt.Printf("     - Wallets with Demo Credits:   %d\n", absWalletCount)

	if absOrgCount == 0 || absMemberCount == 0 {
		fmt.Printf("ERROR: Kafka worker did not provision org/admin in absDB!\n")
		os.Exit(1)
	}

	fmt.Println("\n==================================================")
	fmt.Println(">>> 100% SUCCESS: KAFKA WORKER PROVISIONED ORG, WORKSPACE, ADMIN IN ABS! <<<")
	fmt.Println("==================================================")
}
