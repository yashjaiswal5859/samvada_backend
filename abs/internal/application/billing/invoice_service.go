// =============================================================================
// Samvada Platform — ABS Invoice Generation & Payment Collection Service (Phase 4.2)
// =============================================================================
// Package: billing
//
// Responsibilities:
//   1. End-of-period reconciliation of subscription base price + overages
//   2. Generation of compliant enterprise invoices with unique numbering
//   3. Automated charge execution against saved default payment methods
//   4. Monthly cron cycle runner for automated organization billing
// =============================================================================

package billing

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/domain"
	"github.com/samvada/abs/internal/grpc/client"
	"github.com/samvada/abs/internal/infrastructure/payment"
	"github.com/samvada/abs/internal/infrastructure/storage"
)

type InvoiceService interface {
	GenerateMonthlyInvoice(ctx context.Context, orgID uuid.UUID, periodStart, periodEnd time.Time) (*domain.Invoice, error)
	ListInvoices(ctx context.Context, orgID uuid.UUID) ([]*domain.Invoice, error)
	GetInvoice(ctx context.Context, orgID uuid.UUID, invoiceID uuid.UUID) (*domain.Invoice, error)
	ChargeInvoice(ctx context.Context, invoiceID uuid.UUID) error
	RunMonthlyBillingCycle(ctx context.Context) (int, error)
}

type invoiceService struct {
	billingRepo     domain.BillingRepository
	orgRepo         domain.OrgRepository
	reportingClient client.ReportingClient
	gateways        map[string]payment.PaymentGateway
	storage         storage.Storage
	log             *zap.Logger
}

func NewInvoiceService(
	billingRepo domain.BillingRepository,
	orgRepo domain.OrgRepository,
	reportingClient client.ReportingClient,
	gateways map[string]payment.PaymentGateway,
	storage storage.Storage,
	log *zap.Logger,
) InvoiceService {
	return &invoiceService{
		billingRepo:     billingRepo,
		orgRepo:         orgRepo,
		reportingClient: reportingClient,
		gateways:        gateways,
		storage:         storage,
		log:             log.Named("invoice_service"),
	}
}

func (s *invoiceService) getGateway(name string) (payment.PaymentGateway, error) {
	if name == "" {
		name = "stripe"
	}
	gw, ok := s.gateways[name]
	if !ok || gw == nil {
		return nil, fmt.Errorf("payment gateway %q not configured", name)
	}
	return gw, nil
}

func (s *invoiceService) GenerateMonthlyInvoice(
	ctx context.Context,
	orgID uuid.UUID,
	periodStart, periodEnd time.Time,
) (*domain.Invoice, error) {
	// 1. Fetch Subscription & Plan
	sub, err := s.billingRepo.GetSubscriptionByOrgID(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("subscription lookup failed: %w", err)
	}

	plan := sub.Plan
	if plan == nil {
		plan, _ = s.billingRepo.GetPlanByID(ctx, sub.PlanID)
	}
	if plan == nil {
		plan, _ = s.billingRepo.GetPlanByTier(ctx, "free")
	}

	// 2. Base Amount
	baseAmount := plan.MonthlyPriceUSD
	if sub.BillingCycle == "annual" {
		baseAmount = plan.MonthlyPriceUSD * 10.0 // Annual 2-months discount
	}

	// 3. Query Usage from Reporting Engine
	totalBillableSeconds := int64(0)
	if s.reportingClient != nil {
		startStr := periodStart.Format(time.RFC3339)
		endStr := periodEnd.Format(time.RFC3339)
		resp, err := s.reportingClient.GetUsageSummary(ctx, orgID.String(), startStr, endStr)
		if err == nil && resp != nil {
			totalBillableSeconds = int64(resp.TotalBillableSeconds)
		}
	}

	totalMinutesUsed := int(math.Ceil(float64(totalBillableSeconds) / 60.0))
	overageMinutes := 0
	if totalMinutesUsed > plan.IncludedMinutes {
		overageMinutes = totalMinutesUsed - plan.IncludedMinutes
	}
	overageAmount := math.Round(float64(overageMinutes)*plan.OveragePerMinuteUSD*100) / 100

	// 4. Tax (e.g. 18% standard business services GST or 0 for export)
	taxRate := 0.18
	taxAmount := math.Round((baseAmount+overageAmount)*taxRate*100) / 100
	totalAmount := math.Round((baseAmount+overageAmount+taxAmount)*100) / 100

	// 5. Unique Invoice Number: INV-YYYYMM-<org8>-<seq>
	seq := time.Now().Unix() % 10000
	invNumber := fmt.Sprintf("INV-%s-%s-%04d", periodStart.Format("200601"), orgID.String()[:8], seq)

	invID := uuid.New()
	invoice := &domain.Invoice{
		ID:             invID,
		InvoiceNumber:  invNumber,
		OrgID:          orgID,
		SubscriptionID: &sub.ID,
		PeriodStart:    periodStart,
		PeriodEnd:      periodEnd,
		BaseAmount:     baseAmount,
		OverageAmount:  overageAmount,
		TaxAmount:      taxAmount,
		TotalAmount:    totalAmount,
		Currency:       "USD",
		Status:         "open",
		CreatedAt:      time.Now().UTC(),
	}

	// If total amount is zero, mark as paid immediately
	if totalAmount <= 0.001 {
		now := time.Now().UTC()
		invoice.Status = "paid"
		invoice.PaidAt = &now
	}

	// 6. Attempt payment collection if payment method available
	if invoice.Status == "open" && sub.GatewayCustomerID != "" {
		pm, _ := s.billingRepo.GetDefaultPaymentMethod(ctx, orgID)
		if pm != nil {
			gw, err := s.getGateway(sub.PaymentGateway)
			if err == nil {
				chargeID, chargeErr := gw.ChargeInvoice(ctx, payment.ChargeParams{
					CustomerID:      sub.GatewayCustomerID,
					PaymentMethodID: pm.GatewayPaymentMethod,
					Amount:          totalAmount,
					Currency:        invoice.Currency,
					Description:     fmt.Sprintf("Samvada Voice AI Invoice %s", invNumber),
					InvoiceID:       invNumber,
				})
				if chargeErr == nil && chargeID != "" {
					now := time.Now().UTC()
					invoice.Status = "paid"
					invoice.GatewayInvoice = &chargeID
					invoice.PaidAt = &now
				} else {
					s.log.Warn("Automated invoice charge failed, left as open for retry",
						zap.String("invoice_number", invNumber),
						zap.Error(chargeErr),
					)
				}
			}
		}
	}

	// 7. Generate PDF/Document asset in storage if storage available
	if s.storage != nil {
		pdfContent := fmt.Sprintf(`SAMVADA PLATFORM INVOICE
=========================================
Invoice Number : %s
Organization ID: %s
Billing Period : %s to %s
Plan           : %s (%s)
Base Amount    : $%.2f
Overages (%d m): $%.2f
Tax (18%%)      : $%.2f
-----------------------------------------
TOTAL AMOUNT   : $%.2f %s
Status         : %s
=========================================
Thank you for building with Samvada AI.
`, invNumber, orgID, periodStart.Format("2006-01-02"), periodEnd.Format("2006-01-02"),
			plan.Name, sub.BillingCycle, baseAmount, overageMinutes, overageAmount, taxAmount, totalAmount, invoice.Currency, strings.ToUpper(invoice.Status))

		storagePath := fmt.Sprintf("invoices/%s/%s.txt", orgID.String(), invNumber)
		fileURL, err := s.storage.Upload(ctx, storagePath, []byte(pdfContent), "text/plain")
		if err == nil {
			invoice.PDFURL = &fileURL
		}
	}

	// 8. Persist Invoice
	if err := s.billingRepo.CreateInvoice(ctx, invoice); err != nil {
		return nil, fmt.Errorf("persisting invoice: %w", err)
	}

	return invoice, nil
}

