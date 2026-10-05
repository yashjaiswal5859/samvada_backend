// =============================================================================
// Samvada Platform — Reporting Engine Calls Application Service
// =============================================================================

package calls

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/reporting/internal/domain"
)

type CallDetail struct {
	Event *domain.CallEvent       `json:"event"`
	Turns []*domain.SentimentTurn `json:"turns"`
}

type CallsService interface {
	ListCalls(ctx context.Context, filter domain.CallFilter) ([]*domain.CallEvent, int64, error)
	GetCallDetail(ctx context.Context, orgID, callID uuid.UUID) (*CallDetail, error)
}

type callsService struct {
	callRepo domain.CallRepository
	log      *zap.Logger
}

func NewCallsService(callRepo domain.CallRepository, log *zap.Logger) CallsService {
	return &callsService{
		callRepo: callRepo,
		log:      log.Named("calls_service"),
	}
}

func (s *callsService) ListCalls(ctx context.Context, filter domain.CallFilter) ([]*domain.CallEvent, int64, error) {
	return s.callRepo.ListCalls(ctx, filter)
}

func (s *callsService) GetCallDetail(ctx context.Context, orgID, callID uuid.UUID) (*CallDetail, error) {
	ev, err := s.callRepo.GetCallByID(ctx, orgID, callID)
	if err != nil {
		return nil, err
	}

	turns, err := s.callRepo.GetCallTurns(ctx, callID)
	if err != nil {
		s.log.Warn("Could not load sentiment turns for call", zap.String("call_id", callID.String()), zap.Error(err))
		turns = []*domain.SentimentTurn{}
	}

	return &CallDetail{
		Event: ev,
		Turns: turns,
	}, nil
}
