// =============================================================================
// Samvada Platform — ABS Billing & Wallet Repository (GORM PostgreSQL)
// =============================================================================

package repos

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/samvada/abs/internal/domain"
)

type billingRepo struct {
	db *gorm.DB
}

func NewBillingRepository(db *gorm.DB) domain.BillingRepository {
	return &billingRepo{db: db}
}

func (r *billingRepo) GetPlanByID(ctx context.Context, id uuid.UUID) (*domain.Plan, error) {
	var plan domain.Plan
	if err := r.db.WithContext(ctx).First(&plan, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrPlanNotFound
		}
		return nil, err
	}
	return &plan, nil
}

func (r *billingRepo) GetPlanByTier(ctx context.Context, tier string) (*domain.Plan, error) {
	var plan domain.Plan
	if err := r.db.WithContext(ctx).First(&plan, "tier = ?", tier).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrPlanNotFound
		}
		return nil, err
	}
	return &plan, nil
}

func (r *billingRepo) ListActivePlans(ctx context.Context) ([]*domain.Plan, error) {
	var plans []*domain.Plan
	err := r.db.WithContext(ctx).
		Where("is_active = ?", true).
		Order("monthly_price_usd ASC").
		Find(&plans).Error
	return plans, err
}

func (r *billingRepo) GetSubscriptionByOrgID(ctx context.Context, orgID uuid.UUID) (*domain.Subscription, error) {
	var sub domain.Subscription
	if err := r.db.WithContext(ctx).
		Preload("Plan").
		First(&sub, "org_id = ?", orgID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrSubscriptionNotFound
		}
		return nil, err
	}
	return &sub, nil
}

func (r *billingRepo) CreateSubscription(ctx context.Context, sub *domain.Subscription) error {
	if sub.ID == uuid.Nil {
		sub.ID = uuid.New()
	}
	return r.db.WithContext(ctx).Create(sub).Error
}

func (r *billingRepo) UpdateSubscription(ctx context.Context, sub *domain.Subscription) error {
	return r.db.WithContext(ctx).Omit(clause.Associations).Save(sub).Error
}

func (r *billingRepo) GetWalletByOrgID(ctx context.Context, orgID uuid.UUID) (*domain.OrganizationWallet, error) {
	var wallet domain.OrganizationWallet
	if err := r.db.WithContext(ctx).First(&wallet, "org_id = ?", orgID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrWalletNotFound
		}
		return nil, err
	}
	return &wallet, nil
}

func (r *billingRepo) GetWalletByID(ctx context.Context, walletID uuid.UUID) (*domain.OrganizationWallet, error) {
	var wallet domain.OrganizationWallet
	if err := r.db.WithContext(ctx).First(&wallet, "id = ?", walletID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrWalletNotFound
		}
		return nil, err
	}
	return &wallet, nil
}

func (r *billingRepo) CreateWallet(ctx context.Context, wallet *domain.OrganizationWallet) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if wallet.ID == uuid.Nil {
			wallet.ID = uuid.New()
		}
		if wallet.BalanceUSD <= 0 {
			wallet.BalanceUSD = 5.0000
			wallet.PromotionalCreditUSD = 5.0000
			wallet.LifetimeCreditAddedUSD = 5.0000
			wallet.LifetimeSpendUSD = 0.0000
		}
		if wallet.Currency == "" {
			wallet.Currency = "USD"
		}
		wallet.UpdatedAt = time.Now().UTC()

		if err := tx.Create(wallet).Error; err != nil {
			return err
		}

		desc := fmt.Sprintf("Welcome promotional credit: $%.2f", wallet.PromotionalCreditUSD)
		promoTx := &domain.WalletTransaction{
			ID:              uuid.New(),
			WalletID:        wallet.ID,
			Type:            "signup_promo",
			AmountUSD:       wallet.PromotionalCreditUSD,
			BalanceAfterUSD: wallet.BalanceUSD,
			Description:     &desc,
			CreatedAt:       time.Now().UTC(),
		}
		return tx.Create(promoTx).Error
	})
}

// DeductCredits executes an atomic wallet deduction protected by row-level locking
func (r *billingRepo) DeductCredits(
	ctx context.Context,
	walletID uuid.UUID,
	amountUSD float64,
	description string,
	callID *uuid.UUID,
) (*domain.OrganizationWallet, *domain.WalletTransaction, error) {
	var updatedWallet domain.OrganizationWallet
	var txRecord domain.WalletTransaction

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Row-level lock FOR UPDATE
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&updatedWallet, "id = ?", walletID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrWalletNotFound
			}
			return err
		}

		if updatedWallet.IsFrozen {
			return domain.ErrWalletFrozen
		}

		if updatedWallet.BalanceUSD < amountUSD {
			return domain.ErrDemoCreditsExhausted
		}

		newBalance := updatedWallet.BalanceUSD - amountUSD
		updatedWallet.BalanceUSD = newBalance
		updatedWallet.LifetimeSpendUSD += amountUSD
		updatedWallet.UpdatedAt = time.Now().UTC()

		if err := tx.Save(&updatedWallet).Error; err != nil {
			return err
		}

		var descPtr *string
		if description != "" {
			descPtr = &description
		}

		txRecord = domain.WalletTransaction{
			ID:              uuid.New(),
			WalletID:        walletID,
			CallID:          callID,
			Type:            "call_deduction",
			AmountUSD:       amountUSD,
			BalanceAfterUSD: newBalance,
			Description:     descPtr,
			CreatedAt:       time.Now().UTC(),
		}

		if err := tx.Create(&txRecord).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, nil, err
	}

	return &updatedWallet, &txRecord, nil
}

