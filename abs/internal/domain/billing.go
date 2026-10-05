// =============================================================================
// Samvada Platform — ABS Billing & Wallets Domain Entity & Repository
// =============================================================================

package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type Plan struct {
	ID                  uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Tier                string         `gorm:"type:varchar(50);uniqueIndex;not null" json:"tier"` // free, starter, pro, enterprise
	Name                string         `gorm:"type:varchar(100);not null" json:"name"`
	MonthlyPriceUSD     float64        `gorm:"type:numeric(10,2);not null" json:"monthly_price_usd"`
	MonthlyPriceINR     float64        `gorm:"type:numeric(10,2);not null" json:"monthly_price_inr"`
	IncludedMinutes     int            `gorm:"not null" json:"included_minutes"`
	IncludedAgents      int            `gorm:"not null" json:"included_agents"`
	IncludedWorkspaces  int            `gorm:"not null" json:"included_workspaces"`
	OveragePerMinuteUSD float64        `gorm:"type:numeric(10,4);not null" json:"overage_per_minute_usd"`
	OveragePerMinuteINR float64        `gorm:"type:numeric(10,4);not null" json:"overage_per_minute_inr"`
	Features            datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"features"`
	IsActive            bool           `gorm:"not null;default:true" json:"is_active"`
}

func (Plan) TableName() string {
	return "plans"
}

type Subscription struct {
	ID                    uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrgID                 uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex" json:"org_id"`
	PlanID                uuid.UUID  `gorm:"type:uuid;not null" json:"plan_id"`
	Status                string     `gorm:"type:varchar(50);not null;default:'active'" json:"status"` // trialing, active, past_due, canceled, unpaid
	BillingCycle          string     `gorm:"type:varchar(20);not null;default:'monthly'" json:"billing_cycle"`
	PaymentGateway        string     `gorm:"type:varchar(50);not null" json:"payment_gateway"` // stripe, razorpay
	GatewaySubscriptionID *string    `gorm:"type:varchar(255)" json:"gateway_subscription_id,omitempty"`
	GatewayCustomerID     string     `gorm:"type:varchar(255);not null" json:"gateway_customer_id"`
	CurrentPeriodStart    time.Time  `gorm:"not null" json:"current_period_start"`
	CurrentPeriodEnd      time.Time  `gorm:"not null" json:"current_period_end"`
	CancelAtPeriodEnd     bool       `gorm:"not null;default:false" json:"cancel_at_period_end"`
	CreatedAt             time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt             time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`

	Plan *Plan `gorm:"foreignKey:PlanID" json:"plan,omitempty"`
}

func (Subscription) TableName() string {
	return "subscriptions"
}

type Invoice struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	InvoiceNumber  string     `gorm:"type:varchar(50);uniqueIndex;not null" json:"invoice_number"`
	OrgID          uuid.UUID  `gorm:"type:uuid;not null;index:idx_invoices_org_id" json:"org_id"`
	SubscriptionID *uuid.UUID `gorm:"type:uuid" json:"subscription_id,omitempty"`
	PeriodStart    time.Time  `gorm:"not null" json:"period_start"`
	PeriodEnd      time.Time  `gorm:"not null" json:"period_end"`
	BaseAmount     float64    `gorm:"type:numeric(12,2);not null" json:"base_amount"`
	OverageAmount  float64    `gorm:"type:numeric(12,2);default:0.00" json:"overage_amount"`
	TaxAmount      float64    `gorm:"type:numeric(12,2);default:0.00" json:"tax_amount"`
	TotalAmount    float64    `gorm:"type:numeric(12,2);not null" json:"total_amount"`
	Currency       string     `gorm:"type:varchar(3);not null" json:"currency"`
	Status         string     `gorm:"type:varchar(50);not null;default:'draft'" json:"status"` // draft, open, paid, void, uncollectible
	GatewayInvoice *string    `gorm:"column:gateway_invoice_id;type:varchar(255)" json:"gateway_invoice_id,omitempty"`
	PDFURL         *string    `gorm:"column:pdf_url;type:text" json:"pdf_url,omitempty"`
	PaidAt         *time.Time `json:"paid_at,omitempty"`
	CreatedAt      time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
}

func (Invoice) TableName() string {
	return "invoices"
}

type PaymentMethod struct {
	ID                   uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrgID                uuid.UUID `gorm:"type:uuid;not null;index:idx_payment_methods_org_id" json:"org_id"`
	Gateway              string    `gorm:"type:varchar(50);not null" json:"gateway"`
	GatewayPaymentMethod string    `gorm:"column:gateway_payment_method_id;type:varchar(255);not null" json:"gateway_payment_method_id"`
	Brand                *string   `gorm:"type:varchar(50)" json:"brand,omitempty"`
	Last4                *string   `gorm:"type:varchar(4)" json:"last4,omitempty"`
	ExpMonth             *int      `json:"exp_month,omitempty"`
	ExpYear              *int      `json:"exp_year,omitempty"`
	IsDefault            bool      `gorm:"not null;default:false" json:"is_default"`
	CreatedAt            time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
}

func (PaymentMethod) TableName() string {
	return "payment_methods"
}

type OrganizationWallet struct {
	ID                     uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrgID                  uuid.UUID `gorm:"type:uuid;not null;uniqueIndex" json:"org_id"`
	BalanceUSD             float64   `gorm:"type:numeric(10,4);default:5.0000" json:"balance_usd"`
	PromotionalCreditUSD   float64   `gorm:"type:numeric(10,4);default:5.0000" json:"promotional_credit_usd"`
	LifetimeCreditAddedUSD float64   `gorm:"type:numeric(10,4);default:5.0000" json:"lifetime_credit_added_usd"`
	LifetimeSpendUSD       float64   `gorm:"type:numeric(10,4);default:0.0000" json:"lifetime_spend_usd"`
	Currency               string    `gorm:"type:varchar(3);default:'USD'" json:"currency"`
	IsFrozen               bool      `gorm:"not null;default:false" json:"is_frozen"`
	UpdatedAt              time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`

	// Associations
	Transactions []WalletTransaction `gorm:"foreignKey:WalletID;constraint:OnDelete:CASCADE" json:"transactions,omitempty"`
}

