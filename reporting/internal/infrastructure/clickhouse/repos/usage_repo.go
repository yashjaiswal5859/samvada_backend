// =============================================================================
// Samvada Platform — ClickHouse Usage & Metering Repository
// =============================================================================

package repos

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/reporting/internal/domain"
)

type usageRepo struct {
	conn driver.Conn
	log  *zap.Logger
}

func NewUsageRepository(conn driver.Conn, log *zap.Logger) domain.UsageRepository {
	return &usageRepo{
		conn: conn,
		log:  log.Named("clickhouse_usage_repo"),
	}
}

func (r *usageRepo) GetUsageSummary(
	ctx context.Context,
	orgID uuid.UUID,
	start, end time.Time,
) (*domain.UsageSummary, error) {
	queryTotal := `
		SELECT
			toInt64(sum(total_calls)) AS total_calls,
			toInt64(sum(total_duration_sec)) AS total_duration_seconds,
			toInt64(sum(total_billable_sec)) AS total_billable_seconds,
			toFloat64(sum(spend_telephony_usd)) AS total_cost_telephony_usd,
			toFloat64(sum(spend_stt_usd)) AS total_cost_stt_usd,
			toFloat64(sum(spend_llm_usd)) AS total_cost_llm_usd,
			toFloat64(sum(spend_tts_usd)) AS total_cost_tts_usd,
			toFloat64(sum(total_spend_usd)) AS total_spend_usd
		FROM mv_daily_usage
		WHERE org_id = ? AND date >= toDate(?) AND date <= toDate(?)
	`

	var s domain.UsageSummary
	s.PerWorkspaceMinutes = make(map[string]int64)

	row := r.conn.QueryRow(ctx, queryTotal, orgID, start, end)
	err := row.Scan(
		&s.TotalCalls,
		&s.TotalDurationSeconds,
		&s.TotalBillableSeconds,
		&s.TotalCostTelephonyUSD,
		&s.TotalCostSTTUSD,
		&s.TotalCostLLMUSD,
		&s.TotalCostTTSUSD,
		&s.TotalSpendUSD,
	)
	if err != nil {
		return nil, fmt.Errorf("scanning aggregate usage: %w", err)
	}

	queryPerWS := `
		SELECT
			toString(workspace_id) AS ws_id,
			toInt64(intDiv(sum(total_billable_sec), 60)) AS minutes
		FROM mv_daily_usage
		WHERE org_id = ? AND date >= toDate(?) AND date <= toDate(?)
		GROUP BY workspace_id
	`
	rows, err := r.conn.Query(ctx, queryPerWS, orgID, start, end)
	if err != nil {
		return nil, fmt.Errorf("querying per-workspace usage: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var wsID string
		var minutes int64
		if err := rows.Scan(&wsID, &minutes); err != nil {
			return nil, fmt.Errorf("scanning per-workspace usage: %w", err)
		}
		s.PerWorkspaceMinutes[wsID] = minutes
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating per-workspace usage rows: %w", err)
	}

	return &s, nil
}

func (r *usageRepo) GetCurrentMonthUsage(
	ctx context.Context,
	orgID, workspaceID uuid.UUID,
) (*domain.CurrentMonthUsage, error) {
	query := `
		SELECT
			toInt64(intDiv(sum(total_billable_sec), 60)) AS minutes_used,
			toFloat64(sum(total_spend_usd)) AS estimated_spend_usd
		FROM mv_daily_usage
		WHERE org_id = ? AND workspace_id = ? AND date >= toStartOfMonth(today())
	`
	var u domain.CurrentMonthUsage
	u.MinutesLimit = 10000 // Standard workspace quota (10,000 mins)

	row := r.conn.QueryRow(ctx, query, orgID, workspaceID)
	if err := row.Scan(&u.MinutesUsed, &u.EstimatedSpendUSD); err != nil {
		return nil, fmt.Errorf("scanning current month usage: %w", err)
	}

	if u.MinutesLimit > 0 {
		u.PercentUsed = float64(u.MinutesUsed) * 100.0 / float64(u.MinutesLimit)
	}

	return &u, nil
}
