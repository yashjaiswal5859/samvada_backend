package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

type mockKetoWorkspaceClient struct {
	checkFunc func(ctx context.Context, namespace, object, relation, subjectID string) (bool, error)
}

func (m *mockKetoWorkspaceClient) CheckPermission(ctx context.Context, namespace, object, relation, subjectID string) (bool, error) {
	if m.checkFunc != nil {
		return m.checkFunc(ctx, namespace, object, relation, subjectID)
	}
	return false, nil
}

func (m *mockKetoWorkspaceClient) CreateRelationship(ctx context.Context, namespace, object, relation, subjectID string) error {
	return nil
}

func (m *mockKetoWorkspaceClient) EnsureAdminRole(ctx context.Context, subjectID string) error {
	return nil
}

func TestReBACMiddleware_KetoAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockKeto := &mockKetoWorkspaceClient{
		checkFunc: func(ctx context.Context, namespace, object, relation, subjectID string) (bool, error) {
			assert.Equal(t, NamespaceWorkspaces, namespace)
			assert.Equal(t, "ws-123", object)
			assert.Equal(t, "usr-456", subjectID)
			if relation == RoleOperator {
				return true, nil
			}
			return false, nil
		},
	}

	rebac := NewReBACMiddleware(mockKeto, nil, nil, "production", zap.NewNop())

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(ContextWorkspaceIDKey, "ws-123")
		c.Set(ContextUserIDKey, "usr-456")
		c.Next()
	})
	r.POST("/test", rebac.RequireWorkspaceReBAC(RoleOperator), func(c *gin.Context) {
		role, _ := c.Get(ContextWorkspaceRoleKey)
		c.JSON(http.StatusOK, gin.H{"status": "ok", "role": role})
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"role":"operator"`)
}

func TestReBACMiddleware_KetoForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockKeto := &mockKetoWorkspaceClient{
		checkFunc: func(ctx context.Context, namespace, object, relation, subjectID string) (bool, error) {
			// Disallowed for all candidate relations
			return false, nil
		},
	}

	rebac := NewReBACMiddleware(mockKeto, nil, nil, "production", zap.NewNop())

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(ContextWorkspaceIDKey, "ws-123")
		c.Set(ContextUserIDKey, "usr-456")
		c.Next()
	})
	r.POST("/test", rebac.RequireWorkspaceReBAC(RoleOperator), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "FORBIDDEN_KETO_REBAC_DENIED")
}

func TestReBACMiddleware_KetoUnavailable_FailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Keto is unreachable (network timeout / connection refused)
	mockKeto := &mockKetoWorkspaceClient{
		checkFunc: func(ctx context.Context, namespace, object, relation, subjectID string) (bool, error) {
			return false, errors.New("connection refused: keto service down")
		},
	}

	rebac := NewReBACMiddleware(mockKeto, nil, nil, "production", zap.NewNop())

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(ContextWorkspaceIDKey, "ws-123")
		c.Set(ContextUserIDKey, "usr-456")
		c.Next()
	})
	r.POST("/test", rebac.RequireWorkspaceReBAC(RoleOperator), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Must fail closed with 503 Service Unavailable when Keto is down (zero fallback)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "KETO_SERVICE_UNAVAILABLE")
}

func TestReBACMiddleware_MissingContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockKeto := &mockKetoWorkspaceClient{}
	rebac := NewReBACMiddleware(mockKeto, nil, nil, "production", zap.NewNop())

	r := gin.New()
	r.POST("/test", rebac.RequireWorkspaceReBAC(RoleOperator), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Missing workspace context
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Provide workspace but missing user auth
	r2 := gin.New()
	r2.Use(func(c *gin.Context) {
		c.Set(ContextWorkspaceIDKey, "ws-123")
		c.Next()
	})
	r2.POST("/test", rebac.RequireWorkspaceReBAC(RoleOperator), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	req2 := httptest.NewRequest(http.MethodPost, "/test", nil)
	w2 := httptest.NewRecorder()
	r2.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusUnauthorized, w2.Code)
}
