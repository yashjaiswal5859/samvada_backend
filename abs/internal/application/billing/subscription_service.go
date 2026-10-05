// =============================================================================
// Samvada Platform — ABS Subscription & Gateway Application Service (Phase 4.2)
// =============================================================================
// Package: billing
//
// Manages recurring subscriptions across Stripe and Razorpay:
//   - Plan listing & tier resolution
//   - Gateway customer provisioning & subscription lifecycle
//   - Payment method tokenization and defaults
//   - Webhook processing & dunning state machine
// =============================================================================

package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/domain"
	"github.com/samvada/abs/internal/infrastructure/payment"
)

type SubscribeRequest struct {
	PlanID       *uuid.UUID `json:"plan_id,omitempty"`
	PlanTier     string     `json:"plan_tier"`     // starter, pro, enterprise
	Gateway      string     `json:"gateway"`       // stripe, razorpay
	BillingCycle string     `json:"billing_cycle"` // monthly, annual
	PaymentToken *string    `json:"payment_token,omitempty"`
}

type SubscriptionService interface {
	ListPlans(ctx context.Context) ([]*domain.Plan, error)
	GetSubscription(ctx context.Context, orgID uuid.UUID) (*domain.Subscription, error)
	Subscribe(ctx context.Context, orgID uuid.UUID, req SubscribeRequest) (*domain.Subscription, error)
	CancelSubscription(ctx context.Context, orgID uuid.UUID, immediately bool) error

	AddPaymentMethod(ctx context.Context, orgID uuid.UUID, gateway, token string, makeDefault bool) (*domain.PaymentMethod, error)
	ListPaymentMethods(ctx context.Context, orgID uuid.UUID) ([]*domain.PaymentMethod, error)
	GetDefaultPaymentMethod(ctx context.Context, orgID uuid.UUID) (*domain.PaymentMethod, error)
	SetDefaultPaymentMethod(ctx context.Context, orgID uuid.UUID, pmID uuid.UUID) error
	DeletePaymentMethod(ctx context.Context, orgID uuid.UUID, pmID uuid.UUID) error

	HandleWebhook(ctx context.Context, gatewayName string, payload []byte, signature string) (*payment.WebhookEvent, error)
}

type subscriptionService struct {
	billingRepo domain.BillingRepository
	orgRepo     domain.OrgRepository
	gateways    map[string]payment.PaymentGateway
	log         *zap.Logger
}

func NewSubscriptionService(
	billingRepo domain.BillingRepository,
	orgRepo domain.OrgRepository,
	gateways map[string]payment.PaymentGateway,
	log *zap.Logger,
) SubscriptionService {
	return &subscriptionService{
		billingRepo: billingRepo,
		orgRepo:     orgRepo,
		gateways:    gateways,
		log:         log.Named("subscription_service"),
	}
}

func (s *subscriptionService) getGateway(name string) (payment.PaymentGateway, error) {
	if name == "" {
		name = "stripe"
	}
	gw, ok := s.gateways[name]
	if !ok || gw == nil {
		return nil, fmt.Errorf("payment gateway %q not configured", name)
	}
	return gw, nil
}

func (s *subscriptionService) ListPlans(ctx context.Context) ([]*domain.Plan, error) {
	return s.billingRepo.ListActivePlans(ctx)
}

func (s *subscriptionService) GetSubscription(ctx context.Context, orgID uuid.UUID) (*domain.Subscription, error) {
	return s.billingRepo.GetSubscriptionByOrgID(ctx, orgID)
}

func (s *subscriptionService) Subscribe(ctx context.Context, orgID uuid.UUID, req SubscribeRequest) (*domain.Subscription, error) {
	// 1. Resolve Plan
	var plan *domain.Plan
	var err error
	if req.PlanID != nil && *req.PlanID != uuid.Nil {
		plan, err = s.billingRepo.GetPlanByID(ctx, *req.PlanID)
	} else if req.PlanTier != "" {
		plan, err = s.billingRepo.GetPlanByTier(ctx, req.PlanTier)
	} else {
		return nil, errors.New("must specify plan_id or plan_tier")
	}
	if err != nil {
		return nil, fmt.Errorf("plan lookup failed: %w", err)
	}

	gatewayName := req.Gateway
	if gatewayName == "" {
		gatewayName = "stripe"
	}
	gw, err := s.getGateway(gatewayName)
	if err != nil {
		return nil, err
	}

	// 2. Fetch Organization Details
	org, err := s.orgRepo.GetByID(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("organization lookup failed: %w", err)
	}

	// 3. Existing Subscription check
	existingSub, _ := s.billingRepo.GetSubscriptionByOrgID(ctx, orgID)
	customerID := ""
	if existingSub != nil {
		customerID = existingSub.GatewayCustomerID
	}

	// 4. Create Gateway Customer if none exists
	if customerID == "" {
		customerID, err = gw.CreateCustomer(ctx, payment.CustomerParams{
			OrgID: orgID,
			Name:  org.Name,
			Email: org.BillingEmail,
		})
		if err != nil {
			return nil, fmt.Errorf("creating gateway customer: %w", err)
		}
	}

	// 5. Attach token if provided
	if req.PaymentToken != nil && *req.PaymentToken != "" {
		_, _ = s.AddPaymentMethod(ctx, orgID, gatewayName, *req.PaymentToken, true)
	}

	// 6. Create Gateway Subscription
	cycle := req.BillingCycle
	if cycle == "" {
		cycle = "monthly"
	}
	subRes, err := gw.CreateSubscription(ctx, payment.SubscriptionParams{
		CustomerID:   customerID,
		PlanTier:     plan.Tier,
		PriceID:      plan.Tier,
		Currency:     org.BillingCurrency,
		BillingCycle: cycle,
	})
	if err != nil {
		return nil, fmt.Errorf("creating gateway subscription: %w", err)
	}

	now := time.Now().UTC()
	periodStart := time.Unix(subRes.CurrentPeriodStart, 0).UTC()
	periodEnd := time.Unix(subRes.CurrentPeriodEnd, 0).UTC()
	if periodStart.IsZero() {
		periodStart = now
	}
	if periodEnd.IsZero() {
		periodEnd = now.AddDate(0, 1, 0)
	}

	// 7. Persist or Update Subscription record
	if existingSub == nil {
		newSub := &domain.Subscription{
			ID:                    uuid.New(),
			OrgID:                 orgID,
			PlanID:                plan.ID,
			Status:                subRes.Status,
			BillingCycle:          cycle,
			PaymentGateway:        gatewayName,
			GatewaySubscriptionID: &subRes.GatewaySubscriptionID,
			GatewayCustomerID:     customerID,
			CurrentPeriodStart:    periodStart,
			CurrentPeriodEnd:      periodEnd,
			CancelAtPeriodEnd:     false,
			CreatedAt:             now,
			UpdatedAt:             now,
		}
		if err := s.billingRepo.CreateSubscription(ctx, newSub); err != nil {
			return nil, err
		}
		newSub.Plan = plan
		return newSub, nil
	}

	existingSub.PlanID = plan.ID
	existingSub.Status = subRes.Status
	existingSub.BillingCycle = cycle
	existingSub.PaymentGateway = gatewayName
	existingSub.GatewaySubscriptionID = &subRes.GatewaySubscriptionID
	existingSub.GatewayCustomerID = customerID
	existingSub.CurrentPeriodStart = periodStart
	existingSub.CurrentPeriodEnd = periodEnd
	existingSub.CancelAtPeriodEnd = false
	existingSub.UpdatedAt = now

	if err := s.billingRepo.UpdateSubscription(ctx, existingSub); err != nil {
		return nil, err
	}
	existingSub.Plan = plan
	return existingSub, nil
}

