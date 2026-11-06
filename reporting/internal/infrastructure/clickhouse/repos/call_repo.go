// =============================================================================
// Samvada Platform — ClickHouse Call Events Repository
// =============================================================================

package repos

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/samvada/reporting/internal/domain"
)

type callRepo struct {
	conn driver.Conn
	log  *zap.Logger
}

func NewCallRepository(conn driver.Conn, log *zap.Logger) domain.CallRepository {
	return &callRepo{
		conn: conn,
		log:  log.Named("clickhouse_call_repo"),
	}
}

func (r *callRepo) BatchInsertCallEvents(ctx context.Context, events []*domain.CallEvent) error {
	if len(events) == 0 {
		return nil
	}

	batch, err := r.conn.PrepareBatch(ctx, "INSERT INTO call_events")
	if err != nil {
		return fmt.Errorf("prepare batch for call_events: %w", err)
	}

	for _, ev := range events {
		if ev.CreatedAt.IsZero() {
			ev.CreatedAt = time.Now().UTC()
		}
		err := batch.Append(
			ev.CallID,
			ev.OrgID,
			ev.WorkspaceID,
			ev.AgentID,
			ev.CampaignID,
			ev.Direction,
			ev.CallerNumber,
			ev.CalledNumber,
			ev.CustomerName,
			ev.StartedAt,
			ev.EndedAt,
			ev.DurationSec,
			ev.BillableSec,
			ev.Disposition,
			ev.EvaluationScore,
			ev.STTLatencyMS,
			ev.LLMLatencyMS,
			ev.TTSLatencyMS,
			ev.E2ELatencyMS,
			decimal.NewFromFloat(ev.CostTelephonyUSD),
			decimal.NewFromFloat(ev.CostSTTUSD),
			decimal.NewFromFloat(ev.CostLLMUSD),
			decimal.NewFromFloat(ev.CostTTSUSD),
			decimal.NewFromFloat(ev.TotalCostUSD),
			ev.TurnCount,
			ev.OverallSentiment,
			ev.RecordingURL,
			ev.TranscriptSummary,
			ev.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("appending event to batch: %w", err)
		}
	}

	return batch.Send()
}

func (r *callRepo) BatchInsertSentimentTurns(ctx context.Context, turns []*domain.SentimentTurn) error {
	if len(turns) == 0 {
		return nil
	}

	batch, err := r.conn.PrepareBatch(ctx, "INSERT INTO sentiment_turns")
	if err != nil {
		return fmt.Errorf("prepare batch for sentiment_turns: %w", err)
	}

	for _, t := range turns {
		if t.Timestamp.IsZero() {
			t.Timestamp = time.Now().UTC()
		}
		err := batch.Append(
			t.CallID,
			t.OrgID,
			t.WorkspaceID,
			t.AgentID,
			t.TurnIndex,
			t.Speaker,
			t.Timestamp,
			t.Text,
			t.Sentiment,
			t.Emotion,
			t.SentimentScore,
			t.LatencyMS,
		)
		if err != nil {
			return fmt.Errorf("appending turn to batch: %w", err)
		}
	}

	return batch.Send()
}

