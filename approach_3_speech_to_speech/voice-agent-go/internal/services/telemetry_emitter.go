// =============================================================================
// Samvada Platform — S2S Real-Time Telemetry & Kafka Event Emitter (Phase 3.1)
// =============================================================================
// Emits call lifecycle, turn-by-turn sentiment, and call completion CDR events
// to Kafka for consumption by the ClickHouse Reporting Engine & ABS billing.
// =============================================================================

package services

import (
	"context"
	"encoding/json"
	"time"

	"github.com/segmentio/kafka-go"
)

type S2SCallStartedEvent struct {
	CallID       string    `json:"call_id"`
	OrgID        string    `json:"org_id"`
	WorkspaceID  string    `json:"workspace_id"`
	AgentID      string    `json:"agent_id"`
	CampaignID   *string   `json:"campaign_id,omitempty"`
	Direction    string    `json:"direction"`
	CallerNumber string    `json:"caller_number"`
	CalledNumber string    `json:"called_number"`
	CustomerName string    `json:"customer_name"`
	StartedAt    time.Time `json:"started_at"`
}

type S2SSentimentTurnEvent struct {
	CallID         string    `json:"call_id"`
	OrgID          string    `json:"org_id"`
	WorkspaceID    string    `json:"workspace_id"`
	AgentID        string    `json:"agent_id"`
	TurnIndex      uint16    `json:"turn_index"`
	Speaker        string    `json:"speaker"`
	Timestamp      time.Time `json:"timestamp"`
	Text           string    `json:"text"`
	Sentiment      string    `json:"sentiment"`
	Emotion        string    `json:"emotion"`
	SentimentScore float32   `json:"sentiment_score"`
	LatencyMS      uint16    `json:"latency_ms"`
}

type S2SCallCompletedEvent struct {
	CallID            string    `json:"call_id"`
	OrgID             string    `json:"org_id"`
	WorkspaceID       string    `json:"workspace_id"`
	AgentID           string    `json:"agent_id"`
	CampaignID        *string   `json:"campaign_id,omitempty"`
	Direction         string    `json:"direction"`
	CallerNumber      string    `json:"caller_number"`
	CalledNumber      string    `json:"called_number"`
	CustomerName      string    `json:"customer_name"`
	StartedAt         time.Time `json:"started_at"`
	EndedAt           time.Time `json:"ended_at"`
	DurationSec       uint32    `json:"duration_sec"`
	BillableSec       uint32    `json:"billable_sec"`
	Disposition       string    `json:"disposition"`
	EvaluationScore   uint8     `json:"evaluation_score"`
	STTLatencyMS      uint16    `json:"stt_latency_ms"`
	LLMLatencyMS      uint16    `json:"llm_latency_ms"`
	TTSLatencyMS      uint16    `json:"tts_latency_ms"`
	E2ELatencyMS      uint16    `json:"e2e_latency_ms"`
	CostTelephonyUSD  float64   `json:"cost_telephony_usd"`
	CostSTTUSD        float64   `json:"cost_stt_usd"`
	CostLLMUSD        float64   `json:"cost_llm_usd"`
	CostTTSUSD        float64   `json:"cost_tts_usd"`
	TotalCostUSD      float64   `json:"total_cost_usd"`
	TurnCount         uint16    `json:"turn_count"`
	OverallSentiment  string    `json:"overall_sentiment"`
	RecordingURL      string    `json:"recording_url,omitempty"`
	TranscriptSummary string    `json:"transcript_summary,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

type TelemetryEmitter struct {
	kafkaBrokers []string
	writer       *kafka.Writer
}

func NewTelemetryEmitter(brokers []string) *TelemetryEmitter {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireOne,
		Async:        true,
	}
	return &TelemetryEmitter{
		kafkaBrokers: brokers,
		writer:       writer,
	}
}

func (e *TelemetryEmitter) Close() error {
	if e.writer != nil {
		return e.writer.Close()
	}
	return nil
}

func (e *TelemetryEmitter) EmitCallStarted(ctx context.Context, ev S2SCallStartedEvent) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return e.writer.WriteMessages(ctx, kafka.Message{
		Topic: "s2s.call.started",
		Key:   []byte(ev.CallID),
		Value: data,
	})
}

func (e *TelemetryEmitter) EmitSentimentTurn(ctx context.Context, ev S2SSentimentTurnEvent) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return e.writer.WriteMessages(ctx, kafka.Message{
		Topic: "s2s.call.turns",
		Key:   []byte(ev.CallID),
		Value: data,
	})
}

func (e *TelemetryEmitter) EmitCallCompleted(ctx context.Context, ev S2SCallCompletedEvent) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return e.writer.WriteMessages(ctx, kafka.Message{
		Topic: "s2s.call.completed",
		Key:   []byte(ev.CallID),
		Value: data,
	})
}
