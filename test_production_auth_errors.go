package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func main() {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	allPassed := true

	fmt.Println("=======================================================")
	fmt.Println("Testing Production-Ready Auth Error Handling & Status Codes")
	fmt.Println("=======================================================")

	// Test 1: POST /api/v1/auth/login with non-existent email
	// MUST return 400 Bad Request (NOT 404 Not Found)
	loginPayload, _ := json.Marshal(map[string]string{
		"email": fmt.Sprintf("nonexistent-%s@example.com", uuid.New().String()[:8]),
	})
	resp1, err := client.Post("http://localhost:8081/api/v1/auth/login", "application/json", bytes.NewReader(loginPayload))
	if err != nil {
		fmt.Printf("[FAIL] Test 1: Connection error: %v\n", err)
		allPassed = false
	} else {
		body, _ := io.ReadAll(resp1.Body)
		resp1.Body.Close()
		if resp1.StatusCode == http.StatusBadRequest {
			fmt.Printf("[PASS] Test 1: Non-existent email login returned HTTP 400 Bad Request (not 404). Body: %s\n", string(body))
		} else {
			fmt.Printf("[FAIL] Test 1: Expected HTTP 400, got HTTP %d. Body: %s\n", resp1.StatusCode, string(body))
			allPassed = false
		}
	}

	// Test 2: POST /api/v1/auth/login/verify without session cookie
	// MUST return 401 Unauthorized (session required)
	verifyPayload, _ := json.Marshal(map[string]string{
		"requestId": uuid.New().String(),
		"otp":       "123456",
	})
	resp2, err := client.Post("http://localhost:8081/api/v1/auth/login/verify", "application/json", bytes.NewReader(verifyPayload))
	if err != nil {
		fmt.Printf("[FAIL] Test 2: Connection error: %v\n", err)
		allPassed = false
	} else {
		body, _ := io.ReadAll(resp2.Body)
		resp2.Body.Close()
		if resp2.StatusCode == http.StatusUnauthorized {
			fmt.Printf("[PASS] Test 2: Missing session cookie on login/verify returned HTTP 401 Unauthorized (not 404). Body: %s\n", string(body))
		} else {
			fmt.Printf("[FAIL] Test 2: Expected HTTP 401, got HTTP %d. Body: %s\n", resp2.StatusCode, string(body))
			allPassed = false
		}
	}

	// Test 3: GET /api/v1/auth/me with signed JWT for a user not found in local DB
	// MUST return 401 Unauthorized (not 404 Not Found)
	claims := jwt.MapClaims{
		"sub":     uuid.New().String(),
		"email":   "ghost-user@example.com",
		"iss":     "samvada-auth",
		"exp":     time.Now().Add(1 * time.Hour).Unix(),
		"user_id": uuid.New().String(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, _ := token.SignedString([]byte("samvada-super-secret-jwt-key-2026-production-grade"))

	req3, _ := http.NewRequest("GET", "http://localhost:8081/api/v1/auth/me", nil)
	req3.Header.Set("Authorization", "Bearer "+tokenStr)
	resp3, err := client.Do(req3)
	if err != nil {
		fmt.Printf("[FAIL] Test 3: Connection error: %v\n", err)
		allPassed = false
	} else {
		body, _ := io.ReadAll(resp3.Body)
		resp3.Body.Close()
		if resp3.StatusCode == http.StatusUnauthorized {
			fmt.Printf("[PASS] Test 3: Unknown user session on /me returned HTTP 401 Unauthorized (not 404). Body: %s\n", string(body))
		} else {
			fmt.Printf("[FAIL] Test 3: Expected HTTP 401, got HTTP %d. Body: %s\n", resp3.StatusCode, string(body))
			allPassed = false
		}
	}

	fmt.Println("=======================================================")
	if allPassed {
		fmt.Println(">>> ALL PRODUCTION ERROR HANDLING CHECKS PASSED! <<<")
		os.Exit(0)
	} else {
		fmt.Println(">>> CHECKS FAILED! <<<")
		os.Exit(1)
	}
}
