package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	jwtSecret = "samvada_jwt_super_secret_dev_2026_key"
	absURL    = "http://localhost:8082"
)

func main() {
	// 1. Pick an existing user ID from samvada_abs (from our earlier db query):
	// user_id: 8f12a34a-7469-466e-961f-fcd499d59425
	userID := "8f12a34a-7469-466e-961f-fcd499d59425"
	userEmail := "yash.jaiswal@samvada.ai"

	claims := jwt.MapClaims{
		"sub":   userID,
		"email": userEmail,
		"exp":   time.Now().Add(24 * time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		panic(err)
	}

	fmt.Println("Minted test JWT:", tokenString[:20]+"...")

	// 2. Call ABS /api/v1/user/me directly
	req, err := http.NewRequest(http.MethodGet, absURL+"/api/v1/user/me", nil)
	if err != nil {
		panic(err)
	}
	req.Header.Set("Authorization", "Bearer "+tokenString)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	fmt.Printf("ABS Response Status: %d\n", resp.StatusCode)

	var pretty map[string]interface{}
	if err := json.Unmarshal(body, &pretty); err == nil {
		formatted, _ := json.MarshalIndent(pretty, "", "  ")
		fmt.Println("ABS Response Body:\n", string(formatted))
	} else {
		fmt.Println("Raw body:", string(body))
	}
}
