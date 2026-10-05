// =============================================================================
// Samvada Platform — ABS Prometheus Metrics Middleware (Phase 6.1 & 6.3)
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
	"github.com/samvada/abs/internal/infrastructure/redis"
)

type metricsCollector struct {
	mu           sync.RWMutex
	requestCount map[string]*uint64
	inFlight     int64
	durationSum  map[string]*uint64 // in microseconds
}

var (
	defaultCollector = &metricsCollector{
		requestCount: make(map[string]*uint64),
		durationSum:  make(map[string]*uint64),
	}
)

// PrometheusMetrics collects request metrics and exposes /metrics
func PrometheusMetrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/metrics" || c.Request.URL.Path == "/health" {
			c.Next()
			return
		}

		atomic.AddInt64(&defaultCollector.inFlight, 1)
		start := time.Now()

		c.Next()

		atomic.AddInt64(&defaultCollector.inFlight, -1)
		duration := time.Since(start)

		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}
		status := fmt.Sprintf("%d", c.Writer.Status())
		method := c.Request.Method
		key := fmt.Sprintf("method=\"%s\",path=\"%s\",status=\"%s\"", method, path, status)

		defaultCollector.mu.Lock()
		counter, exists := defaultCollector.requestCount[key]
		if !exists {
			var val uint64
			counter = &val
			defaultCollector.requestCount[key] = counter
		}
		durCounter, durExists := defaultCollector.durationSum[key]
		if !durExists {
			var durVal uint64
			durCounter = &durVal
			defaultCollector.durationSum[key] = durCounter
		}
		defaultCollector.mu.Unlock()

		atomic.AddUint64(counter, 1)
		atomic.AddUint64(durCounter, uint64(duration.Microseconds()))
	}
}

// MetricsHandler serves Prometheus scrape endpoint at /metrics
func MetricsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var sb strings.Builder

		sb.WriteString("# HELP http_requests_in_flight Current in-flight HTTP requests\n")
		sb.WriteString("# TYPE http_requests_in_flight gauge\n")
		sb.WriteString(fmt.Sprintf("http_requests_in_flight %d\n\n", atomic.LoadInt64(&defaultCollector.inFlight)))

		sb.WriteString("# HELP http_requests_total Total number of HTTP requests processed\n")
		sb.WriteString("# TYPE http_requests_total counter\n")

		defaultCollector.mu.RLock()
		for labels, ptr := range defaultCollector.requestCount {
			sb.WriteString(fmt.Sprintf("http_requests_total{%s} %d\n", labels, atomic.LoadUint64(ptr)))
		}

		sb.WriteString("\n# HELP http_request_duration_seconds_sum Total duration of HTTP requests in seconds\n")
		sb.WriteString("# TYPE http_request_duration_seconds_sum counter\n")
		for labels, ptr := range defaultCollector.durationSum {
			sec := float64(atomic.LoadUint64(ptr)) / 1_000_000.0
			sb.WriteString(fmt.Sprintf("http_request_duration_seconds_sum{%s} %.6f\n", labels, sec))
		}
		defaultCollector.mu.RUnlock()

		// Redis Cache Hit / Miss Metrics & Hit Ratio (Phase 6.3 - Line 514)
		hits, misses, ratio := redis.GetCacheMetrics()
		sb.WriteString("\n# HELP redis_cache_hits_total Total Redis cache hit operations\n")
		sb.WriteString("# TYPE redis_cache_hits_total counter\n")
		sb.WriteString(fmt.Sprintf("redis_cache_hits_total %d\n", hits))

		sb.WriteString("\n# HELP redis_cache_misses_total Total Redis cache miss operations\n")
		sb.WriteString("# TYPE redis_cache_misses_total counter\n")
		sb.WriteString(fmt.Sprintf("redis_cache_misses_total %d\n", misses))

		sb.WriteString("\n# HELP redis_cache_hit_ratio Current Redis cache hit ratio (0.0 to 1.0)\n")
		sb.WriteString("# TYPE redis_cache_hit_ratio gauge\n")
		sb.WriteString(fmt.Sprintf("redis_cache_hit_ratio %.4f\n", ratio))

		c.Data(http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", []byte(sb.String()))
	}
}