func (OrganizationWallet) TableName() string {
	return "organization_wallets"
}

type WalletTransaction struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WalletID        uuid.UUID  `gorm:"type:uuid;not null;index:idx_wallet_transactions_wallet_id" json:"wallet_id"`
	CallID          *uuid.UUID `gorm:"type:uuid" json:"call_id,omitempty"`
	Type            string     `gorm:"type:varchar(50);not null" json:"type"` // signup_promo, topup, call_deduction, refund
	AmountUSD       float64    `gorm:"type:numeric(10,4);not null" json:"amount_usd"`
	BalanceAfterUSD float64    `gorm:"type:numeric(10,4);not null" json:"balance_after_usd"`
	Description     *string    `gorm:"type:text" json:"description,omitempty"`
	CreatedAt       time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
}

func (WalletTransaction) TableName() string {
	return "wallet_transactions"
}

// BillingRepository defines data access methods for billing aggregates
type BillingRepository interface {
	GetPlanByID(ctx context.Context, id uuid.UUID) (*Plan, error)
	GetPlanByTier(ctx context.Context, tier string) (*Plan, error)
	ListActivePlans(ctx context.Context) ([]*Plan, error)

	GetSubscriptionByOrgID(ctx context.Context, orgID uuid.UUID) (*Subscription, error)
	CreateSubscription(ctx context.Context, sub *Subscription) error
	UpdateSubscription(ctx context.Context, sub *Subscription) error

	GetWalletByOrgID(ctx context.Context, orgID uuid.UUID) (*OrganizationWallet, error)
	GetWalletByID(ctx context.Context, walletID uuid.UUID) (*OrganizationWallet, error)
	CreateWallet(ctx context.Context, wallet *OrganizationWallet) error
	DeductCredits(ctx context.Context, walletID uuid.UUID, amountUSD float64, description string, callID *uuid.UUID) (*OrganizationWallet, *WalletTransaction, error)
	AddCredits(ctx context.Context, walletID uuid.UUID, amountUSD float64, txType, description string) (*OrganizationWallet, *WalletTransaction, error)
	ListWalletTransactions(ctx context.Context, walletID uuid.UUID, limit, offset int) ([]*WalletTransaction, int64, error)

	ListInvoices(ctx context.Context, orgID uuid.UUID) ([]*Invoice, error)
	CreateInvoice(ctx context.Context, invoice *Invoice) error
	GetInvoiceByID(ctx context.Context, id uuid.UUID) (*Invoice, error)
	UpdateInvoice(ctx context.Context, invoice *Invoice) error

	ListAllActiveSubscriptions(ctx context.Context) ([]*Subscription, error)

	AddPaymentMethod(ctx context.Context, pm *PaymentMethod) error
	ListPaymentMethods(ctx context.Context, orgID uuid.UUID) ([]*PaymentMethod, error)
	GetDefaultPaymentMethod(ctx context.Context, orgID uuid.UUID) (*PaymentMethod, error)
	SetDefaultPaymentMethod(ctx context.Context, orgID uuid.UUID, pmID uuid.UUID) error
	DeletePaymentMethod(ctx context.Context, orgID uuid.UUID, pmID uuid.UUID) error
}
