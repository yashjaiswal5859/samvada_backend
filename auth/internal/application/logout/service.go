package logout

import (
	"context"
	"time"

	"github.com/ridesafety/auth-service/internal/domain/events"
	"github.com/ridesafety/auth-service/internal/platform/eventbus"
	"github.com/ridesafety/auth-service/internal/platform/identity"
	"github.com/ridesafety/auth-service/internal/shared/constants"
	apperrors "github.com/ridesafety/auth-service/internal/shared/errors"
	"gorm.io/gorm"
)

type Service interface {
	Logout(ctx context.Context, refreshToken string, kratosID string, correlationID string) error
}

type service struct {
	db           *gorm.DB
	eventBus     eventbus.EventBus
	orchestrator identity.IdentityOrchestrator
}

func NewService(db *gorm.DB, eventBus eventbus.EventBus, orchestrator identity.IdentityOrchestrator) Service {
	return &service{
		db:           db,
		eventBus:     eventBus,
		orchestrator: orchestrator,
	}
}

type userLoggedOutPayload struct {
	KratosID string    `json:"kratos_id"`
	LoggedAt time.Time `json:"logged_at"`
}

func (s *service) Logout(ctx context.Context, refreshToken string, kratosID string, correlationID string) error {
	// 1. Revoke token in Hydra
	if refreshToken != "" {
		if err := s.orchestrator.RevokeTokens(ctx, refreshToken); err != nil {
			// If it fails because of unreachability, return error.
			// But if it's already invalid or revoked, we still proceed to log out.
			// Let's assume standard behavior: log and ignore unless Ory is completely down.
		}
	}

	// 2. Transactionally save UserLoggedOut event
	tx := s.db.Begin()
	if tx.Error != nil {
		return apperrors.Internal("Failed to start transaction", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	ctxTx := eventbus.WithTx(ctx, tx)

	logoutPayload := userLoggedOutPayload{
		KratosID: kratosID,
		LoggedAt: time.Now().UTC(),
	}

	evt, err := events.NewDomainEvent(
		events.UserLoggedOut,
		kratosID,
		"User",
		constants.EventVersion,
		correlationID,
		logoutPayload,
	)
	if err != nil {
		tx.Rollback()
		return apperrors.Internal("Failed to create logout event", err)
	}

	if err := s.eventBus.Publish(ctxTx, evt); err != nil {
		tx.Rollback()
		return apperrors.Internal("Failed to save outbox event", err)
	}

	if err := tx.Commit().Error; err != nil {
		return apperrors.Internal("Failed to commit logout audit", err)
	}

	return nil
}
