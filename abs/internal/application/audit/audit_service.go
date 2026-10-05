// =============================================================================
// Samvada Platform — ABS Audit Log Application Service
// =============================================================================

package audit

import (
	"context"

	"go.uber.org/zap"

	"github.com/samvada/abs/internal/domain"
)

type AuditService struct {
	repo domain.AuditRepository
	log  *zap.Logger
}

func NewAuditService(repo domain.AuditRepository, log *zap.Logger) *AuditService {
	return &AuditService{
		repo: repo,
		log:  log.Named("audit_service"),
	}
}

func (s *AuditService) Record(ctx context.Context, entry *domain.AuditLog) error {
	if err := s.repo.Record(ctx, entry); err != nil {
		s.log.Error("Failed to record audit log entry",
			zap.String("action", entry.Action),
			zap.String("resource_type", entry.ResourceType),
			zap.Error(err),
		)
		return err
	}
	return nil
}

func (s *AuditService) List(ctx context.Context, filter domain.AuditLogFilter) ([]*domain.AuditLog, int64, error) {
	return s.repo.List(ctx, filter)
}
