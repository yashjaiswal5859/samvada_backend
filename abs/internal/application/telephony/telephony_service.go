// =============================================================================
// Samvada Platform — ABS Telephony Application Service
// =============================================================================

package telephony

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/domain"
	infraTelephony "github.com/samvada/abs/internal/infrastructure/telephony"
)

type TelephonyService interface {
	ListWorkspaceNumbers(ctx context.Context, workspaceID uuid.UUID, search string) ([]*domain.PhoneNumber, error)
	ListAvailableNumbersFromDB(ctx context.Context, workspaceID *uuid.UUID, search string) ([]*domain.PhoneNumber, error)
	AllocateNumberFromDB(ctx context.Context, workspaceID, numberID uuid.UUID, agentID *uuid.UUID) (*domain.PhoneNumber, error)
	GetNumber(ctx context.Context, workspaceID, id uuid.UUID) (*domain.PhoneNumber, error)
	GetByNumber(ctx context.Context, number string) (*domain.PhoneNumber, error)
	SearchAvailableNumbers(ctx context.Context, provider, country, areaCode, numType string, limit int) ([]infraTelephony.AvailableNumber, error)
	PurchaseNumber(ctx context.Context, workspaceID uuid.UUID, provider, phoneNumber, webhookURL string) (*domain.PhoneNumber, error)
	ProvisionNumber(ctx context.Context, workspaceID uuid.UUID, number, provider, numType string, assignedAgentID *uuid.UUID) (*domain.PhoneNumber, error)
	AssignAgent(ctx context.Context, workspaceID, id, agentID uuid.UUID) error
	UnassignAgent(ctx context.Context, workspaceID, id uuid.UUID) error
	ReleaseNumber(ctx context.Context, workspaceID, id uuid.UUID) error
}

type telephonyService struct {
	telephonyRepo domain.TelephonyRepository
	carriers      map[string]infraTelephony.CarrierAdapter
	log           *zap.Logger
}

func NewTelephonyService(
	telephonyRepo domain.TelephonyRepository,
	carriers map[string]infraTelephony.CarrierAdapter,
	log *zap.Logger,
) TelephonyService {
	return &telephonyService{
		telephonyRepo: telephonyRepo,
		carriers:      carriers,
		log:           log.Named("telephony_service"),
	}
}

func (s *telephonyService) getCarrier(provider string) infraTelephony.CarrierAdapter {
	if provider == "" {
		provider = "twilio"
	}
	adapter, exists := s.carriers[strings.ToLower(provider)]
	if !exists {
		// Fallback to twilio if available, or first available adapter
		for _, a := range s.carriers {
			return a
		}
	}
	return adapter
}

func (s *telephonyService) ListWorkspaceNumbers(ctx context.Context, workspaceID uuid.UUID, search string) ([]*domain.PhoneNumber, error) {
	return s.telephonyRepo.ListByWorkspace(ctx, workspaceID, search)
}

func (s *telephonyService) ListAvailableNumbersFromDB(ctx context.Context, workspaceID *uuid.UUID, search string) ([]*domain.PhoneNumber, error) {
	return s.telephonyRepo.ListAvailable(ctx, workspaceID, search)
}

func (s *telephonyService) AllocateNumberFromDB(ctx context.Context, workspaceID, numberID uuid.UUID, agentID *uuid.UUID) (*domain.PhoneNumber, error) {
	return s.telephonyRepo.AllocateNumber(ctx, workspaceID, numberID, agentID)
}

func (s *telephonyService) GetNumber(ctx context.Context, workspaceID, id uuid.UUID) (*domain.PhoneNumber, error) {
	return s.telephonyRepo.GetByID(ctx, workspaceID, id)
}

func (s *telephonyService) GetByNumber(ctx context.Context, number string) (*domain.PhoneNumber, error) {
	return s.telephonyRepo.GetByNumber(ctx, number)
}

func (s *telephonyService) SearchAvailableNumbers(
	ctx context.Context,
	provider, country, areaCode, numType string,
	limit int,
) ([]infraTelephony.AvailableNumber, error) {
	carrier := s.getCarrier(provider)
	if carrier == nil {
		return nil, fmt.Errorf("no carrier configured for provider: %s", provider)
	}
	return carrier.SearchAvailableNumbers(ctx, country, areaCode, numType, limit)
}

func (s *telephonyService) PurchaseNumber(
	ctx context.Context,
	workspaceID uuid.UUID,
	provider, phoneNumber, webhookURL string,
) (*domain.PhoneNumber, error) {
	carrier := s.getCarrier(provider)
	if carrier == nil {
		return nil, fmt.Errorf("no carrier configured for provider: %s", provider)
	}

	pn, err := carrier.PurchaseNumber(ctx, phoneNumber, webhookURL)
	if err != nil {
		s.log.Error("Carrier purchase failed", zap.Error(err), zap.String("number", phoneNumber))
		return nil, err
	}

	pn.WorkspaceID = &workspaceID
	if err := s.telephonyRepo.Create(ctx, pn); err != nil {
		s.log.Error("Failed to persist purchased phone number", zap.Error(err))
		return nil, err
	}

	s.log.Info("Successfully purchased and registered DID",
		zap.String("number", pn.PhoneNumber),
		zap.String("workspace_id", workspaceID.String()),
		zap.String("provider", pn.Provider),
	)
	return pn, nil
}

func (s *telephonyService) ProvisionNumber(
	ctx context.Context,
	workspaceID uuid.UUID,
	number, provider, numType string,
	assignedAgentID *uuid.UUID,
) (*domain.PhoneNumber, error) {
	if provider == "" {
		provider = "twilio"
	}
	if numType == "" {
		numType = "toll_free"
	}

	pn := &domain.PhoneNumber{
		ID:              uuid.New(),
		WorkspaceID:     &workspaceID,
		PhoneNumber:     number,
		Type:            numType,
		Provider:        provider,
		AssignedAgentID: assignedAgentID,
		Status:          "active",
		MonthlyCostUSD:  1.00,
		CreatedAt:       time.Now().UTC(),
	}

	if err := s.telephonyRepo.Create(ctx, pn); err != nil {
		s.log.Error("Failed to provision phone number", zap.Error(err))
		return nil, err
	}
	return pn, nil
}

func (s *telephonyService) AssignAgent(ctx context.Context, workspaceID, id, agentID uuid.UUID) error {
	return s.telephonyRepo.AssignAgent(ctx, workspaceID, id, agentID)
}

func (s *telephonyService) UnassignAgent(ctx context.Context, workspaceID, id uuid.UUID) error {
	return s.telephonyRepo.UnassignAgent(ctx, workspaceID, id)
}

func (s *telephonyService) ReleaseNumber(ctx context.Context, workspaceID, id uuid.UUID) error {
	pn, err := s.telephonyRepo.GetByID(ctx, workspaceID, id)
	if err != nil {
		return err
	}

	if pn.CarrierSID != nil && *pn.CarrierSID != "" {
		carrier := s.getCarrier(pn.Provider)
		if carrier != nil {
			if err := carrier.ReleaseNumber(ctx, *pn.CarrierSID); err != nil {
				s.log.Warn("Failed to release carrier DID, proceeding with local deletion", zap.Error(err))
			}
		}
	}

	return s.telephonyRepo.Delete(ctx, workspaceID, id)
}
