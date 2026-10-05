// =============================================================================
// Samvada Platform — ABS Request Body Limit Middleware Unit Tests (Phase 6.2)
// =============================================================================

package middleware_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/samvada/abs/internal/api/middleware"
)

func TestRequestBodyLimit_AllowsWithinLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestBodyLimit(1024)) // 1KB limit

	r.POST("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	body := bytes.NewBuffer(make([]byte, 500))
	req, _ := http.NewRequest("POST", "/test", body)
	req.Header.Set("Content-Length", "500")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequestBodyLimit_RejectsExcessContentLength(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestBodyLimit(1024)) // 1KB limit

	r.POST("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	body := bytes.NewBuffer(make([]byte, 2048))
	req, _ := http.NewRequest("POST", "/test", body)
	req.Header.Set("Content-Length", "2048")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	assert.Contains(t, w.Body.String(), "PAYLOAD_TOO_LARGE")
}