// AddCredits executes an atomic wallet credit protected by row-level locking
func (r *billingRepo) AddCredits(
	ctx context.Context,
	walletID uuid.UUID,
	amountUSD float64,
	txType, description string,
) (*domain.OrganizationWallet, *domain.WalletTransaction, error) {
	var updatedWallet domain.OrganizationWallet
	var txRecord domain.WalletTransaction

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&updatedWallet, "id = ?", walletID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrWalletNotFound
			}
			return err
		}

		if updatedWallet.IsFrozen {
			return domain.ErrWalletFrozen
		}

		newBalance := updatedWallet.BalanceUSD + amountUSD
		updatedWallet.BalanceUSD = newBalance
		updatedWallet.LifetimeCreditAddedUSD += amountUSD
		updatedWallet.UpdatedAt = time.Now().UTC()

		if err := tx.Save(&updatedWallet).Error; err != nil {
			return err
		}

		var descPtr *string
		if description != "" {
			descPtr = &description
		}

		if txType == "" {
			txType = "topup"
		}

		txRecord = domain.WalletTransaction{
			ID:              uuid.New(),
			WalletID:        walletID,
			Type:            txType,
			AmountUSD:       amountUSD,
			BalanceAfterUSD: newBalance,
			Description:     descPtr,
			CreatedAt:       time.Now().UTC(),
		}

		if err := tx.Create(&txRecord).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, nil, err
	}

	return &updatedWallet, &txRecord, nil
}

func (r *billingRepo) ListWalletTransactions(ctx context.Context, walletID uuid.UUID, limit, offset int) ([]*domain.WalletTransaction, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	var count int64
	var txs []*domain.WalletTransaction

	baseQuery := r.db.WithContext(ctx).Model(&domain.WalletTransaction{}).Where("wallet_id = ?", walletID)

	if err := baseQuery.Count(&count).Error; err != nil {
		return nil, 0, err
	}

	err := baseQuery.
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&txs).Error

	return txs, count, err
}

func (r *billingRepo) ListInvoices(ctx context.Context, orgID uuid.UUID) ([]*domain.Invoice, error) {
	var invoices []*domain.Invoice
	err := r.db.WithContext(ctx).
		Where("org_id = ?", orgID).
		Order("created_at DESC").
		Find(&invoices).Error
	return invoices, err
}

func (r *billingRepo) CreateInvoice(ctx context.Context, invoice *domain.Invoice) error {
	if invoice.ID == uuid.Nil {
		invoice.ID = uuid.New()
	}
	if invoice.CreatedAt.IsZero() {
		invoice.CreatedAt = time.Now().UTC()
	}
	return r.db.WithContext(ctx).Create(invoice).Error
}

func (r *billingRepo) GetInvoiceByID(ctx context.Context, id uuid.UUID) (*domain.Invoice, error) {
	var inv domain.Invoice
	if err := r.db.WithContext(ctx).First(&inv, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &inv, nil
}

func (r *billingRepo) UpdateInvoice(ctx context.Context, invoice *domain.Invoice) error {
	return r.db.WithContext(ctx).Save(invoice).Error
}

func (r *billingRepo) ListAllActiveSubscriptions(ctx context.Context) ([]*domain.Subscription, error) {
	var subs []*domain.Subscription
	err := r.db.WithContext(ctx).
		Preload("Plan").
		Where("status IN ('active', 'trialing')").
		Find(&subs).Error
	return subs, err
}

func (r *billingRepo) AddPaymentMethod(ctx context.Context, pm *domain.PaymentMethod) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if pm.ID == uuid.Nil {
			pm.ID = uuid.New()
		}
		if pm.CreatedAt.IsZero() {
			pm.CreatedAt = time.Now().UTC()
		}
		if pm.IsDefault {
			if err := tx.Model(&domain.PaymentMethod{}).
				Where("org_id = ?", pm.OrgID).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Create(pm).Error
	})
}

func (r *billingRepo) ListPaymentMethods(ctx context.Context, orgID uuid.UUID) ([]*domain.PaymentMethod, error) {
	var pms []*domain.PaymentMethod
	err := r.db.WithContext(ctx).
		Where("org_id = ?", orgID).
		Order("is_default DESC, created_at DESC").
		Find(&pms).Error
	return pms, err
}

func (r *billingRepo) GetDefaultPaymentMethod(ctx context.Context, orgID uuid.UUID) (*domain.PaymentMethod, error) {
	var pm domain.PaymentMethod
	if err := r.db.WithContext(ctx).
		Where("org_id = ? AND is_default = ?", orgID, true).
		First(&pm).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Fallback to latest payment method
			if err := r.db.WithContext(ctx).
				Where("org_id = ?", orgID).
				Order("created_at DESC").
				First(&pm).Error; err != nil {
				return nil, err
			}
			return &pm, nil
		}
		return nil, err
	}
	return &pm, nil
}

func (r *billingRepo) SetDefaultPaymentMethod(ctx context.Context, orgID uuid.UUID, pmID uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&domain.PaymentMethod{}).
			Where("org_id = ?", orgID).
			Update("is_default", false).Error; err != nil {
			return err
		}
		res := tx.Model(&domain.PaymentMethod{}).
			Where("org_id = ? AND id = ?", orgID, pmID).
			Update("is_default", true)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func (r *billingRepo) DeletePaymentMethod(ctx context.Context, orgID uuid.UUID, pmID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("org_id = ? AND id = ?", orgID, pmID).
		Delete(&domain.PaymentMethod{}).Error
}

