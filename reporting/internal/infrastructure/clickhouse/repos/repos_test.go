// =============================================================================
// Samvada Platform — ClickHouse Repositories Integration Test
// =============================================================================

package repos_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/samvada/reporting/configs"
	"github.com/samvada/reporting/internal/domain"
	"github.com/samvada/reporting/internal/infrastructure/clickhouse"
	"github.com/samvada/reporting/internal/infrastructure/clickhouse/repos"
)

func setupTestCH(t *testing.T) (domain.CallRepository, domain.AnalyticsRepository, domain.UsageRepository) {
	cfg := &configs.ClickHouseConfig{
		Addr:           "localhost:9000",
		Database:       "samvada_analytics",
		User:           "samvada",
		Password:       "samvada_ch_2026",
		DialTimeoutSec: 5,
		MaxOpenConns:   10,
		MaxIdleConns:   5,
	}

	logger := zap.NewNop()
	conn, err := clickhouse.NewConnection(cfg, logger)
	if err != nil {
		t.Skipf("Skipping test: ClickHouse is not reachable on localhost:9000: %v", err)
		return nil, nil, nil
	}

	callRepo := repos.NewCallRepository(conn, logger)
	analyticsRepo := repos.NewAnalyticsRepository(conn, logger)
	usageRepo := repos.NewUsageRepository(conn, logger)

	return callRepo, analyticsRepo, usageRepo
}

func TestClickHouse_CallEventsAndRollups(t *testing.T) {
	callRepo, analyticsRepo, usageRepo := setupTestCH(t)
	ctx := context.Background()

	orgID := uuid.New()
	wsID := uuid.New()
	agentID := uuid.New()
	callID := uuid.New()

	now := time.Now().UTC()
	start := now.Add(-10 * time.Minute)
	end := now.Add(-5 * time.Minute)

	// 1. Insert Call Event
	callEvent := &domain.CallEvent{
		CallID:            callID,
		OrgID:             orgID,
		WorkspaceID:       wsID,
		AgentID:           agentID,
		Direction:         "inbound",
		CallerNumber:      "+18005550199",
		CalledNumber:      "+18005550100",
		CustomerName:      "Sarah Connor",
		StartedAt:         start,
		EndedAt:           &end,
		DurationSec:       300,
		BillableSec:       300,
		Disposition:       "completed",
		EvaluationScore:   92,
		STTLatencyMS:      120,
		LLMLatencyMS:      450,
		TTSLatencyMS:      180,
		E2ELatencyMS:      750,
		CostTelephonyUSD:  0.05,
		CostSTTUSD:        0.03,
		CostLLMUSD:        0.12,
		CostTTSUSD:        0.04,
		TotalCostUSD:      0.24,
		TurnCount:         8,
		OverallSentiment:  "positive",
		RecordingURL:      "https://storage.samvada.ai/recordings/test.wav",
		TranscriptSummary: "Customer inquired about premium enterprise plan features.",
		CreatedAt:         now,
	}

	err := callRepo.BatchInsertCallEvents(ctx, []*domain.CallEvent{callEvent})
	require.NoError(t, err)

	// 2. Insert Sentiment Turns
	turn1 := &domain.SentimentTurn{
		CallID:         callID,
		OrgID:          orgID,
		WorkspaceID:    wsID,
		AgentID:        agentID,
		TurnIndex:      1,
		Speaker:        "agent",
		Timestamp:      start.Add(5 * time.Second),
		Text:           "Hello! Thank you for calling Samvada. How may I assist you today?",
		Sentiment:      "positive",
		Emotion:        "happy",
		SentimentScore: 0.85,
		LatencyMS:      720,
	}
	turn2 := &domain.SentimentTurn{
		CallID:         callID,
		OrgID:          orgID,
		WorkspaceID:    wsID,
		AgentID:        agentID,
		TurnIndex:      2,
		Speaker:        "customer",
		Timestamp:      start.Add(15 * time.Second),
		Text:           "I want to know more about the voice AI pricing.",
		Sentiment:      "neutral",
		Emotion:        "neutral",
		SentimentScore: 0.1,
		LatencyMS:      0,
	}

	err = callRepo.BatchInsertSentimentTurns(ctx, []*domain.SentimentTurn{turn1, turn2})
	require.NoError(t, err)

	// 3. Test GetCallByID
	fetchedCall, err := callRepo.GetCallByID(ctx, orgID, callID)
	require.NoError(t, err)
	assert.Equal(t, callID, fetchedCall.CallID)
	assert.Equal(t, "completed", fetchedCall.Disposition)
	assert.Equal(t, uint32(300), fetchedCall.DurationSec)
	assert.Equal(t, 0.24, fetchedCall.TotalCostUSD)

	// 4. Test GetCallTurns
	turns, err := callRepo.GetCallTurns(ctx, callID)
	require.NoError(t, err)
	assert.Len(t, turns, 2)
	assert.Equal(t, "agent", turns[0].Speaker)
	assert.Equal(t, "customer", turns[1].Speaker)

	// 5. Test ListCalls
	calls, total, err := callRepo.ListCalls(ctx, domain.CallFilter{
		OrgID:       orgID,
		WorkspaceID: &wsID,
		Limit:       10,
	})
	require.NoError(t, err)
	assert.True(t, total >= 1)
	assert.NotEmpty(t, calls)

	// 6. Test Analytics KPI Summary
	kpi, err := analyticsRepo.GetKPISummary(ctx, orgID, &wsID, start.Add(-1*time.Hour), end.Add(1*time.Hour))
	require.NoError(t, err)
	assert.True(t, kpi.TotalCalls >= 1)
	assert.Equal(t, int64(1), kpi.CompletedCalls)
	assert.Equal(t, 100.0, kpi.SuccessRatePercent)
	assert.Equal(t, 0.24, kpi.TotalSpendUSD)

	// 7. Test Usage Reporting
	usage, err := usageRepo.GetUsageSummary(ctx, orgID, start.Add(-24*time.Hour), end.Add(24*time.Hour))
	require.NoError(t, err)
	assert.NotNil(t, usage)
}
