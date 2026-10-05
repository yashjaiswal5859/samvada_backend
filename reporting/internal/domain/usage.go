// =============================================================================
// Samvada Platform — Reporting Engine Billing & Metering Domain
// =============================================================================

package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type UsageSummary struct {
	TotalCalls            int64            `json:"total_calls"`
	TotalDurationSeconds  int64            `json:"total_duration_seconds"`
	TotalBillableSeconds  int64            `json:"total_billable_seconds"`
	TotalCostTelephonyUSD float64          `json:"total_cost_telephony_usd"`
	TotalCostSTTUSD       float64          `json:"total_cost_stt_usd"`
	TotalCostLLMUSD       float64          `json:"total_cost_llm_usd"`
	TotalCostTTSUSD       float64          `json:"total_cost_tts_usd"`
	TotalSpendUSD         float64          `json:"total_spend_usd"`
	PerWorkspaceMinutes   map[string]int64 `json:"per_workspace_minutes"`
}

type CurrentMonthUsage struct {
	MinutesUsed       int64   `json:"minutes_used"`
	MinutesLimit      int64   `json:"minutes_limit"`
	EstimatedSpendUSD float64 `json:"estimated_spend_usd"`
	PercentUsed       float64 `json:"percent_used"`
}

type UsageRepository interface {
	GetUsageSummary(ctx context.Context, orgID uuid.UUID, start, end time.Time) (*UsageSummary, error)
	GetCurrentMonthUsage(ctx context.Context, orgID, workspaceID uuid.UUID) (*CurrentMonthUsage, error)
}
