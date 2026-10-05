// =============================================================================
// Samvada Platform — ABS Usage Metering & Quota Application Service (Phase 4.2)
// =============================================================================
// Package: billing
//
// Reconciles telemetry from Reporting Engine ClickHouse with PostgreSQL subscription
// quotas to compute:
//   - Current month minutes used vs. quota limit
//   - Billable seconds breakdown
//   - Real-time overage cost calculations
// =============================================================================

package billing

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/domain"
	"github.com/samvada/abs/internal/grpc/client"
)

type QuotaUsageSummary struct {
	OrgID                 uuid.UUID `json:"org_id"`
	PlanTier              string    `json:"plan_tier"`
	PlanName              string    `json:"plan_name"`
	IncludedMinutes       int       `json:"included_minutes"`
	UsedMinutes           int       `json:"used_minutes"`
	RemainingMinutes      int       `json:"remaining_minutes"`
	PercentUsed           float64   `json:"percent_used"`
	OverageMinutes        int       `json:"overage_minutes"`
	OverageAmountUSD      float64   `json:"overage_amount_usd"`
	BasePlanPriceUSD      float64   `json:"base_plan_price_usd"`
	EstimatedTotalCostUSD float64   `json:"estimated_total_cost_usd"`
	WalletBalanceUSD      float64   `json:"wallet_balance_usd"`
	BillingCycle          string    `json:"billing_cycle"`
	PeriodStart           time.Time `json:"period_start"`
	PeriodEnd             time.Time `json:"period_end"`
}

type UsageMeteringService interface {
	GetQuotaUsage(ctx context.Context, orgID uuid.UUID) (*QuotaUsageSummary, error)
}

type usageMeteringService struct {
	billingRepo     domain.BillingRepository
	reportingClient client.ReportingClient
	log             *zap.Logger
}

func NewUsageMeteringService(
	billingRepo domain.BillingRepository,
	reportingClient client.ReportingClient,
	log *zap.Logger,
) UsageMeteringService {
	return &usageMeteringService{
		billingRepo:     billingRepo,
		reportingClient: reportingClient,
		log:             log.Named("usage_metering_service"),
	}
}

func (s *usageMeteringService) GetQuotaUsage(ctx context.Context, orgID uuid.UUID) (*QuotaUsageSummary, error) {
	// 1. Fetch Subscription & Plan
	sub, err := s.billingRepo.GetSubscriptionByOrgID(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("retrieving subscription: %w", err)
	}

	plan := sub.Plan
	if plan == nil {
		plan, _ = s.billingRepo.GetPlanByID(ctx, sub.PlanID)
	}
	if plan == nil {
		plan, _ = s.billingRepo.GetPlanByTier(ctx, "free")
	}

	// 2. Fetch Organization Wallet
	wallet, _ := s.billingRepo.GetWalletByOrgID(ctx, orgID)
	walletBalance := 0.0
	if wallet != nil {
		walletBalance = wallet.BalanceUSD
	}

	// 3. Query billable usage from Reporting Engine
	usedSeconds := int64(0)
	now := time.Now().UTC()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	endOfMonth := startOfMonth.AddDate(0, 1, 0).Add(-time.Nanosecond)

	if s.reportingClient != nil {
		startStr := startOfMonth.Format(time.RFC3339)
		endStr := endOfMonth.Format(time.RFC3339)

		resp, err := s.reportingClient.GetUsageSummary(ctx, orgID.String(), startStr, endStr)
		if err == nil && resp != nil {
			usedSeconds = int64(resp.TotalBillableSeconds)
		} else {
			s.log.Debug("Reporting Client call fallback to zero usage", zap.Error(err))
		}
	}

	usedMinutes := int(math.Ceil(float64(usedSeconds) / 60.0))
	includedMinutes := plan.IncludedMinutes
	if includedMinutes <= 0 {
		includedMinutes = 83 // Default starter/free minutes
	}

	remainingMinutes := includedMinutes - usedMinutes
	if remainingMinutes < 0 {
		remainingMinutes = 0
	}

	overageMinutes := 0
	if usedMinutes > includedMinutes {
		overageMinutes = usedMinutes - includedMinutes
	}

	overageCost := float64(overageMinutes) * plan.OveragePerMinuteUSD
	percentUsed := 0.0
	if includedMinutes > 0 {
		percentUsed = math.Min(100.0, (float64(usedMinutes)/float64(includedMinutes))*100.0)
	}

	periodStart := sub.CurrentPeriodStart
	if periodStart.IsZero() {
		periodStart = startOfMonth
	}
	periodEnd := sub.CurrentPeriodEnd
	if periodEnd.IsZero() {
		periodEnd = endOfMonth
	}

	return &QuotaUsageSummary{
		OrgID:                 orgID,
		PlanTier:              plan.Tier,
		PlanName:              plan.Name,
		IncludedMinutes:       includedMinutes,
		UsedMinutes:           usedMinutes,
		RemainingMinutes:      remainingMinutes,
		PercentUsed:           math.Round(percentUsed*100) / 100,
		OverageMinutes:        overageMinutes,
		OverageAmountUSD:      math.Round(overageCost*100) / 100,
		BasePlanPriceUSD:      plan.MonthlyPriceUSD,
		EstimatedTotalCostUSD: math.Round((plan.MonthlyPriceUSD+overageCost)*100) / 100,
		WalletBalanceUSD:      walletBalance,
		BillingCycle:          sub.BillingCycle,
		PeriodStart:           periodStart,
		PeriodEnd:             periodEnd,
	}, nil
}
