// =============================================================================
// Samvada Platform — ClickHouse Analytics & KPI Repository
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

type analyticsRepo struct {
	conn driver.Conn
	log  *zap.Logger
}

func NewAnalyticsRepository(conn driver.Conn, log *zap.Logger) domain.AnalyticsRepository {
	return &analyticsRepo{
		conn: conn,
		log:  log.Named("clickhouse_analytics_repo"),
	}
}

func (r *analyticsRepo) GetKPISummary(
	ctx context.Context,
	orgID uuid.UUID,
	workspaceID *uuid.UUID,
	start, end time.Time,
) (*domain.KPISummary, error) {
	where := "started_at >= ? AND started_at <= ?"
	args := []interface{}{start, end}

	if orgID != uuid.Nil {
		where += " AND org_id = ?"
		args = append(args, orgID)
	}

	if workspaceID != nil {
		where += " AND workspace_id = ?"
		args = append(args, *workspaceID)
	}

	query := fmt.Sprintf(`
		SELECT
			toInt64(count()) AS total_calls,
			toInt64(countIf(disposition = 'completed')) AS completed_calls,
			toInt64(countIf(disposition = 'failed')) AS failed_calls,
			toInt64(countIf(disposition = 'escalated')) AS escalated_calls,
			toInt64(sum(duration_sec)) AS total_duration_sec,
			toInt64(sum(billable_sec)) AS total_billable_sec,
			toFloat64(if(count() > 0, avg(duration_sec), 0.0)) AS avg_duration_sec,
			toFloat64(if(count() > 0, countIf(disposition = 'completed') * 100.0 / count(), 0.0)) AS success_rate_percent,
			toFloat64(sum(total_cost_usd)) AS total_spend_usd,
			toFloat64(if(count() > 0, avg(e2e_latency_ms), 0.0)) AS avg_e2e_latency_ms,
			toFloat64(if(count() > 0, countIf(overall_sentiment = 'positive') * 100.0 / count(), 0.0)) AS positive_sentiment_percent,
			toFloat64(if(count() > 0, countIf(overall_sentiment = 'neutral') * 100.0 / count(), 0.0)) AS neutral_sentiment_percent,
			toFloat64(if(count() > 0, countIf(overall_sentiment = 'negative') * 100.0 / count(), 0.0)) AS negative_sentiment_percent
		FROM call_events
		WHERE %s
	`, where)

	var s domain.KPISummary
	row := r.conn.QueryRow(ctx, query, args...)
	err := row.Scan(
		&s.TotalCalls,
		&s.CompletedCalls,
		&s.FailedCalls,
		&s.EscalatedCalls,
		&s.TotalDurationSec,
		&s.TotalBillableSec,
		&s.AvgDurationSec,
		&s.SuccessRatePercent,
		&s.TotalSpendUSD,
		&s.AvgE2ELatencyMS,
		&s.PositiveSentimentPercent,
		&s.NeutralSentimentPercent,
		&s.NegativeSentimentPercent,
	)
	if err != nil {
		return nil, fmt.Errorf("scanning kpi summary: %w", err)
	}

	return &s, nil
}

func (r *analyticsRepo) GetDailyVolumeTrend(
	ctx context.Context,
	orgID uuid.UUID,
	workspaceID *uuid.UUID,
	days int,
) ([]*domain.DailyVolumeTrend, error) {
	if days <= 0 {
		days = 30
	}

	where := "date >= subtractDays(today(), ?)"
	args := []interface{}{days}

	if orgID != uuid.Nil {
		where += " AND org_id = ?"
		args = append(args, orgID)
	}

	if workspaceID != nil {
		where += " AND workspace_id = ?"
		args = append(args, *workspaceID)
	}

	query := fmt.Sprintf(`
		SELECT
			toString(date) AS date_str,
			toInt64(sum(total_calls)) AS total_calls,
			toInt64(sum(completed_calls)) AS completed_calls,
			toFloat64(sum(total_duration_sec) / 60.0) AS total_minutes,
			toFloat64(sum(total_spend_usd)) AS total_spend_usd,
			toFloat64(avg(avg_e2e_latency_ms)) AS avg_e2e_latency_ms
		FROM mv_daily_usage
		WHERE %s
		GROUP BY date
		ORDER BY date ASC
	`, where)

	rows, err := r.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying daily volume trend: %w", err)
	}
	defer rows.Close()

	var trends []*domain.DailyVolumeTrend
	for rows.Next() {
		var t domain.DailyVolumeTrend
		if err := rows.Scan(&t.Date, &t.TotalCalls, &t.CompletedCalls, &t.TotalMinutes, &t.TotalSpendUSD, &t.AvgE2ELatencyMS); err != nil {
			return nil, fmt.Errorf("scanning daily volume row: %w", err)
		}
		trends = append(trends, &t)
	}

	return trends, nil
}

