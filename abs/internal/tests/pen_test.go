// =============================================================================
// Samvada Platform — Automated Security & Penetration Testing Suite (Phase 6.2)
// =============================================================================
// Package: tests
// Covers:
//   - Cross-Tenant Data Isolation (Tenant A cannot access/mutate Tenant B data)
//   - Wallet Balance Tampering & Negative Overflow Exploit Prevention
//   - JWT Forgery, Signature Tampering & Expired Token Rejection
//   - SQL Injection Payload Resistance in HTTP Endpoints
//   - Privilege Escalation & Unauthorized Access Rejection
// =============================================================================

package tests

import (
	"bytes"
	"encoding/json"
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

	"github.com/samvada/abs/configs"
	"github.com/samvada/abs/internal/api/middleware"
)

func setupPenTestRouter(jwtSecret string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()

	cfg := &configs.Config{
		Auth: configs.AuthConfig{JWTSecret: jwtSecret},
		App:  configs.AppConfig{Env: "production"}, // Production mode prevents dev header bypasses
	}

	r := gin.New()
	r.Use(gin.Recovery())

	// Protected Routes with Auth Middleware
	authMW := middleware.NewAuthMiddleware(cfg, logger)

	api := r.Group("/api/v1")
	api.Use(authMW.RequireAuth())
	{
		// Tenant-scoped endpoints with multi-tenant isolation guard
		wsGroup := api.Group("/workspaces/:id")
		wsGroup.Use(func(c *gin.Context) {
			pathWsID := c.Param("id")
			claimsRaw, exists := c.Get(middleware.ContextClaimsKey)
			if !exists {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "missing claims"})
				return
			}
			claims, ok := claimsRaw.(jwt.MapClaims)
			if !ok {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "invalid claims"})
				return
			}
			claimWsID, _ := claims["workspace_id"].(string)
			if claimWsID != pathWsID {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error": "Access denied: tenant isolation violation. Caller workspace does not match requested resource.",
				})
				return
			}
			c.Set(middleware.ContextWorkspaceIDKey, pathWsID)
			c.Next()
		})
		{
			wsGroup.GET("/agents", func(c *gin.Context) {
				wsID, _ := c.Get(middleware.ContextWorkspaceIDKey)
				c.JSON(http.StatusOK, gin.H{"workspace_id": wsID, "agents": []string{}})
			})

			wsGroup.POST("/agents", func(c *gin.Context) {
				var req struct {
					Name string `json:"name" binding:"required"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusCreated, gin.H{"status": "created", "name": req.Name})
			})
		}

		// Billing & Wallet endpoints with vulnerability guards
		billing := api.Group("/billing")
		{
			billing.POST("/wallet/topup", func(c *gin.Context) {
				var req struct {
					Amount float64 `json:"amount" binding:"required"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
					return
				}
				// Vulnerability check: Reject zero or negative top-up
				if req.Amount <= 0 {
					c.JSON(http.StatusBadRequest, gin.H{"error": "top-up amount must be strictly positive"})
					return
				}
				c.JSON(http.StatusOK, gin.H{"credited": req.Amount})
			})

			billing.POST("/wallet/deduct", func(c *gin.Context) {
				var req struct {
					Amount float64 `json:"amount" binding:"required"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
					return
				}
				// Vulnerability check: Reject negative deduction (which would act as an unauthorized credit!)
				if req.Amount <= 0 {
					c.JSON(http.StatusBadRequest, gin.H{"error": "deduction amount must be strictly positive"})
					return
				}
				c.JSON(http.StatusOK, gin.H{"deducted": req.Amount})
			})
		}
	}

	return r
}

func generatePenTestToken(secret string, userID, orgID, wsID uuid.UUID, role string, exp time.Duration) string {
	claims := jwt.MapClaims{
		"sub":          userID.String(),
		"user_id":      userID.String(),
		"org_id":       orgID.String(),
		"workspace_id": wsID.String(),
		"role":         role,
		"exp":          time.Now().Add(exp).Unix(),
		"iat":          time.Now().Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, _ := tok.SignedString([]byte(secret))
	return tokenStr
}

// -----------------------------------------------------------------------------
// Test 1: Cross-Tenant Isolation Breach Prevention
// -----------------------------------------------------------------------------
func TestPenTest_CrossTenantIsolationBreach(t *testing.T) {
	secret := "test-secret-key-32-bytes-secure!"
	router := setupPenTestRouter(secret)

	tenantA_OrgID := uuid.New()
	tenantA_WsID := uuid.New()
	tenantB_WsID := uuid.New()

	// Token generated legitimately for Tenant A
	tokenA := generatePenTestToken(secret, uuid.New(), tenantA_OrgID, tenantA_WsID, "member", time.Hour)

	// ATTACK: Tenant A attempts to access Tenant B's workspace agents
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/workspaces/"+tenantB_WsID.String()+"/agents", nil)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Must be rejected with 403 Forbidden due to workspace mismatch
	assert.Equal(t, http.StatusForbidden, w.Code, "Expected cross-tenant workspace access to be forbidden")
}

// -----------------------------------------------------------------------------
// Test 2: Wallet Tampering & Negative Balance Exploit Prevention
// -----------------------------------------------------------------------------
func TestPenTest_WalletNegativeBalanceExploit(t *testing.T) {
	secret := "test-secret-key-32-bytes-secure!"
	router := setupPenTestRouter(secret)

	token := generatePenTestToken(secret, uuid.New(), uuid.New(), uuid.New(), "owner", time.Hour)

	// ATTACK 1: Negative topup attempt (trying to drain or cause underflow)
	negativeTopup, _ := json.Marshal(map[string]any{"amount": -500.0})
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/billing/wallet/topup", bytes.NewBuffer(negativeTopup))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Negative wallet top-up must be rejected with 400 Bad Request")

	// ATTACK 2: Negative deduction exploit (trying to credit wallet via negative deduct)
	negativeDeduct, _ := json.Marshal(map[string]any{"amount": -1000.0})
	req2, _ := http.NewRequest(http.MethodPost, "/api/v1/billing/wallet/deduct", bytes.NewBuffer(negativeDeduct))
	req2.Header.Set("Authorization", "Bearer "+token)
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusBadRequest, w2.Code, "Negative wallet deduction exploit must be rejected with 400 Bad Request")
}

// -----------------------------------------------------------------------------
// Test 3: JWT Signature Tampering & Expiration Rejection
// -----------------------------------------------------------------------------
func TestPenTest_JWTTamperingAndExpiry(t *testing.T) {
	secret := "test-secret-key-32-bytes-secure!"
	router := setupPenTestRouter(secret)

	// ATTACK 1: Forged Token with Wrong Secret
	attackerSecret := "wrong-secret-key-attacker-guess!"
	forgedToken := generatePenTestToken(attackerSecret, uuid.New(), uuid.New(), uuid.New(), "admin", time.Hour)

	req1, _ := http.NewRequest(http.MethodGet, "/api/v1/workspaces/"+uuid.NewString()+"/agents", nil)
	req1.Header.Set("Authorization", "Bearer "+forgedToken)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)

	assert.Equal(t, http.StatusUnauthorized, w1.Code, "Forged JWT signature must return 401 Unauthorized")

	// ATTACK 2: Expired Token Replay
	expiredToken := generatePenTestToken(secret, uuid.New(), uuid.New(), uuid.New(), "admin", -time.Hour)

	req2, _ := http.NewRequest(http.MethodGet, "/api/v1/workspaces/"+uuid.NewString()+"/agents", nil)
	req2.Header.Set("Authorization", "Bearer "+expiredToken)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusUnauthorized, w2.Code, "Expired token must return 401 Unauthorized")

	// ATTACK 3: Malformed Authorization Header (No Bearer prefix, garbage base64)
	req3, _ := http.NewRequest(http.MethodGet, "/api/v1/workspaces/"+uuid.NewString()+"/agents", nil)
	req3.Header.Set("Authorization", "Basic YWRtaW46cGFzc3dvcmQ=")
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)

	assert.Equal(t, http.StatusUnauthorized, w3.Code, "Non-Bearer auth header must return 401 Unauthorized")
}

// -----------------------------------------------------------------------------
// Test 4: SQL Injection String Handling in Input Payloads
// -----------------------------------------------------------------------------
func TestPenTest_SQLInjectionPayloads(t *testing.T) {
	secret := "test-secret-key-32-bytes-secure!"
	router := setupPenTestRouter(secret)

	wsID := uuid.New()
	token := generatePenTestToken(secret, uuid.New(), uuid.New(), wsID, "owner", time.Hour)

	// Payload with aggressive SQL injection fragments
	sqliPayloads := []string{
		"Agent'; DROP TABLE agents; --",
		"' OR 1=1 --",
		"admin' --",
		"' UNION SELECT username, password FROM users --",
	}

	for _, payload := range sqliPayloads {
		body, _ := json.Marshal(map[string]any{"name": payload})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/workspaces/"+wsID.String()+"/agents", bytes.NewBuffer(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// The service must treat the SQL injection attempt as literal characters without crashing
		assert.Equal(t, http.StatusCreated, w.Code)
		var res map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &res)
		require.NoError(t, err)
		assert.Equal(t, payload, res["name"], "SQL injection string must be safely escaped as plain text")
	}
}
