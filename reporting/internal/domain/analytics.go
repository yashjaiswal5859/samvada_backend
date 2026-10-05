// =============================================================================
// Samvada Platform — Reporting Engine Analytics Domain Models
// =============================================================================

package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type KPISummary struct {
	TotalCalls               int64   `json:"total_calls"`
	CompletedCalls           int64   `json:"completed_calls"`
	FailedCalls              int64   `json:"failed_calls"`
	EscalatedCalls           int64   `json:"escalated_calls"`
	TotalDurationSec         int64   `json:"total_duration_sec"`
	TotalBillableSec         int64   `json:"total_billable_sec"`
	AvgDurationSec           float64 `json:"avg_duration_sec"`
	SuccessRatePercent       float64 `json:"success_rate_percent"`
	TotalSpendUSD            float64 `json:"total_spend_usd"`
	AvgE2ELatencyMS          float64 `json:"avg_e2e_latency_ms"`
	PositiveSentimentPercent float64 `json:"positive_sentiment_percent"`
	NeutralSentimentPercent  float64 `json:"neutral_sentiment_percent"`
	NegativeSentimentPercent float64 `json:"negative_sentiment_percent"`
}

type DailyVolumeTrend struct {
	Date            string  `json:"date"`
	TotalCalls      int64   `json:"total_calls"`
	CompletedCalls  int64   `json:"completed_calls"`
	TotalMinutes    float64 `json:"total_minutes"`
	TotalSpendUSD   float64 `json:"total_spend_usd"`
	AvgE2ELatencyMS float64 `json:"avg_e2e_latency_ms"`
}

type AgentLeaderboardItem struct {
	AgentID            uuid.UUID `json:"agent_id"`
	AgentName          string    `json:"agent_name"`
	TotalCalls         int64     `json:"total_calls"`
	SuccessfulCalls    int64     `json:"successful_calls"`
	SuccessRatePercent float64   `json:"success_rate_percent"`
	TotalMinutes       float64   `json:"total_minutes"`
	AvgScore           float64   `json:"avg_score"`
	AvgLatencyMS       float64   `json:"avg_latency_ms"`
	PositiveCalls      int64     `json:"positive_calls"`
	NegativeCalls      int64     `json:"negative_calls"`
}

type ConversionFunnelStage struct {
	Date           string  `json:"date"`
	TotalDialed    int64   `json:"total_dialed"`
	ConnectedCalls int64   `json:"connected_calls"`
	EngagedCalls   int64   `json:"engaged_calls"`
	QualifiedCalls int64   `json:"qualified_calls"`
	ConvertedCalls int64   `json:"converted_calls"`
	ConversionRate float64 `json:"conversion_rate"`
}

type AnalyticsRepository interface {
	GetKPISummary(ctx context.Context, orgID uuid.UUID, workspaceID *uuid.UUID, start, end time.Time) (*KPISummary, error)
	GetDailyVolumeTrend(ctx context.Context, orgID uuid.UUID, workspaceID *uuid.UUID, days int) ([]*DailyVolumeTrend, error)
	GetAgentLeaderboard(ctx context.Context, orgID uuid.UUID, workspaceID *uuid.UUID, start, end time.Time, limit int) ([]*AgentLeaderboardItem, error)
	GetConversionFunnels(ctx context.Context, orgID uuid.UUID, workspaceID *uuid.UUID, start, end time.Time) ([]*ConversionFunnelStage, error)
}
