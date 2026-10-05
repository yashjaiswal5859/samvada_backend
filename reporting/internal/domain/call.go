// =============================================================================
// Samvada Platform — Reporting Engine Call Detail Record (CDR) Domain
// =============================================================================

package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type CallEvent struct {
	CallID             uuid.UUID  `json:"call_id"`
	OrgID              uuid.UUID  `json:"org_id"`
	WorkspaceID        uuid.UUID  `json:"workspace_id"`
	AgentID            uuid.UUID  `json:"agent_id"`
	CampaignID         *uuid.UUID `json:"campaign_id,omitempty"`
	Direction          string     `json:"direction"` // inbound, outbound
	CallerNumber       string     `json:"caller_number"`
	CalledNumber       string     `json:"called_number"`
	CustomerName       string     `json:"customer_name"`
	StartedAt          time.Time  `json:"started_at"`
	EndedAt            *time.Time `json:"ended_at,omitempty"`
	DurationSec        uint32     `json:"duration_sec"`
	BillableSec        uint32     `json:"billable_sec"`
	Disposition        string     `json:"disposition"` // completed, busy, failed, no_answer, in_progress, escalated
	EvaluationScore    uint8      `json:"evaluation_score"`
	STTLatencyMS       uint16     `json:"stt_latency_ms"`
	LLMLatencyMS       uint16     `json:"llm_latency_ms"`
	TTSLatencyMS       uint16     `json:"tts_latency_ms"`
	E2ELatencyMS       uint16     `json:"e2e_latency_ms"`
	CostTelephonyUSD   float64    `json:"cost_telephony_usd"`
	CostSTTUSD         float64    `json:"cost_stt_usd"`
	CostLLMUSD         float64    `json:"cost_llm_usd"`
	CostTTSUSD         float64    `json:"cost_tts_usd"`
	TotalCostUSD       float64    `json:"total_cost_usd"`
	TurnCount          uint16     `json:"turn_count"`
	OverallSentiment   string     `json:"overall_sentiment"` // positive, neutral, negative
	RecordingURL       string     `json:"recording_url"`
	TranscriptSummary  string     `json:"transcript_summary"`
	CreatedAt          time.Time  `json:"created_at"`
}

type SentimentTurn struct {
	CallID         uuid.UUID `json:"call_id"`
	OrgID          uuid.UUID `json:"org_id"`
	WorkspaceID    uuid.UUID `json:"workspace_id"`
	AgentID        uuid.UUID `json:"agent_id"`
	TurnIndex      uint16    `json:"turn_index"`
	Speaker        string    `json:"speaker"` // agent, customer
	Timestamp      time.Time `json:"timestamp"`
	Text           string    `json:"text"`
	Sentiment      string    `json:"sentiment"` // positive, neutral, negative
	Emotion        string    `json:"emotion"`   // happy, frustrated, neutral, angry, confused
	SentimentScore float32   `json:"sentiment_score"`
	LatencyMS      uint16    `json:"latency_ms"`
}

type CallFilter struct {
	OrgID        uuid.UUID  `json:"org_id"`
	WorkspaceID  *uuid.UUID `json:"workspace_id,omitempty"`
	AgentID      *uuid.UUID `json:"agent_id,omitempty"`
	CampaignID   *uuid.UUID `json:"campaign_id,omitempty"`
	Disposition  *string    `json:"disposition,omitempty"`
	Sentiment    *string    `json:"sentiment,omitempty"`
	Direction    *string    `json:"direction,omitempty"`
	StartDate    *time.Time `json:"start_date,omitempty"`
	EndDate      *time.Time `json:"end_date,omitempty"`
	Limit        int        `json:"limit"`
	Offset       int        `json:"offset"`
}

type CallRepository interface {
	BatchInsertCallEvents(ctx context.Context, events []*CallEvent) error
	BatchInsertSentimentTurns(ctx context.Context, turns []*SentimentTurn) error
	GetCallByID(ctx context.Context, orgID, callID uuid.UUID) (*CallEvent, error)
	ListCalls(ctx context.Context, filter CallFilter) ([]*CallEvent, int64, error)
	GetCallTurns(ctx context.Context, callID uuid.UUID) ([]*SentimentTurn, error)
}
