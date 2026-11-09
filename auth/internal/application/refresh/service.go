package refresh

import (
	"context"
	"errors"

	"github.com/ridesafety/auth-service/internal/platform/identity"
	apperrors "github.com/ridesafety/auth-service/internal/shared/errors"
)

type Service interface {
	Refresh(ctx context.Context, refreshToken string) (*identity.TokenPair, error)
}

type service struct {
	orchestrator identity.IdentityOrchestrator
}

func NewService(orchestrator identity.IdentityOrchestrator) Service {
	return &service{
		orchestrator: orchestrator,
	}
}

func (s *service) Refresh(ctx context.Context, refreshToken string) (*identity.TokenPair, error) {
	if refreshToken == "" {
		return nil, apperrors.BadRequest("Refresh token is required", nil)
	}

	tokens, err := s.orchestrator.RotateTokens(ctx, refreshToken)
	if err != nil {
		if errors.Is(err, identity.ErrInvalidRefreshToken) {
			return nil, apperrors.Unauthorized("Invalid or expired refresh token", err)
		}
		return nil, apperrors.Internal("Failed to rotate session tokens", err)
	}

	return tokens, nil
}