func (r *analyticsRepo) GetAgentLeaderboard(
	ctx context.Context,
	orgID uuid.UUID,
	workspaceID *uuid.UUID,
	start, end time.Time,
	limit int,
) ([]*domain.AgentLeaderboardItem, error) {
	if limit <= 0 {
		limit = 10
	}

	where := "started_at >= ? AND started_at <= ?"
	args := []interface{}{start, end}

	if orgID != uuid.Nil {
		where += " AND org_id = ?"
		args = append(args, orgID)
	}

	if workspaceID != nil {
		where += " AND workspace_id = ?"
		args = append(args, *workspaceID)
	}

	query := fmt.Sprintf(`
		SELECT
			agent_id,
			toInt64(count()) AS total_calls,
			toInt64(countIf(disposition = 'completed')) AS successful_calls,
			toFloat64(if(count() > 0, countIf(disposition = 'completed') * 100.0 / count(), 0.0)) AS success_rate_percent,
			toFloat64(sum(duration_sec) / 60.0) AS total_minutes,
			toFloat64(if(count() > 0, avg(evaluation_score), 0.0)) AS avg_score,
			toFloat64(if(count() > 0, avg(e2e_latency_ms), 0.0)) AS avg_latency_ms,
			toInt64(countIf(overall_sentiment = 'positive')) AS positive_calls,
			toInt64(countIf(overall_sentiment = 'negative')) AS negative_calls
		FROM call_events
		WHERE %s
		GROUP BY agent_id
		ORDER BY total_calls DESC
		LIMIT ?
	`, where)

	queryArgs := append(args, limit)
	rows, err := r.conn.Query(ctx, query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("querying agent leaderboard: %w", err)
	}
	defer rows.Close()

	var items []*domain.AgentLeaderboardItem
	for rows.Next() {
		var item domain.AgentLeaderboardItem
		err := rows.Scan(
			&item.AgentID,
			&item.TotalCalls,
			&item.SuccessfulCalls,
			&item.SuccessRatePercent,
			&item.TotalMinutes,
			&item.AvgScore,
			&item.AvgLatencyMS,
			&item.PositiveCalls,
			&item.NegativeCalls,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning agent leaderboard item: %w", err)
		}
		item.AgentName = "Agent " + item.AgentID.String()[:8]
		items = append(items, &item)
	}

	return items, nil
}

func (r *analyticsRepo) GetConversionFunnels(
	ctx context.Context,
	orgID uuid.UUID,
	workspaceID *uuid.UUID,
	start, end time.Time,
) ([]*domain.ConversionFunnelStage, error) {
	where := "date >= toDate(?) AND date <= toDate(?)"
	args := []interface{}{start, end}

	if orgID != uuid.Nil {
		where += " AND org_id = ?"
		args = append(args, orgID)
	}

	if workspaceID != nil {
		where += " AND workspace_id = ?"
		args = append(args, *workspaceID)
	}

	query := fmt.Sprintf(`
		SELECT
			date_str,
			total_dialed,
			connected_calls,
			engaged_calls,
			qualified_calls,
			converted_calls,
			toFloat64(if(total_dialed > 0, converted_calls * 100.0 / total_dialed, 0.0)) AS conversion_rate
		FROM (
			SELECT
				toString(date) AS date_str,
				toInt64(sum(total_dialed)) AS total_dialed,
				toInt64(sum(connected_calls)) AS connected_calls,
				toInt64(sum(engaged_calls)) AS engaged_calls,
				toInt64(sum(qualified_calls)) AS qualified_calls,
				toInt64(sum(converted_calls)) AS converted_calls
			FROM mv_conversion_funnels
			WHERE %s
			GROUP BY date
			ORDER BY date ASC
		)
	`, where)

	rows, err := r.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying conversion funnels: %w", err)
	}
	defer rows.Close()

	var stages []*domain.ConversionFunnelStage
	for rows.Next() {
		var st domain.ConversionFunnelStage
		err := rows.Scan(
			&st.Date,
			&st.TotalDialed,
			&st.ConnectedCalls,
			&st.EngagedCalls,
			&st.QualifiedCalls,
			&st.ConvertedCalls,
			&st.ConversionRate,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning conversion funnel row: %w", err)
		}
		stages = append(stages, &st)
	}

	return stages, nil
}
