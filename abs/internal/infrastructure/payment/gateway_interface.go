// =============================================================================
// Samvada Platform — ABS Payment Gateway Strategy Interface (Phase 4.1)
// =============================================================================
// Package: payment
//
// Defines unified abstraction across payment gateways (Stripe, Razorpay) for:
//   1. Customer lifecycle (creation, metadata sync)
//   2. Subscriptions (creation, plan upgrade/downgrade, cancellation)
//   3. Payment methods (card tokenization, e-mandate, UPI AutoPay)
//   4. Invoice charging & webhook signature validation
// =============================================================================

package payment

import (
	"context"

	"github.com/google/uuid"
	"github.com/samvada/abs/internal/domain"
)

type CustomerParams struct {
	OrgID uuid.UUID
	Name  string
	Email string
	Phone string
}

type SubscriptionParams struct {
	CustomerID   string
	PlanTier     string
	PriceID      string
	Currency     string
	BillingCycle string // monthly, annual
}

type SubscriptionResult struct {
	GatewaySubscriptionID string
	Status                string
	CurrentPeriodStart    int64
	CurrentPeriodEnd      int64
}

type PaymentMethodParams struct {
	CustomerID string
	Token      string // Stripe pm_xxx or Razorpay token_xxx
}

type ChargeParams struct {
	CustomerID      string
	PaymentMethodID string
	Amount          float64
	Currency        string
	Description     string
	InvoiceID       string
}

type WebhookEvent struct {
	ID        string
	Type      string // e.g. "invoice.paid", "customer.subscription.updated", "payment.failed"
	Data      map[string]interface{}
	Timestamp int64
}

// PaymentGateway defines the strategy contract implemented by Stripe and Razorpay.
type PaymentGateway interface {
	GatewayName() string

	// Customer Management
	CreateCustomer(ctx context.Context, params CustomerParams) (string, error)

	// Subscription Lifecycle
	CreateSubscription(ctx context.Context, params SubscriptionParams) (*SubscriptionResult, error)
	CancelSubscription(ctx context.Context, gatewaySubID string) error

	// Payment Methods
	AttachPaymentMethod(ctx context.Context, orgID uuid.UUID, params PaymentMethodParams) (*domain.PaymentMethod, error)

	// Invoicing & Charging
	ChargeInvoice(ctx context.Context, params ChargeParams) (string, error)

	// Webhook Verification
	VerifyWebhook(payload []byte, signature string) (*WebhookEvent, error)
}
