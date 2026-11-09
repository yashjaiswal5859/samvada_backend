package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ridesafety/auth-service/internal/platform/identity"
)

func TestHealthEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHealthHandler(nil, nil)

	r := gin.New()
	r.GET("/health", h.Health)
	r.GET("/live", h.Live)

	// Test /health
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("GET", "/health", nil)
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", w1.Code)
	}

	// Test /live
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/live", nil)
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", w2.Code)
	}
}

func TestLoginValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewLoginHandler(nil, nil, nil, nil, nil, false)

	r := gin.New()
	r.POST("/login", h.Login)

	// Missing email body -> 422 Unprocessable Entity
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/login", bytes.NewBuffer([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("Expected 422 for missing email, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestVerifyLoginMissingCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewLoginHandler(nil, nil, nil, nil, nil, false)

	r := gin.New()
	r.POST("/login/verify", h.VerifyLogin)

	body, _ := json.Marshal(map[string]string{
		"requestId": "d54407b1-21c6-43ad-8d96-03fdf5388049",
		"otp":       "123456",
	})

	// Request without session_id cookie -> 401 Unauthorized
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/login/verify", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 for missing session cookie, got %d (body: %s)", w.Code, w.Body.String())
	}
}



func TestMeEndpointMissingAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewMeHandler(nil, nil, nil)

	r := gin.New()
	r.GET("/me", h.Me)

	// No cookie and no Authorization header -> 401
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/me", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 for unauthenticated /me, got %d (body: %s)", w.Code, w.Body.String())
	}
}

type mockTokenProvider struct {
	active bool
	sub    string
}

func (m *mockTokenProvider) GenerateTokens(ctx context.Context, kratosID string, customClaims map[string]interface{}) (*identity.TokenPair, error) {
	return nil, nil
}
func (m *mockTokenProvider) RotateTokens(ctx context.Context, refreshToken string) (*identity.TokenPair, error) {
	return nil, nil
}
func (m *mockTokenProvider) RevokeTokens(ctx context.Context, refreshToken string) error {
	return nil
}
func (m *mockTokenProvider) AcceptLoginChallenge(ctx context.Context, challenge string, subject string) (string, error) {
	return "", nil
}
func (m *mockTokenProvider) AcceptConsentChallenge(ctx context.Context, challenge string, subject string, scopes []string, customClaims map[string]interface{}) (string, error) {
	return "", nil
}
func (m *mockTokenProvider) GetConsentRequest(ctx context.Context, challenge string) (*identity.ConsentRequest, error) {
	return nil, nil
}
func (m *mockTokenProvider) ExchangeCodeForTokens(ctx context.Context, code string, codeVerifier string) (*identity.TokenPair, error) {
	return nil, nil
}
func (m *mockTokenProvider) IntrospectToken(ctx context.Context, token string) (bool, string, int64, error) {
	return m.active, m.sub, 3600, nil
}
func (m *mockTokenProvider) ValidateIDToken(ctx context.Context, rawJWT string, expectedNonce string, expectedAudience string) (*identity.IDTokenClaims, error) {
	return nil, nil
}
func (m *mockTokenProvider) GetLoginRequest(ctx context.Context, challenge string) (bool, error) {
	return false, nil
}

func TestSecurityHandler_Unauthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mock := &mockTokenProvider{active: false}
	h := NewSecurityHandler(mock)

	r := gin.New()
	r.POST("/password", h.UpdatePassword)
	r.GET("/sessions", h.ListSessions)

	// 1. Missing token -> 401
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("GET", "/sessions", nil)
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 for missing token, got %d", w1.Code)
	}

	// 2. Invalid token -> 401
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/password", bytes.NewBufferString(`{"current_password":"old","new_password":"newpassword123"}`))
	req2.Header.Set("Authorization", "Bearer invalid-token")
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 for invalid token, got %d", w2.Code)
	}
}

