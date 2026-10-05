// =============================================================================
// Samvada Platform — OpenTelemetry Distributed Tracing & W3C Trace Propagation
// =============================================================================

package telemetry

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	HeaderTraceParent = "traceparent"
	HeaderTraceState  = "tracestate"
	HeaderTraceID     = "X-Trace-ID"
	HeaderSpanID      = "X-Span-ID"
)

type traceContextKey struct{}

// SpanContext holds distributed tracing context following W3C TraceContext specs.
type SpanContext struct {
	TraceID string
	SpanID  string
	Sampled bool
}

// Span represents an active OpenTelemetry trace span.
type Span struct {
	Name      string
	Context   SpanContext
	StartTime time.Time
	logger    *zap.Logger
}

// GenerateTraceID generates a 128-bit (32 hex char) W3C trace ID.
func GenerateTraceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "00000000000000000000000000000001"
	}
	return hex.EncodeToString(b)
}

// GenerateSpanID generates a 64-bit (16 hex char) W3C span ID.
func GenerateSpanID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "0000000000000001"
	}
	return hex.EncodeToString(b)
}

// FormatTraceparent formats trace information into W3C traceparent string:
// 00-{trace_id}-{span_id}-{trace_flags}
func FormatTraceparent(traceID, spanID string, sampled bool) string {
	flags := "01"
	if !sampled {
		flags = "00"
	}
	return fmt.Sprintf("00-%s-%s-%s", traceID, spanID, flags)
}

// ParseTraceparent parses a W3C traceparent header.
func ParseTraceparent(tp string) (SpanContext, bool) {
	tp = strings.TrimSpace(tp)
	parts := strings.Split(tp, "-")
	if len(parts) != 4 || parts[0] != "00" {
		return SpanContext{}, false
	}
	if len(parts[1]) != 32 || len(parts[2]) != 16 {
		return SpanContext{}, false
	}
	return SpanContext{
		TraceID: parts[1],
		SpanID:  parts[2],
		Sampled: parts[3] == "01",
	}, true
}

// WithSpanContext stores the SpanContext in the Go context.
func WithSpanContext(ctx context.Context, sc SpanContext) context.Context {
	return context.WithValue(ctx, traceContextKey{}, sc)
}

// FromContext extracts the active SpanContext from context if present.
func FromContext(ctx context.Context) (SpanContext, bool) {
	if ctx == nil {
		return SpanContext{}, false
	}
	sc, ok := ctx.Value(traceContextKey{}).(SpanContext)
	return sc, ok
}

// StartSpan creates and begins a new child span.
func StartSpan(ctx context.Context, name string, logger *zap.Logger) (context.Context, *Span) {
	parentSC, found := FromContext(ctx)
	traceID := parentSC.TraceID
	if !found || traceID == "" {
		traceID = GenerateTraceID()
	}
	spanID := GenerateSpanID()

	childSC := SpanContext{
		TraceID: traceID,
		SpanID:  spanID,
		Sampled: true,
	}

	span := &Span{
		Name:      name,
		Context:   childSC,
		StartTime: time.Now(),
		logger:    logger,
	}

	return WithSpanContext(ctx, childSC), span
}

// End finishes the span and logs span duration if sampled.
func (s *Span) End() {
	duration := time.Since(s.StartTime)
	if s.logger != nil {
		s.logger.Debug("Span completed",
			zap.String("span_name", s.Name),
			zap.String("trace_id", s.Context.TraceID),
			zap.String("span_id", s.Context.SpanID),
			zap.Duration("duration_ms", duration),
		)
	}
}
