package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type mockCache struct {
	counts map[string]int64
}

func newMockCache() *mockCache {
	return &mockCache{counts: make(map[string]int64)}
}

func (m *mockCache) Get(ctx context.Context, key string) (string, error) { return "", nil }
func (m *mockCache) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return nil
}
func (m *mockCache) Delete(ctx context.Context, key string) error { return nil }
func (m *mockCache) Exists(ctx context.Context, key string) (bool, error) { return false, nil }
func (m *mockCache) Increment(ctx context.Context, key string) (int64, error) {
	m.counts[key]++
	return m.counts[key], nil
}
func (m *mockCache) Expire(ctx context.Context, key string, expiration time.Duration) error {
	return nil
}
func (m *mockCache) AcquireLock(ctx context.Context, key string, value string, expiration time.Duration) (bool, error) {
	return true, nil
}
func (m *mockCache) ReleaseLock(ctx context.Context, key string, value string) (bool, error) {
	return true, nil
}

func TestSecurityHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(SecurityHeaders())
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", w.Code)
	}

	headers := map[string]string{
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains; preload",
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"X-XSS-Protection":          "1; mode=block",
	}

	for k, expected := range headers {
		if got := w.Header().Get(k); got != expected {
			t.Errorf("Header %s mismatch: expected %q, got %q", k, expected, got)
		}
	}
}

func TestCorrelationID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CorrelationID())
	r.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, c.GetString("correlation_id"))
	})

	// Test auto-generation
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w1, req1)
	if w1.Header().Get("X-Correlation-ID") == "" {
		t.Fatalf("Expected X-Correlation-ID header to be set")
	}

	// Test existing correlation ID preservation
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/test", nil)
	req2.Header.Set("X-Correlation-ID", "custom-corr-12345")
	r.ServeHTTP(w2, req2)
	if got := w2.Header().Get("X-Correlation-ID"); got != "custom-corr-12345" {
		t.Fatalf("Expected custom correlation ID, got %s", got)
	}
}

func TestRateLimiter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := newMockCache()

	r := gin.New()
	r.Use(RateLimiter(cache, 2, 1*time.Minute, "test_action"))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// Request 1: OK
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("Request 1 failed: %d", w1.Code)
	}

	// Request 2: OK
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("Request 2 failed: %d", w2.Code)
	}

	// Request 3: Exceeded limit (2 allowed, this is 3rd) -> 429
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusTooManyRequests {
		t.Fatalf("Expected 429 Too Many Requests, got %d", w3.Code)
	}
}
