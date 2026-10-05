// =============================================================================
// Samvada Platform — ABS Subscription & Invoice Cron Worker (Phase 4.3)
// =============================================================================

package workers

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/samvada/abs/internal/application/billing"
	"github.com/samvada/abs/internal/domain"
)

type InvoiceCron struct {
	billingRepo    domain.BillingRepository
	invoiceService billing.InvoiceService
	subService     billing.SubscriptionService
	interval       time.Duration
	stopChan       chan struct{}
	log            *zap.Logger
}

func NewInvoiceCron(
	billingRepo domain.BillingRepository,
	invoiceService billing.InvoiceService,
	subService billing.SubscriptionService,
	interval time.Duration,
	log *zap.Logger,
) *InvoiceCron {
	if interval <= 0 {
		interval = 1 * time.Hour
	}
	return &InvoiceCron{
		billingRepo:    billingRepo,
		invoiceService: invoiceService,
		subService:     subService,
		interval:       interval,
		stopChan:       make(chan struct{}),
		log:            log.Named("invoice_cron"),
	}
}

func (c *InvoiceCron) Start(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-c.stopChan:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.runBillingCycle(ctx)
			}
		}
	}()
	c.log.Info("ABS Subscription Invoice Cron Worker started", zap.Duration("check_interval", c.interval))
}

func (c *InvoiceCron) Stop() {
	close(c.stopChan)
	c.log.Info("ABS Subscription Invoice Cron Worker stopped")
}

func (c *InvoiceCron) runBillingCycle(ctx context.Context) {
	c.log.Info("Running scheduled billing cycle check...")
	now := time.Now().UTC()

	subs, err := c.billingRepo.ListAllActiveSubscriptions(ctx)
	if err != nil {
		c.log.Error("Failed to fetch subscriptions for billing renewal", zap.Error(err))
		return
	}

	for _, sub := range subs {
		if sub.CurrentPeriodEnd.After(now) {
			continue
		}

		c.log.Info("Processing monthly subscription invoice renewal",
			zap.String("subscription_id", sub.ID.String()),
			zap.String("org_id", sub.OrgID.String()),
		)

		periodStart := sub.CurrentPeriodStart
		periodEnd := sub.CurrentPeriodEnd

		inv, err := c.invoiceService.GenerateMonthlyInvoice(ctx, sub.OrgID, periodStart, periodEnd)
		if err != nil {
			c.log.Error("Failed to generate monthly invoice for subscription",
				zap.String("sub_id", sub.ID.String()),
				zap.Error(err),
			)
			continue
		}

		if err := c.invoiceService.ChargeInvoice(ctx, inv.ID); err != nil {
			c.log.Warn("Invoice auto-charge failed during billing cron",
				zap.String("invoice_id", inv.ID.String()),
				zap.Error(err),
			)
			sub.Status = "past_due"
			_ = c.billingRepo.UpdateSubscription(ctx, sub)
		} else {
			nextPeriodEnd := now.AddDate(0, 1, 0)
			if sub.BillingCycle == "annual" {
				nextPeriodEnd = now.AddDate(1, 0, 0)
			}
			sub.CurrentPeriodStart = now
			sub.CurrentPeriodEnd = nextPeriodEnd
			sub.Status = "active"
			_ = c.billingRepo.UpdateSubscription(ctx, sub)
			c.log.Info("Successfully renewed subscription & paid invoice",
				zap.String("sub_id", sub.ID.String()),
				zap.String("invoice_number", inv.InvoiceNumber),
			)
		}
	}
}
