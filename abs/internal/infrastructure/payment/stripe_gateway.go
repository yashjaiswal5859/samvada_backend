// =============================================================================
// Samvada Platform — Stripe Payment Gateway Adapter (Phase 4.1)
// =============================================================================
// Package: payment
//
// Implements PaymentGateway for Stripe. Supports:
//   - Customer creation with metadata
//   - Recurring subscription management
//   - Payment method attachment (cards / SEPA / ACH)
//   - In-flight charges via PaymentIntents
//   - Webhook signature validation (Stripe-Signature v1 HMAC-SHA256)
//   - Graceful sandbox mock mode for offline dev/testing environments
// =============================================================================

package payment

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/domain"
)

type StripeGateway struct {
	secretKey     string
	webhookSecret string
	httpClient    *http.Client
	log           *zap.Logger
}

func NewStripeGateway(secretKey, webhookSecret string, log *zap.Logger) *StripeGateway {
	return &StripeGateway{
		secretKey:     secretKey,
		webhookSecret: webhookSecret,
		httpClient:    &http.Client{Timeout: 15 * time.Second},
		log:           log.Named("stripe_gateway"),
	}
}

func (g *StripeGateway) GatewayName() string {
	return "stripe"
}

func (g *StripeGateway) isSandbox() bool {
	return g.secretKey == "" || strings.HasPrefix(g.secretKey, "mock_") || strings.HasPrefix(g.secretKey, "test_")
}

func (g *StripeGateway) CreateCustomer(ctx context.Context, params CustomerParams) (string, error) {
	if g.isSandbox() {
		customerID := fmt.Sprintf("cus_stripe_mock_%s", params.OrgID.String()[:8])
		g.log.Info("Created Stripe mock customer",
			zap.String("customer_id", customerID),
			zap.String("org_id", params.OrgID.String()),
		)
		return customerID, nil
	}

	data := url.Values{}
	data.Set("name", params.Name)
	data.Set("email", params.Email)
	if params.Phone != "" {
		data.Set("phone", params.Phone)
	}
	data.Set("metadata[org_id]", params.OrgID.String())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.stripe.com/v1/customers", strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(g.secretKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("stripe customer creation failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("stripe api error (%d): %s", resp.StatusCode, string(bodyBytes))
	}

	var res struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(bodyBytes, &res); err != nil {
		return "", err
	}

	return res.ID, nil
}

func (g *StripeGateway) CreateSubscription(ctx context.Context, params SubscriptionParams) (*SubscriptionResult, error) {
	now := time.Now().UTC()
	periodEnd := now.AddDate(0, 1, 0)
	if params.BillingCycle == "annual" {
		periodEnd = now.AddDate(1, 0, 0)
	}

	if g.isSandbox() {
		subID := fmt.Sprintf("sub_stripe_mock_%d", time.Now().UnixNano()%1000000)
		g.log.Info("Created Stripe mock subscription",
			zap.String("sub_id", subID),
			zap.String("customer_id", params.CustomerID),
			zap.String("tier", params.PlanTier),
		)
		return &SubscriptionResult{
			GatewaySubscriptionID: subID,
			Status:                "active",
			CurrentPeriodStart:    now.Unix(),
			CurrentPeriodEnd:      periodEnd.Unix(),
		}, nil
	}

	data := url.Values{}
	data.Set("customer", params.CustomerID)
	data.Set("items[0][price]", params.PriceID)
	data.Set("payment_behavior", "default_incomplete")
	data.Set("expand[0]", "latest_invoice.payment_intent")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.stripe.com/v1/subscriptions", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(g.secretKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stripe subscription call failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("stripe api error (%d): %s", resp.StatusCode, string(bodyBytes))
	}

	var res struct {
		ID                 string `json:"id"`
		Status             string `json:"status"`
		CurrentPeriodStart int64  `json:"current_period_start"`
		CurrentPeriodEnd   int64  `json:"current_period_end"`
	}
	if err := json.Unmarshal(bodyBytes, &res); err != nil {
		return nil, err
	}

	return &SubscriptionResult{
		GatewaySubscriptionID: res.ID,
		Status:                res.Status,
		CurrentPeriodStart:    res.CurrentPeriodStart,
		CurrentPeriodEnd:      res.CurrentPeriodEnd,
	}, nil
}

