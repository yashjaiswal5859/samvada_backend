// =============================================================================
// Samvada Platform — Reporting Engine Usage & Metering Service
// =============================================================================

package usage

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/reporting/internal/domain"
)

type MeteringService interface {
	GetUsageSummary(ctx context.Context, orgID uuid.UUID, start, end time.Time) (*domain.UsageSummary, error)
	GetCurrentMonthUsage(ctx context.Context, orgID, workspaceID uuid.UUID) (*domain.CurrentMonthUsage, error)
}

type meteringService struct {
	usageRepo domain.UsageRepository
	log       *zap.Logger
}

func NewMeteringService(usageRepo domain.UsageRepository, log *zap.Logger) MeteringService {
	return &meteringService{
		usageRepo: usageRepo,
		log:       log.Named("metering_service"),
	}
}

func (s *meteringService) GetUsageSummary(
	ctx context.Context,
	orgID uuid.UUID,
	start, end time.Time,
) (*domain.UsageSummary, error) {
	return s.usageRepo.GetUsageSummary(ctx, orgID, start, end)
}

func (s *meteringService) GetCurrentMonthUsage(
	ctx context.Context,
	orgID, workspaceID uuid.UUID,
) (*domain.CurrentMonthUsage, error) {
	return s.usageRepo.GetCurrentMonthUsage(ctx, orgID, workspaceID)
}