func (r *callRepo) GetCallByID(ctx context.Context, orgID, callID uuid.UUID) (*domain.CallEvent, error) {
	query := `
		SELECT
			call_id, org_id, workspace_id, agent_id, campaign_id,
			direction, caller_number, called_number, customer_name,
			started_at, ended_at, duration_sec, billable_sec,
			disposition, evaluation_score,
			stt_latency_ms, llm_latency_ms, tts_latency_ms, e2e_latency_ms,
			toFloat64(cost_telephony_usd) AS cost_telephony_usd,
			toFloat64(cost_stt_usd) AS cost_stt_usd,
			toFloat64(cost_llm_usd) AS cost_llm_usd,
			toFloat64(cost_tts_usd) AS cost_tts_usd,
			toFloat64(total_cost_usd) AS total_cost_usd,
			turn_count, overall_sentiment, recording_url, transcript_summary, created_at
		FROM call_events
		WHERE org_id = ? AND call_id = ?
		LIMIT 1
	`
	var ev domain.CallEvent
	row := r.conn.QueryRow(ctx, query, orgID, callID)
	err := row.Scan(
		&ev.CallID, &ev.OrgID, &ev.WorkspaceID, &ev.AgentID, &ev.CampaignID,
		&ev.Direction, &ev.CallerNumber, &ev.CalledNumber, &ev.CustomerName,
		&ev.StartedAt, &ev.EndedAt, &ev.DurationSec, &ev.BillableSec,
		&ev.Disposition, &ev.EvaluationScore,
		&ev.STTLatencyMS, &ev.LLMLatencyMS, &ev.TTSLatencyMS, &ev.E2ELatencyMS,
		&ev.CostTelephonyUSD, &ev.CostSTTUSD, &ev.CostLLMUSD, &ev.CostTTSUSD, &ev.TotalCostUSD,
		&ev.TurnCount, &ev.OverallSentiment, &ev.RecordingURL, &ev.TranscriptSummary, &ev.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &ev, nil
}

func (r *callRepo) ListCalls(ctx context.Context, filter domain.CallFilter) ([]*domain.CallEvent, int64, error) {
	var where []string
	var args []interface{}

	if filter.OrgID != uuid.Nil {
		where = append(where, "org_id = ?")
		args = append(args, filter.OrgID)
	}

	if filter.WorkspaceID != nil {
		where = append(where, "workspace_id = ?")
		args = append(args, *filter.WorkspaceID)
	}

	if len(where) == 0 {
		where = append(where, "1 = 1")
	}
	if filter.AgentID != nil {
		where = append(where, "agent_id = ?")
		args = append(args, *filter.AgentID)
	}
	if filter.CampaignID != nil {
		where = append(where, "campaign_id = ?")
		args = append(args, *filter.CampaignID)
	}
	if filter.Disposition != nil {
		where = append(where, "disposition = ?")
		args = append(args, *filter.Disposition)
	}
	if filter.Sentiment != nil {
		where = append(where, "overall_sentiment = ?")
		args = append(args, *filter.Sentiment)
	}
	if filter.Direction != nil {
		where = append(where, "direction = ?")
		args = append(args, *filter.Direction)
	}
	if filter.StartDate != nil {
		where = append(where, "started_at >= ?")
		args = append(args, *filter.StartDate)
	}
	if filter.EndDate != nil {
		where = append(where, "started_at <= ?")
		args = append(args, *filter.EndDate)
	}

	whereClause := strings.Join(where, " AND ")

	// 1. Get total count
	countQuery := fmt.Sprintf("SELECT toInt64(count()) FROM call_events WHERE %s", whereClause)
	var total int64
	if err := r.conn.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count query: %w", err)
	}

	// 2. Query paginated rows
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	query := fmt.Sprintf(`
		SELECT
			call_id, org_id, workspace_id, agent_id, campaign_id,
			direction, caller_number, called_number, customer_name,
			started_at, ended_at, duration_sec, billable_sec,
			disposition, evaluation_score,
			stt_latency_ms, llm_latency_ms, tts_latency_ms, e2e_latency_ms,
			toFloat64(cost_telephony_usd) AS cost_telephony_usd,
			toFloat64(cost_stt_usd) AS cost_stt_usd,
			toFloat64(cost_llm_usd) AS cost_llm_usd,
			toFloat64(cost_tts_usd) AS cost_tts_usd,
			toFloat64(total_cost_usd) AS total_cost_usd,
			turn_count, overall_sentiment, recording_url, transcript_summary, created_at
		FROM call_events
		WHERE %s
		ORDER BY started_at DESC
		LIMIT ? OFFSET ?
	`, whereClause)

	queryArgs := append(args, limit, offset)
	rows, err := r.conn.Query(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying call_events: %w", err)
	}
	defer rows.Close()

	var events []*domain.CallEvent
	for rows.Next() {
		var ev domain.CallEvent
		err := rows.Scan(
			&ev.CallID, &ev.OrgID, &ev.WorkspaceID, &ev.AgentID, &ev.CampaignID,
			&ev.Direction, &ev.CallerNumber, &ev.CalledNumber, &ev.CustomerName,
			&ev.StartedAt, &ev.EndedAt, &ev.DurationSec, &ev.BillableSec,
			&ev.Disposition, &ev.EvaluationScore,
			&ev.STTLatencyMS, &ev.LLMLatencyMS, &ev.TTSLatencyMS, &ev.E2ELatencyMS,
			&ev.CostTelephonyUSD, &ev.CostSTTUSD, &ev.CostLLMUSD, &ev.CostTTSUSD, &ev.TotalCostUSD,
			&ev.TurnCount, &ev.OverallSentiment, &ev.RecordingURL, &ev.TranscriptSummary, &ev.CreatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning call row: %w", err)
		}
		events = append(events, &ev)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating call rows: %w", err)
	}

	return events, total, nil
}

func (r *callRepo) GetCallTurns(ctx context.Context, callID uuid.UUID) ([]*domain.SentimentTurn, error) {
	query := `
		SELECT
			call_id, org_id, workspace_id, agent_id, turn_index,
			speaker, timestamp, text, sentiment, emotion, sentiment_score, latency_ms
		FROM sentiment_turns
		WHERE call_id = ?
		ORDER BY turn_index ASC
	`
	rows, err := r.conn.Query(ctx, query, callID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var turns []*domain.SentimentTurn
	for rows.Next() {
		var t domain.SentimentTurn
		err := rows.Scan(
			&t.CallID, &t.OrgID, &t.WorkspaceID, &t.AgentID, &t.TurnIndex,
			&t.Speaker, &t.Timestamp, &t.Text, &t.Sentiment, &t.Emotion, &t.SentimentScore, &t.LatencyMS,
		)
		if err != nil {
			return nil, err
		}
		turns = append(turns, &t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating call turn rows: %w", err)
	}

	return turns, nil
}