func (s *invoiceService) ListInvoices(ctx context.Context, orgID uuid.UUID) ([]*domain.Invoice, error) {
	return s.billingRepo.ListInvoices(ctx, orgID)
}

func (s *invoiceService) GetInvoice(ctx context.Context, orgID uuid.UUID, invoiceID uuid.UUID) (*domain.Invoice, error) {
	inv, err := s.billingRepo.GetInvoiceByID(ctx, invoiceID)
	if err != nil {
		return nil, err
	}
	if inv.OrgID != orgID {
		return nil, domain.ErrInvoiceNotFound
	}
	return inv, nil
}

func (s *invoiceService) ChargeInvoice(ctx context.Context, invoiceID uuid.UUID) error {
	inv, err := s.billingRepo.GetInvoiceByID(ctx, invoiceID)
	if err != nil {
		return err
	}

	if inv.Status == "paid" {
		return nil
	}

	sub, err := s.billingRepo.GetSubscriptionByOrgID(ctx, inv.OrgID)
	if err != nil {
		return fmt.Errorf("subscription lookup failed: %w", err)
	}

	pm, err := s.billingRepo.GetDefaultPaymentMethod(ctx, inv.OrgID)
	if err != nil || pm == nil {
		return errors.New("no default payment method found for organization")
	}

	gw, err := s.getGateway(sub.PaymentGateway)
	if err != nil {
		return err
	}

	chargeID, err := gw.ChargeInvoice(ctx, payment.ChargeParams{
		CustomerID:      sub.GatewayCustomerID,
		PaymentMethodID: pm.GatewayPaymentMethod,
		Amount:          inv.TotalAmount,
		Currency:        inv.Currency,
		Description:     fmt.Sprintf("Retry charge for Invoice %s", inv.InvoiceNumber),
		InvoiceID:       inv.InvoiceNumber,
	})
	if err != nil {
		return fmt.Errorf("gateway charge failed: %w", err)
	}

	now := time.Now().UTC()
	inv.Status = "paid"
	inv.GatewayInvoice = &chargeID
	inv.PaidAt = &now

	return s.billingRepo.UpdateInvoice(ctx, inv)
}

func (s *invoiceService) RunMonthlyBillingCycle(ctx context.Context) (int, error) {
	subs, err := s.billingRepo.ListAllActiveSubscriptions(ctx)
	if err != nil {
		return 0, err
	}

	now := time.Now().UTC()
	count := 0

	for _, sub := range subs {
		// If subscription period has concluded, generate invoice and advance cycle
		if now.After(sub.CurrentPeriodEnd) {
			_, err := s.GenerateMonthlyInvoice(ctx, sub.OrgID, sub.CurrentPeriodStart, sub.CurrentPeriodEnd)
			if err != nil {
				s.log.Error("Failed to generate monthly invoice during billing cycle",
					zap.String("org_id", sub.OrgID.String()),
					zap.Error(err),
				)
				continue
			}

			// Advance period
			sub.CurrentPeriodStart = sub.CurrentPeriodEnd
			if sub.BillingCycle == "annual" {
				sub.CurrentPeriodEnd = sub.CurrentPeriodStart.AddDate(1, 0, 0)
			} else {
				sub.CurrentPeriodEnd = sub.CurrentPeriodStart.AddDate(0, 1, 0)
			}
			sub.UpdatedAt = now
			_ = s.billingRepo.UpdateSubscription(ctx, sub)

			count++
		}
	}

	return count, nil
}
