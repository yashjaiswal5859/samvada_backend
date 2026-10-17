package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/samvada/abs/configs"
	"github.com/samvada/abs/internal/api/middleware"
	"github.com/samvada/abs/internal/application/admin"
)

type mockKetoCheckClient struct {
	allowed bool
}

func (m *mockKetoCheckClient) CheckPermission(ctx context.Context, namespace, object, relation, subjectID string) (bool, error) {
	return m.allowed, nil
}

func (m *mockKetoCheckClient) CreateRelationship(ctx context.Context, namespace, object, relation, subjectID string) error {
	return nil
}

func (m *mockKetoCheckClient) EnsureAdminRole(ctx context.Context, subjectID string) error {
	return nil
}

func setupAdminTestRouter(ketoAllowed bool, secret string) (*gin.Engine, admin.AdminService) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	cfg := &configs.Config{
		Auth: configs.AuthConfig{JWTSecret: secret},
		Admin: configs.AdminConfig{
			Username: "admin",
			Password: "TestPassword",
			UserID:   "00000000-0000-0000-0000-000000000001",
		},
	}
	ketoMock := &mockKetoCheckClient{allowed: ketoAllowed}
	logger := zap.NewNop()

	adminSvc := admin.NewAdminService(cfg, ketoMock, nil, logger)
	adminMW := middleware.NewAdminAuthMiddleware(adminSvc, ketoMock, secret, nil, logger)

	r.GET("/api/v1/admin/test", adminMW.RequireAdminKeto(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "authorized"})
	})

	return r, adminSvc
}

func TestAdminAuthMiddleware_MissingAccessToken(t *testing.T) {
	r, _ := setupAdminTestRouter(true, "secret123")

	req, _ := http.NewRequest("GET", "/api/v1/admin/test", nil)
	req.Header.Set("X-Refresh-Token", "some-refresh-token")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "MISSING_ACCESS_TOKEN")
}

func TestAdminAuthMiddleware_MissingRefreshToken(t *testing.T) {
	r, adminSvc := setupAdminTestRouter(true, "secret123")

	loginRes, err := adminSvc.Login(context.Background(), "admin", "TestPassword")
	require.NoError(t, err)

	req, _ := http.NewRequest("GET", "/api/v1/admin/test", nil)
	req.Header.Set("Authorization", "Bearer "+loginRes.AccessToken)
	// Notice: X-Refresh-Token header NOT set

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "MISSING_REFRESH_TOKEN")
}

func TestAdminAuthMiddleware_KetoReBAC_Forbidden(t *testing.T) {
	// Setup with ketoAllowed = false
	r, adminSvc := setupAdminTestRouter(false, "secret123")

	loginRes, err := adminSvc.Login(context.Background(), "admin", "TestPassword")
	require.NoError(t, err)

	req, _ := http.NewRequest("GET", "/api/v1/admin/test", nil)
	req.Header.Set("Authorization", "Bearer "+loginRes.AccessToken)
	req.Header.Set("X-Refresh-Token", loginRes.RefreshToken)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "FORBIDDEN_KETO_ADMIN_ROLE_REQUIRED")
}

func TestAdminAuthMiddleware_SuccessWithDualTokensAndKeto(t *testing.T) {
	// Setup with ketoAllowed = true
	r, adminSvc := setupAdminTestRouter(true, "secret123")

	loginRes, err := adminSvc.Login(context.Background(), "admin", "TestPassword")
	require.NoError(t, err)

	req, _ := http.NewRequest("GET", "/api/v1/admin/test", nil)
	req.Header.Set("Authorization", "Bearer "+loginRes.AccessToken)
	req.Header.Set("X-Refresh-Token", loginRes.RefreshToken)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "authorized")
}

func TestAdminAuthMiddleware_ExpiredToken(t *testing.T) {
	r, _ := setupAdminTestRouter(true, "secret123")

	// Generate expired token
	claims := jwt.MapClaims{
		"sub": "00000000-0000-0000-0000-000000000001",
		"exp": time.Now().Add(-1 * time.Hour).Unix(),
	}
	expiredTok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokStr, _ := expiredTok.SignedString([]byte("secret123"))

	req, _ := http.NewRequest("GET", "/api/v1/admin/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokStr)
	req.Header.Set("X-Refresh-Token", "valid-refresh-token")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "TOKEN_EXPIRED")
}
