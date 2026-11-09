package oauthsession

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	Create(ctx context.Context, session *OAuthSession) error
	FindByID(ctx context.Context, id uuid.UUID) (*OAuthSession, error)
	FindByState(ctx context.Context, state string) (*OAuthSession, error)
	UpdateLoginChallenge(ctx context.Context, id uuid.UUID, challenge string) error
	MarkUsed(ctx context.Context, id uuid.UUID) error
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteExpired(ctx context.Context) (int64, error)
}
