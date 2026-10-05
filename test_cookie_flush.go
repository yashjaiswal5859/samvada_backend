package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func main() {
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	allPassed := true

	fmt.Println("=======================================================")
	fmt.Println("Testing Server-Side Cookie Flushing on Auth Failures")
	fmt.Println("=======================================================")

	// Test 1: Refresh token revoked / invalid on POST /api/v1/auth/refresh
	// Expects Set-Cookie headers with Max-Age=-1 for access_token and refresh_token
	refreshPayload, _ := json.Marshal(map[string]string{
		"refreshToken": "revoked_or_invalid_refresh_token_12345",
	})
	req1, _ := http.NewRequest("POST", "http://localhost:8081/api/v1/auth/refresh", bytes.NewReader(refreshPayload))
	req1.Header.Set("Content-Type", "application/json")
	resp1, err := client.Do(req1)
	if err != nil {
		fmt.Printf("[FAIL] Test 1: Connection error: %v\n", err)
		allPassed = false
	} else {
		resp1.Body.Close()
		cookies := resp1.Header.Values("Set-Cookie")
		hasClearAccessToken := false
		hasClearRefreshToken := false
		for _, c := range cookies {
			if strings.HasPrefix(c, "access_token=;") && strings.Contains(c, "Max-Age=0") || strings.Contains(c, "Max-Age=-1") || strings.Contains(c, "1970") {
				hasClearAccessToken = true
			}
			if strings.HasPrefix(c, "refresh_token=;") && strings.Contains(c, "Max-Age=0") || strings.Contains(c, "Max-Age=-1") || strings.Contains(c, "1970") {
				hasClearRefreshToken = true
			}
		}

		if hasClearAccessToken && hasClearRefreshToken {
			fmt.Printf("[PASS] Test 1: POST /api/v1/auth/refresh with invalid token flushed cookies successfully! Cookies: %v\n", cookies)
		} else {
			fmt.Printf("[FAIL] Test 1: Expected Set-Cookie clearing headers. Got: %v\n", cookies)
			allPassed = false
		}
	}

	// Test 2: GET /api/v1/auth/me with invalid or missing session
	// Expects Set-Cookie clearing cookies
	req2, _ := http.NewRequest("GET", "http://localhost:8081/api/v1/auth/me", nil)
	resp2, err := client.Do(req2)
	if err != nil {
		fmt.Printf("[FAIL] Test 2: Connection error: %v\n", err)
		allPassed = false
	} else {
		resp2.Body.Close()
		cookies := resp2.Header.Values("Set-Cookie")
		hasClearAccessToken := false
		for _, c := range cookies {
			if strings.HasPrefix(c, "access_token=;") {
				hasClearAccessToken = true
			}
		}
		if hasClearAccessToken {
			fmt.Printf("[PASS] Test 2: GET /api/v1/auth/me missing/invalid auth flushed cookies successfully! Cookies: %v\n", cookies)
		} else {
			fmt.Printf("[FAIL] Test 2: Expected Set-Cookie clearing headers on /me. Got: %v\n", cookies)
			allPassed = false
		}
	}

	// Test 3: GET /api/v1/auth/me with token for non-existent user in DB
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
		resp3.Body.Close()
		cookies := resp3.Header.Values("Set-Cookie")
		hasClearAccessToken := false
		for _, c := range cookies {
			if strings.HasPrefix(c, "access_token=;") {
				hasClearAccessToken = true
			}
		}
		if hasClearAccessToken {
			fmt.Printf("[PASS] Test 3: GET /api/v1/auth/me with deleted user flushed cookies successfully! Cookies: %v\n", cookies)
		} else {
			fmt.Printf("[FAIL] Test 3: Expected Set-Cookie clearing headers. Got: %v\n", cookies)
			allPassed = false
		}
	}

	fmt.Println("=======================================================")
	if allPassed {
		fmt.Println(">>> ALL SERVER-SIDE COOKIE FLUSHING TESTS PASSED! <<<")
		os.Exit(0)
	} else {
		fmt.Println(">>> CHECKS FAILED! <<<")
		os.Exit(1)
	}
}
