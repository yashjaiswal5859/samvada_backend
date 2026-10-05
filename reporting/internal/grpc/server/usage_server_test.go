// =============================================================================
// Samvada Platform — Reporting Engine gRPC Server Test
// =============================================================================

package server_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/samvada/reporting/configs"
	"github.com/samvada/reporting/internal/application/usage"
	"github.com/samvada/reporting/internal/grpc/server"
	"github.com/samvada/reporting/internal/infrastructure/clickhouse"
	"github.com/samvada/reporting/internal/infrastructure/clickhouse/repos"
	reportingv1 "github.com/samvada/proto/gen/go/reporting/v1"
)

func TestUsageServer_gRPC(t *testing.T) {
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
	chConn, err := clickhouse.NewConnection(cfg, logger)
	if err != nil {
		t.Skipf("Skipping test: ClickHouse is not reachable on localhost:9000: %v", err)
		return
	}

	usageRepo := repos.NewUsageRepository(chConn, logger)
	meteringService := usage.NewMeteringService(usageRepo, logger)
	usageServer := server.NewUsageServer(meteringService, logger)

	ctx := context.Background()
	orgID := uuid.New()
	wsID := uuid.New()

	// 1. Test GetUsageSummary
	summaryResp, err := usageServer.GetUsageSummary(ctx, &reportingv1.GetUsageSummaryRequest{
		OrgId:       orgID.String(),
		PeriodStart: time.Now().AddDate(0, 0, -30).Format(time.RFC3339),
		PeriodEnd:   time.Now().Format(time.RFC3339),
	})
	require.NoError(t, err)
	assert.NotNil(t, summaryResp)

	// 2. Test GetCurrentMonthUsage
	monthResp, err := usageServer.GetCurrentMonthUsage(ctx, &reportingv1.GetCurrentMonthUsageRequest{
		OrgId:       orgID.String(),
		WorkspaceId: wsID.String(),
	})
	require.NoError(t, err)
	assert.NotNil(t, monthResp)
	assert.Equal(t, int64(10000), monthResp.GetMinutesLimit())
}
