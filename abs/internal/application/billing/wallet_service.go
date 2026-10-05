// =============================================================================
// Samvada Platform — ABS Wallet & Billing Application Service
// =============================================================================

package billing

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/domain"
)

type CallAuthorizationResult struct {
	IsAuthorized            bool    `json:"is_authorized"`
	CurrentBalanceUSD       float64 `json:"current_balance_usd"`
	EstimatedRemainingMinutes int   `json:"estimated_remaining_minutes"`
	DenialReason            *string `json:"denial_reason,omitempty"`
}

type WalletService interface {
	GetWallet(ctx context.Context, orgID uuid.UUID) (*domain.OrganizationWallet, error)
	CheckCallAuthorization(ctx context.Context, orgID uuid.UUID) (*CallAuthorizationResult, error)
	DeductCallCost(ctx context.Context, orgID uuid.UUID, amountUSD float64, description string, callID *uuid.UUID) (*domain.OrganizationWallet, *domain.WalletTransaction, error)
	TopUpWallet(ctx context.Context, orgID uuid.UUID, amountUSD float64, paymentReference string) (*domain.OrganizationWallet, *domain.WalletTransaction, error)
	ListTransactions(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]*domain.WalletTransaction, int64, error)
	ListInvoices(ctx context.Context, orgID uuid.UUID) ([]*domain.Invoice, error)
}

type walletService struct {
	billingRepo domain.BillingRepository
	log         *zap.Logger
}

func NewWalletService(billingRepo domain.BillingRepository, log *zap.Logger) WalletService {
	return &walletService{
		billingRepo: billingRepo,
		log:         log.Named("wallet_service"),
	}
}

func (s *walletService) GetWallet(ctx context.Context, orgID uuid.UUID) (*domain.OrganizationWallet, error) {
	return s.billingRepo.GetWalletByOrgID(ctx, orgID)
}

func (s *walletService) CheckCallAuthorization(ctx context.Context, orgID uuid.UUID) (*CallAuthorizationResult, error) {
	wallet, err := s.billingRepo.GetWalletByOrgID(ctx, orgID)
	if err != nil {
		s.log.Warn("Wallet lookup failed for call authorization", zap.String("org_id", orgID.String()), zap.Error(err))
		denial := "Wallet not found for organization"
		return &CallAuthorizationResult{
			IsAuthorized: false,
			DenialReason: &denial,
		}, nil
	}

	if wallet.IsFrozen {
		denial := "Organization wallet is frozen"
		return &CallAuthorizationResult{
			IsAuthorized:      false,
			CurrentBalanceUSD: wallet.BalanceUSD,
			DenialReason:     &denial,
		}, nil
	}

	// Rate per minute baseline is $0.05
	ratePerMin := 0.05
	sub, err := s.billingRepo.GetSubscriptionByOrgID(ctx, orgID)
	if err == nil && sub != nil && sub.Plan != nil && sub.Plan.OveragePerMinuteUSD > 0 {
		ratePerMin = sub.Plan.OveragePerMinuteUSD
	}

	if wallet.BalanceUSD < ratePerMin {
		denial := "Insufficient credits: demo credits exhausted, please top up wallet"
		return &CallAuthorizationResult{
			IsAuthorized:            false,
			CurrentBalanceUSD:       wallet.BalanceUSD,
			EstimatedRemainingMinutes: 0,
			DenialReason:            &denial,
		}, nil
	}

	estimatedMinutes := int(wallet.BalanceUSD / ratePerMin)

	return &CallAuthorizationResult{
		IsAuthorized:            true,
		CurrentBalanceUSD:       wallet.BalanceUSD,
		EstimatedRemainingMinutes: estimatedMinutes,
	}, nil
}

func (s *walletService) DeductCallCost(
	ctx context.Context,
	orgID uuid.UUID,
	amountUSD float64,
	description string,
	callID *uuid.UUID,
) (*domain.OrganizationWallet, *domain.WalletTransaction, error) {
	wallet, err := s.billingRepo.GetWalletByOrgID(ctx, orgID)
	if err != nil {
		return nil, nil, err
	}

	if description == "" {
		description = "Call turn cost deduction"
	}

	updatedWallet, tx, err := s.billingRepo.DeductCredits(ctx, wallet.ID, amountUSD, description, callID)
	if err != nil {
		s.log.Error("Failed to deduct call cost from wallet",
			zap.String("wallet_id", wallet.ID.String()),
			zap.Float64("amount_usd", amountUSD),
			zap.Error(err),
		)
		return nil, nil, err
	}

	s.log.Info("Call cost deducted atomically",
		zap.String("wallet_id", wallet.ID.String()),
		zap.Float64("amount_usd", amountUSD),
		zap.Float64("new_balance", updatedWallet.BalanceUSD),
	)

	return updatedWallet, tx, nil
}

func (s *walletService) TopUpWallet(
	ctx context.Context,
	orgID uuid.UUID,
	amountUSD float64,
	paymentReference string,
) (*domain.OrganizationWallet, *domain.WalletTransaction, error) {
	wallet, err := s.billingRepo.GetWalletByOrgID(ctx, orgID)
	if err != nil {
		return nil, nil, err
	}

	desc := fmt.Sprintf("Wallet top-up (ref: %s)", paymentReference)
	updatedWallet, tx, err := s.billingRepo.AddCredits(ctx, wallet.ID, amountUSD, "topup", desc)
	if err != nil {
		s.log.Error("Failed to add funds to wallet", zap.Error(err))
		return nil, nil, err
	}

	s.log.Info("Wallet topped up successfully",
		zap.String("wallet_id", wallet.ID.String()),
		zap.Float64("amount_usd", amountUSD),
		zap.Float64("new_balance", updatedWallet.BalanceUSD),
	)

	return updatedWallet, tx, nil
}

func (s *walletService) ListTransactions(
	ctx context.Context,
	orgID uuid.UUID,
	limit, offset int,
) ([]*domain.WalletTransaction, int64, error) {
	wallet, err := s.billingRepo.GetWalletByOrgID(ctx, orgID)
	if err != nil {
		return nil, 0, err
	}
	return s.billingRepo.ListWalletTransactions(ctx, wallet.ID, limit, offset)
}

func (s *walletService) ListInvoices(ctx context.Context, orgID uuid.UUID) ([]*domain.Invoice, error) {
	return s.billingRepo.ListInvoices(ctx, orgID)
}
