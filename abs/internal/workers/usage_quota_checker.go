// =============================================================================
// Samvada Platform — ABS Usage Quota Reconciliation Worker (Phase 4.3)
// =============================================================================

package workers

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/samvada/abs/internal/application/billing"
	"github.com/samvada/abs/internal/domain"
)

type UsageQuotaChecker struct {
	billingRepo     domain.BillingRepository
	meteringService billing.UsageMeteringService
	interval        time.Duration
	stopChan        chan struct{}
	log             *zap.Logger
}

func NewUsageQuotaChecker(
	billingRepo domain.BillingRepository,
	meteringService billing.UsageMeteringService,
	interval time.Duration,
	log *zap.Logger,
) *UsageQuotaChecker {
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	return &UsageQuotaChecker{
		billingRepo:     billingRepo,
		meteringService: meteringService,
		interval:        interval,
		stopChan:        make(chan struct{}),
		log:             log.Named("usage_quota_checker"),
	}
}

func (q *UsageQuotaChecker) Start(ctx context.Context) {
	ticker := time.NewTicker(q.interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-q.stopChan:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				q.reconcileQuotas(ctx)
			}
		}
	}()
	q.log.Info("ABS Usage Quota Reconciliation Worker started", zap.Duration("check_interval", q.interval))
}

func (q *UsageQuotaChecker) Stop() {
	close(q.stopChan)
	q.log.Info("ABS Usage Quota Reconciliation Worker stopped")
}

func (q *UsageQuotaChecker) reconcileQuotas(ctx context.Context) {
	q.log.Info("Checking workspace usage quotas against subscription tiers...")

	subs, err := q.billingRepo.ListAllActiveSubscriptions(ctx)
	if err != nil {
		q.log.Error("Failed to list active subscriptions for quota audit", zap.Error(err))
		return
	}

	for _, sub := range subs {
		summary, err := q.meteringService.GetQuotaUsage(ctx, sub.OrgID)
		if err != nil {
			continue
		}

		if summary.PercentUsed >= 100.0 {
			q.log.Warn("Organization has EXCEEDED included plan minutes!",
				zap.String("org_id", sub.OrgID.String()),
				zap.String("tier", summary.PlanTier),
				zap.Int("minutes_used", summary.UsedMinutes),
				zap.Int("included_minutes", summary.IncludedMinutes),
				zap.Float64("estimated_overage_usd", summary.EstimatedTotalCostUSD-summary.BasePlanPriceUSD),
			)
		} else if summary.PercentUsed >= 90.0 {
			q.log.Info("Organization has reached 90% minute quota threshold alert",
				zap.String("org_id", sub.OrgID.String()),
				zap.Float64("percent", summary.PercentUsed),
			)
		}
	}
}
