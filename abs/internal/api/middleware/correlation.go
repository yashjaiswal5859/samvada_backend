// =============================================================================
// Samvada Platform — ABS Correlation & OpenTelemetry Distributed Tracing Middleware
// =============================================================================

package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/pkg/telemetry"
	"github.com/samvada/abs/pkg/logger"
)

const (
	HeaderCorrelationID = "X-Correlation-ID"
	HeaderRequestID     = "X-Request-ID"
	HeaderTraceParent   = "traceparent"
	HeaderTraceID       = "X-Trace-ID"
)

// CorrelationAndLoggingMiddleware assigns or propagates X-Correlation-ID and OpenTelemetry traceparent
func CorrelationAndLoggingMiddleware(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// 1. Correlation ID Resolution
		correlationID := c.GetHeader(HeaderCorrelationID)
		if correlationID == "" {
			correlationID = c.GetHeader(HeaderRequestID)
		}
		if correlationID == "" {
			correlationID = uuid.New().String()
		}

		c.Header(HeaderCorrelationID, correlationID)
		c.Header(HeaderRequestID, correlationID)

		// 2. OpenTelemetry W3C TraceContext Resolution
		rawTraceparent := c.GetHeader(HeaderTraceParent)
		spanContext, ok := telemetry.ParseTraceparent(rawTraceparent)
		if !ok {
			// Generate new trace ID and root span ID
			spanContext = telemetry.SpanContext{
				TraceID: telemetry.GenerateTraceID(),
				SpanID:  telemetry.GenerateSpanID(),
				Sampled: true,
			}
		}

		formattedTraceparent := telemetry.FormatTraceparent(spanContext.TraceID, spanContext.SpanID, spanContext.Sampled)
		c.Header(HeaderTraceParent, formattedTraceparent)
		c.Header(HeaderTraceID, spanContext.TraceID)

		// 3. Inject into Request Context
		ctx := logger.WithCorrelationID(c.Request.Context(), correlationID)
		ctx = telemetry.WithSpanContext(ctx, spanContext)
		c.Request = c.Request.WithContext(ctx)

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		log.Info("HTTP Request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.String("client_ip", c.ClientIP()),
			zap.String("correlation_id", correlationID),
			zap.String("trace_id", spanContext.TraceID),
			zap.String("span_id", spanContext.SpanID),
		)
	}
}