func (g *StripeGateway) CancelSubscription(ctx context.Context, gatewaySubID string) error {
	if g.isSandbox() {
		g.log.Info("Cancelled Stripe mock subscription", zap.String("sub_id", gatewaySubID))
		return nil
	}

	endpoint := fmt.Sprintf("https://api.stripe.com/v1/subscriptions/%s", gatewaySubID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(g.secretKey, "")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("stripe subscription delete failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("stripe delete error (%d): %s", resp.StatusCode, string(body))
	}

	return nil
}

func (g *StripeGateway) AttachPaymentMethod(ctx context.Context, orgID uuid.UUID, params PaymentMethodParams) (*domain.PaymentMethod, error) {
	brand := "visa"
	last4 := "4242"
	expMonth := 12
	expYear := 2029

	pmID := params.Token
	if pmID == "" {
		pmID = fmt.Sprintf("pm_stripe_mock_%d", time.Now().UnixNano()%1000000)
	}

	if !g.isSandbox() && strings.HasPrefix(pmID, "pm_") {
		// Attach PM to customer in Stripe
		data := url.Values{}
		data.Set("customer", params.CustomerID)

		endpoint := fmt.Sprintf("https://api.stripe.com/v1/payment_methods/%s/attach", pmID)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(data.Encode()))
		if err != nil {
			return nil, err
		}
		req.SetBasicAuth(g.secretKey, "")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := g.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("attaching stripe payment method: %w", err)
		}
		defer resp.Body.Close()

		bodyBytes, _ := io.ReadAll(resp.Body)
		if resp.StatusCode < 400 {
			var pmResp struct {
				Card struct {
					Brand    string `json:"brand"`
					Last4    string `json:"last4"`
					ExpMonth int    `json:"exp_month"`
					ExpYear  int    `json:"exp_year"`
				} `json:"card"`
			}
			if err := json.Unmarshal(bodyBytes, &pmResp); err == nil && pmResp.Card.Last4 != "" {
				brand = pmResp.Card.Brand
				last4 = pmResp.Card.Last4
				expMonth = pmResp.Card.ExpMonth
				expYear = pmResp.Card.ExpYear
			}
		}
	}

	return &domain.PaymentMethod{
		ID:                   uuid.New(),
		OrgID:                orgID,
		Gateway:              "stripe",
		GatewayPaymentMethod: pmID,
		Brand:                &brand,
		Last4:                &last4,
		ExpMonth:             &expMonth,
		ExpYear:              &expYear,
		IsDefault:            true,
		CreatedAt:            time.Now().UTC(),
	}, nil
}

func (g *StripeGateway) ChargeInvoice(ctx context.Context, params ChargeParams) (string, error) {
	if g.isSandbox() {
		chargeID := fmt.Sprintf("ch_stripe_mock_%d", time.Now().UnixNano()%1000000)
		g.log.Info("Stripe mock charge successful",
			zap.String("charge_id", chargeID),
			zap.Float64("amount", params.Amount),
			zap.String("currency", params.Currency),
		)
		return chargeID, nil
	}

	// Stripe amounts are in cents
	amountCents := int64(params.Amount * 100)
	data := url.Values{}
	data.Set("amount", strconv.FormatInt(amountCents, 10))
	data.Set("currency", strings.ToLower(params.Currency))
	data.Set("customer", params.CustomerID)
	data.Set("payment_method", params.PaymentMethodID)
	data.Set("off_session", "true")
	data.Set("confirm", "true")
	data.Set("description", params.Description)
	data.Set("metadata[invoice_id]", params.InvoiceID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.stripe.com/v1/payment_intents", strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(g.secretKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("payment intent creation failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("stripe charge failed (%d): %s", resp.StatusCode, string(bodyBytes))
	}

	var res struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(bodyBytes, &res); err != nil {
		return "", err
	}

	return res.ID, nil
}

func (g *StripeGateway) VerifyWebhook(payload []byte, signature string) (*WebhookEvent, error) {
	if g.webhookSecret == "" || g.isSandbox() {
		// Mock parse for sandbox/testing
		var raw map[string]interface{}
		if err := json.Unmarshal(payload, &raw); err != nil {
			return nil, err
		}
		evtType, _ := raw["type"].(string)
		evtID, _ := raw["id"].(string)
		return &WebhookEvent{
			ID:        evtID,
			Type:      evtType,
			Data:      raw,
			Timestamp: time.Now().Unix(),
		}, nil
	}

	// Signature format: t=timestamp,v1=signature
	parts := strings.Split(signature, ",")
	var timestampStr, sigHex string
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			if kv[0] == "t" {
				timestampStr = kv[1]
			} else if kv[0] == "v1" {
				sigHex = kv[1]
			}
		}
	}

	if timestampStr == "" || sigHex == "" {
		return nil, errors.New("invalid stripe signature format")
	}

	// Signed payload is timestamp + "." + payload
	signedPayload := timestampStr + "." + string(payload)
	mac := hmac.New(sha256.New, []byte(g.webhookSecret))
	mac.Write([]byte(signedPayload))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(sigHex), []byte(expectedSig)) {
		return nil, errors.New("stripe webhook signature mismatch")
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, err
	}

	evtType, _ := raw["type"].(string)
	evtID, _ := raw["id"].(string)

	return &WebhookEvent{
		ID:        evtID,
		Type:      evtType,
		Data:      raw,
		Timestamp: time.Now().Unix(),
	}, nil
}
