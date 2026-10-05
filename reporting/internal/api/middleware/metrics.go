// =============================================================================
// Samvada Platform — Reporting Engine Prometheus Metrics Middleware (Phase 6.1)
// =============================================================================

package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

type metricsCollector struct {
	mu           sync.RWMutex
	requestCount map[string]*uint64
	inFlight     int64
	durationSum  map[string]*uint64 // in microseconds
}

var (
	reCollector = &metricsCollector{
		requestCount: make(map[string]*uint64),
		durationSum:  make(map[string]*uint64),
	}
)

func PrometheusMetrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/metrics" || c.Request.URL.Path == "/health" {
			c.Next()
			return
		}

		atomic.AddInt64(&reCollector.inFlight, 1)
		start := time.Now()

		c.Next()

		atomic.AddInt64(&reCollector.inFlight, -1)
		duration := time.Since(start)

		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}
		status := fmt.Sprintf("%d", c.Writer.Status())
		method := c.Request.Method
		key := fmt.Sprintf("method=\"%s\",path=\"%s\",status=\"%s\"", method, path, status)

		reCollector.mu.Lock()
		counter, exists := reCollector.requestCount[key]
		if !exists {
			var val uint64
			counter = &val
			reCollector.requestCount[key] = counter
		}
		durCounter, durExists := reCollector.durationSum[key]
		if !durExists {
			var durVal uint64
			durCounter = &durVal
			reCollector.durationSum[key] = durCounter
		}
		reCollector.mu.Unlock()

		atomic.AddUint64(counter, 1)
		atomic.AddUint64(durCounter, uint64(duration.Microseconds()))
	}
}

func MetricsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var sb strings.Builder

		sb.WriteString("# HELP http_requests_in_flight Current in-flight HTTP requests\n")
		sb.WriteString("# TYPE http_requests_in_flight gauge\n")
		sb.WriteString(fmt.Sprintf("http_requests_in_flight %d\n\n", atomic.LoadInt64(&reCollector.inFlight)))

		sb.WriteString("# HELP http_requests_total Total number of HTTP requests processed\n")
		sb.WriteString("# TYPE http_requests_total counter\n")

		reCollector.mu.RLock()
		for labels, ptr := range reCollector.requestCount {
			sb.WriteString(fmt.Sprintf("http_requests_total{%s} %d\n", labels, atomic.LoadUint64(ptr)))
		}

		sb.WriteString("\n# HELP http_request_duration_seconds_sum Total duration of HTTP requests in seconds\n")
		sb.WriteString("# TYPE http_request_duration_seconds_sum counter\n")
		for labels, ptr := range reCollector.durationSum {
			sec := float64(atomic.LoadUint64(ptr)) / 1_000_000.0
			sb.WriteString(fmt.Sprintf("http_request_duration_seconds_sum{%s} %.6f\n", labels, sec))
		}
		reCollector.mu.RUnlock()

		c.Data(http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", []byte(sb.String()))
	}
}
