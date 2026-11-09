package pendingauth

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	Create(ctx context.Context, pa *PendingAuth) error
	FindByID(ctx context.Context, id uuid.UUID) (*PendingAuth, error)
	FindByEmailAndPurpose(ctx context.Context, email string, purpose Purpose) (*PendingAuth, error)
	Update(ctx context.Context, pa *PendingAuth) error
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteExpired(ctx context.Context) (int64, error)
}
