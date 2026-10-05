package telemetry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestOpenTelemetry_FormatAndParseTraceparent(t *testing.T) {
	traceID := GenerateTraceID()
	spanID := GenerateSpanID()
	require.Len(t, traceID, 32)
	require.Len(t, spanID, 16)

	tp := FormatTraceparent(traceID, spanID, true)
	sc, ok := ParseTraceparent(tp)
	require.True(t, ok)
	assert.Equal(t, traceID, sc.TraceID)
	assert.Equal(t, spanID, sc.SpanID)
	assert.True(t, sc.Sampled)

	// Malformed traceparents
	_, ok = ParseTraceparent("invalid-traceparent")
	assert.False(t, ok)

	_, ok = ParseTraceparent("01-tooshort-short-01")
	assert.False(t, ok)
}

func TestOpenTelemetry_ContextSpanPropagation(t *testing.T) {
	logger := zap.NewNop()
	ctx := context.Background()

	ctxWithSpan, rootSpan := StartSpan(ctx, "root-operation", logger)
	defer rootSpan.End()

	rootSC, ok := FromContext(ctxWithSpan)
	require.True(t, ok)
	assert.NotEmpty(t, rootSC.TraceID)
	assert.NotEmpty(t, rootSC.SpanID)

	// Child span should inherit root TraceID but get distinct SpanID
	childCtx, childSpan := StartSpan(ctxWithSpan, "child-operation", logger)
	defer childSpan.End()

	childSC, ok := FromContext(childCtx)
	require.True(t, ok)
	assert.Equal(t, rootSC.TraceID, childSC.TraceID, "Child span must retain parent trace ID")
	assert.NotEqual(t, rootSC.SpanID, childSC.SpanID, "Child span must have unique span ID")
}
