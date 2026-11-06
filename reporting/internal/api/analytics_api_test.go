// =============================================================================
// Samvada Platform — Reporting Engine REST API Integration Test
// =============================================================================

package api_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
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

	"github.com/samvada/reporting/configs"
	"github.com/samvada/reporting/internal/api/dto"
	"github.com/samvada/reporting/internal/api/handlers"
	"github.com/samvada/reporting/internal/api/middleware"
	"github.com/samvada/reporting/internal/api/routes"
	"github.com/samvada/reporting/internal/application/analytics"
	"github.com/samvada/reporting/internal/application/calls"
	"github.com/samvada/reporting/internal/domain"
	"github.com/samvada/reporting/internal/infrastructure/clickhouse"
	"github.com/samvada/reporting/internal/infrastructure/clickhouse/repos"
	"github.com/samvada/reporting/internal/infrastructure/redis"
)

var (
	testRSAPrivateKey *rsa.PrivateKey
	testRSAPublicKey  *rsa.PublicKey
)

func init() {
	testRSAPrivateKey, _ = rsa.GenerateKey(rand.Reader, 2048)
	testRSAPublicKey = &testRSAPrivateKey.PublicKey
}

func generateTestJWT(userID uuid.UUID, email, role, orgID string, exp time.Duration) (string, error) {
	claims := jwt.MapClaims{
		"sub":    userID.String(),
		"email":  email,
		"role":   role,
		"org_id": orgID,
		"iat":    time.Now().Unix(),
		"exp":    time.Now().Add(exp).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "test-reporting-key"
	return token.SignedString(testRSAPrivateKey)
}

func setupAPIRouter(t *testing.T) (*gin.Engine, domain.CallRepository) {
	cfg := &configs.Config{
		App: configs.AppConfig{
			Env: "test",
		},
		ClickHouse: configs.ClickHouseConfig{
			Addr:           "localhost:9000",
			Database:       "samvada_analytics",
			User:           "samvada",
			Password:       "samvada_ch_2026",
			DialTimeoutSec: 5,
			MaxOpenConns:   10,
			MaxIdleConns:   5,
		},
		Redis: configs.RedisConfig{
			Addr: "localhost:6379",
			DB:   2,
		},
	}

	logger := zap.NewNop()
	chConn, err := clickhouse.NewConnection(&cfg.ClickHouse, logger)
	if err != nil {
		t.Skipf("Skipping test: ClickHouse is not reachable on localhost:9000: %v", err)
		return nil, nil
	}

	rdb, _ := redis.NewClient(&cfg.Redis, logger)

	callRepo := repos.NewCallRepository(chConn, logger)
	analyticsRepo := repos.NewAnalyticsRepository(chConn, logger)

	analyticsService := analytics.NewAnalyticsService(analyticsRepo, rdb, logger)
	callsService := calls.NewCallsService(callRepo, logger)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.Recovery())

	authMW := middleware.NewAuthMiddleware(cfg, logger)
	authMW.SetTestPublicKey("test-reporting-key", testRSAPublicKey)
	analyticsHandler := handlers.NewAnalyticsHandler(analyticsService)
	callsHandler := handlers.NewCallsHandler(callsService)

	routes.RegisterAPIRoutes(router, routes.RouterDependencies{
		AuthMW:           authMW,
		AnalyticsHandler: analyticsHandler,
		CallsHandler:     callsHandler,
	})

	return router, callRepo
}

func TestReportingAPI_HttpOnlyCookieAuthAndAnalytics(t *testing.T) {
	router, callRepo := setupAPIRouter(t)

	userID := uuid.New()
	orgID := uuid.New()
	wsID := uuid.New()
	agentID := uuid.New()
	callID := uuid.New()

	// Seed one call event in ClickHouse
	now := time.Now().UTC()
	start := now.Add(-10 * time.Minute)
	end := now.Add(-5 * time.Minute)
	err := callRepo.BatchInsertCallEvents(context.Background(), []*domain.CallEvent{
		{
			CallID:            callID,
			OrgID:             orgID,
			WorkspaceID:       wsID,
			AgentID:           agentID,
			Direction:         "outbound",
			CallerNumber:      "+18005550100",
			CalledNumber:      "+14155550299",
			CustomerName:      "Michael Scott",
			StartedAt:         start,
			EndedAt:           &end,
			DurationSec:       300,
			BillableSec:       300,
			Disposition:       "completed",
			EvaluationScore:   88,
			TotalCostUSD:      0.18,
			OverallSentiment:  "positive",
			RecordingURL:      "https://storage.samvada.ai/rec/1.wav",
			TranscriptSummary: "Product demo discussed.",
			CreatedAt:         now,
		},
	})
	require.NoError(t, err)

	// 1. Unauthenticated request should fail with 401
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/analytics/summary", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// 2. Request with Expired Token in Cookie should return 401 with TOKEN_EXPIRED
	expiredToken, _ := generateTestJWT(userID, "test@samvada.ai", "admin", orgID.String(), -10*time.Minute)
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/analytics/summary", nil)
	req.AddCookie(&http.Cookie{
		Name:     "access_token",
		Value:    expiredToken,
		Path:     "/",
		HttpOnly: true,
	})
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "TOKEN_EXPIRED")

	// 3. Request with Valid HttpOnly Cookie token
	validToken, err := generateTestJWT(userID, "test@samvada.ai", "admin", orgID.String(), 1*time.Hour)
	require.NoError(t, err)

	req, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/analytics/summary?workspace_id=%s", wsID), nil)
	req.AddCookie(&http.Cookie{
		Name:     "access_token",
		Value:    validToken,
		Path:     "/",
		HttpOnly: true,
	})
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp dto.APIResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.True(t, resp.Success)

	// 4. Test GET /api/v1/calls
	req, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/calls?workspace_id=%s", wsID), nil)
	req.AddCookie(&http.Cookie{
		Name:     "access_token",
		Value:    validToken,
		Path:     "/",
		HttpOnly: true,
	})
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 5. Test GET /api/v1/calls/:call_id
	req, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/calls/%s", callID), nil)
	req.AddCookie(&http.Cookie{
		Name:     "access_token",
		Value:    validToken,
		Path:     "/",
		HttpOnly: true,
	})
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}
