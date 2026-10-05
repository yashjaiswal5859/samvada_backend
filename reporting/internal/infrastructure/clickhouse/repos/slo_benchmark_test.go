// =============================================================================
// Samvada Platform — ClickHouse Dashboard Query Latency SLO Benchmark (Phase 6.3)
// Enforces < 100ms SLO target for core dashboard aggregations
// =============================================================================

package repos_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/samvada/reporting/internal/domain"
)

const TargetLatencySLO = 100 * time.Millisecond

func TestClickHouse_QueryLatencySLO(t *testing.T) {
	callRepo, analyticsRepo, _ := setupTestCH(t)
	if analyticsRepo == nil {
		t.Skip("ClickHouse is offline, skipping live query latency SLO benchmark")
		return
	}

	ctx := context.Background()
	orgID := uuid.New()
	wsID := uuid.New()
	from := time.Now().Add(-24 * time.Hour)
	to := time.Now()

	// 1. Benchmark: KPI Summary Dashboard Aggregation
	t.Run("SLO_KPISummaryAggregation", func(t *testing.T) {
		start := time.Now()
		_, err := analyticsRepo.GetKPISummary(ctx, orgID, &wsID, from, to)
		latency := time.Since(start)

		require.NoError(t, err)
		assert.LessOrEqual(t, latency, TargetLatencySLO, "KPI Summary query exceeded 100ms SLO threshold")
		t.Logf("KPI summary aggregation executed in %v (SLO: <%v)", latency, TargetLatencySLO)
	})

	// 2. Benchmark: Daily Volume Trends (30-day rollup)
	t.Run("SLO_DailyVolumeTrend", func(t *testing.T) {
		start := time.Now()
		_, err := analyticsRepo.GetDailyVolumeTrend(ctx, orgID, &wsID, 30)
		latency := time.Since(start)

		require.NoError(t, err)
		assert.LessOrEqual(t, latency, TargetLatencySLO, "Daily volume trend query exceeded 100ms SLO threshold")
		t.Logf("Daily volume trend query executed in %v (SLO: <%v)", latency, TargetLatencySLO)
	})

	// 3. Benchmark: Agent Performance Leaderboard
	t.Run("SLO_AgentLeaderboard", func(t *testing.T) {
		start := time.Now()
		_, err := analyticsRepo.GetAgentLeaderboard(ctx, orgID, &wsID, from, to, 10)
		latency := time.Since(start)

		require.NoError(t, err)
		assert.LessOrEqual(t, latency, TargetLatencySLO, "Agent leaderboard query exceeded 100ms SLO threshold")
		t.Logf("Agent leaderboard query executed in %v (SLO: <%v)", latency, TargetLatencySLO)
	})

	// 4. Benchmark: Call History Filtered List
	t.Run("SLO_CallHistoryFiltered", func(t *testing.T) {
		start := time.Now()
		_, _, err := callRepo.ListCalls(ctx, domain.CallFilter{
			OrgID:       orgID,
			WorkspaceID: &wsID,
			Limit:       50,
			Offset:      0,
		})
		latency := time.Since(start)

		require.NoError(t, err)
		assert.LessOrEqual(t, latency, TargetLatencySLO, "Call list query exceeded 100ms SLO threshold")
		t.Logf("Call list query executed in %v (SLO: <%v)", latency, TargetLatencySLO)
	})
}
