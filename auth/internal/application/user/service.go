package user

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/ridesafety/auth-service/internal/domain/user"
	"github.com/ridesafety/auth-service/internal/platform/identity"
	apperrors "github.com/ridesafety/auth-service/internal/shared/errors"
)

type Service interface {
	DeleteUser(ctx context.Context, id uuid.UUID) error
}

type service struct {
	userRepo     user.Repository
	orchestrator identity.IdentityOrchestrator
}

func NewService(userRepo user.Repository, orchestrator identity.IdentityOrchestrator) Service {
	return &service{
		userRepo:     userRepo,
		orchestrator: orchestrator,
	}
}

func (s *service) DeleteUser(ctx context.Context, id uuid.UUID) error {
	// 1. Find user in Postgres
	u, err := s.userRepo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return apperrors.NotFound("User not found", err)
		}
		return apperrors.Internal("Failed to load user", err)
	}

	// 2. Delete relationships from Keto (to prevent orphaned tuples)
	_ = s.orchestrator.DeleteAllRelationshipsForSubject(ctx, u.KratosID)

	// 3. Delete from Kratos (identity provider)
	if err := s.orchestrator.DeleteIdentity(ctx, u.KratosID); err != nil {
		return apperrors.Internal("Failed to delete identity in Kratos", err)
	}

	// 4. Hard delete from local DB
	if err := s.userRepo.Delete(ctx, id); err != nil {
		return apperrors.Internal("Failed to delete user locally", err)
	}

	return nil
}
