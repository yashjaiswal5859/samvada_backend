// =============================================================================
// Samvada Platform — Reporting Engine Correlation & OpenTelemetry Tracing
// =============================================================================

package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/reporting/internal/pkg/telemetry"
)

const (
	HeaderCorrelationID = "X-Correlation-ID"
	HeaderRequestID     = "X-Request-ID"
	HeaderTraceParent   = "traceparent"
	HeaderTraceID       = "X-Trace-ID"
)

func CorrelationAndLoggingMiddleware(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		corrID := c.GetHeader(HeaderCorrelationID)
		if corrID == "" {
			corrID = uuid.New().String()
		}
		c.Header(HeaderCorrelationID, corrID)

		reqID := c.GetHeader(HeaderRequestID)
		if reqID == "" {
			reqID = uuid.New().String()
		}
		c.Header(HeaderRequestID, reqID)

		// OpenTelemetry W3C TraceContext
		rawTraceparent := c.GetHeader(HeaderTraceParent)
		spanContext, ok := telemetry.ParseTraceparent(rawTraceparent)
		if !ok {
			spanContext = telemetry.SpanContext{
				TraceID: telemetry.GenerateTraceID(),
				SpanID:  telemetry.GenerateSpanID(),
				Sampled: true,
			}
		}

		formattedTraceparent := telemetry.FormatTraceparent(spanContext.TraceID, spanContext.SpanID, spanContext.Sampled)
		c.Header(HeaderTraceParent, formattedTraceparent)
		c.Header(HeaderTraceID, spanContext.TraceID)

		ctx := telemetry.WithSpanContext(c.Request.Context(), spanContext)
		c.Request = c.Request.WithContext(ctx)

		c.Set("correlation_id", corrID)
		c.Set("request_id", reqID)
		c.Set("trace_id", spanContext.TraceID)

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		logger.Info("HTTP Request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.String("client_ip", c.ClientIP()),
			zap.String("correlation_id", corrID),
			zap.String("trace_id", spanContext.TraceID),
			zap.String("span_id", spanContext.SpanID),
		)
	}
}