func (s *subscriptionService) CancelSubscription(ctx context.Context, orgID uuid.UUID, immediately bool) error {
	sub, err := s.billingRepo.GetSubscriptionByOrgID(ctx, orgID)
	if err != nil {
		return err
	}

	if immediately {
		if sub.GatewaySubscriptionID != nil && *sub.GatewaySubscriptionID != "" {
			if gw, err := s.getGateway(sub.PaymentGateway); err == nil {
				_ = gw.CancelSubscription(ctx, *sub.GatewaySubscriptionID)
			}
		}
		sub.Status = "canceled"
		sub.CancelAtPeriodEnd = false
	} else {
		sub.CancelAtPeriodEnd = true
	}
	sub.UpdatedAt = time.Now().UTC()

	return s.billingRepo.UpdateSubscription(ctx, sub)
}

func (s *subscriptionService) AddPaymentMethod(ctx context.Context, orgID uuid.UUID, gateway, token string, makeDefault bool) (*domain.PaymentMethod, error) {
	if gateway == "" {
		gateway = "stripe"
	}
	gw, err := s.getGateway(gateway)
	if err != nil {
		return nil, err
	}

	// Fetch or create customer ID
	sub, _ := s.billingRepo.GetSubscriptionByOrgID(ctx, orgID)
	customerID := ""
	if sub != nil {
		customerID = sub.GatewayCustomerID
	}

	pm, err := gw.AttachPaymentMethod(ctx, orgID, payment.PaymentMethodParams{
		CustomerID: customerID,
		Token:      token,
	})
	if err != nil {
		return nil, fmt.Errorf("attaching payment method on gateway: %w", err)
	}

	pm.IsDefault = makeDefault
	if err := s.billingRepo.AddPaymentMethod(ctx, pm); err != nil {
		return nil, fmt.Errorf("saving payment method: %w", err)
	}

	return pm, nil
}

func (s *subscriptionService) ListPaymentMethods(ctx context.Context, orgID uuid.UUID) ([]*domain.PaymentMethod, error) {
	return s.billingRepo.ListPaymentMethods(ctx, orgID)
}

func (s *subscriptionService) GetDefaultPaymentMethod(ctx context.Context, orgID uuid.UUID) (*domain.PaymentMethod, error) {
	return s.billingRepo.GetDefaultPaymentMethod(ctx, orgID)
}

func (s *subscriptionService) SetDefaultPaymentMethod(ctx context.Context, orgID uuid.UUID, pmID uuid.UUID) error {
	return s.billingRepo.SetDefaultPaymentMethod(ctx, orgID, pmID)
}

func (s *subscriptionService) DeletePaymentMethod(ctx context.Context, orgID uuid.UUID, pmID uuid.UUID) error {
	return s.billingRepo.DeletePaymentMethod(ctx, orgID, pmID)
}

func (s *subscriptionService) HandleWebhook(ctx context.Context, gatewayName string, payload []byte, signature string) (*payment.WebhookEvent, error) {
	gw, err := s.getGateway(gatewayName)
	if err != nil {
		return nil, err
	}

	evt, err := gw.VerifyWebhook(payload, signature)
	if err != nil {
		s.log.Warn("Webhook verification failed",
			zap.String("gateway", gatewayName),
			zap.Error(err),
		)
		return nil, err
	}

	s.log.Info("Processed payment gateway webhook",
		zap.String("gateway", gatewayName),
		zap.String("event_type", evt.Type),
		zap.String("event_id", evt.ID),
	)

	return evt, nil
}
