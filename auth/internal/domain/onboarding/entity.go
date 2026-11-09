package onboarding

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Organization struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	OwnerID   uuid.UUID `json:"owner_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Workspace struct {
	ID        uuid.UUID  `json:"id"`
	Name      string     `json:"name"`
	OrgID     *uuid.UUID `json:"org_id,omitempty"`
	OwnerID   uuid.UUID  `json:"owner_id"`
	IsDefault bool       `json:"is_default"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type UserProfile struct {
	ID                  uuid.UUID `json:"id"`
	UserID              uuid.UUID `json:"user_id"`
	FirstName           string    `json:"first_name"`
	LastName            *string   `json:"last_name,omitempty"`
	AccountType         string    `json:"account_type"` // "business" | "individual"
	Industries          []string  `json:"industries"`
	OnboardingCompleted bool      `json:"onboarding_completed"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type Repository interface {
	CreateProfile(ctx context.Context, profile *UserProfile) error
	GetProfileByUserID(ctx context.Context, userID uuid.UUID) (*UserProfile, error)
	UpdateProfile(ctx context.Context, profile *UserProfile) error

	CreateOrganization(ctx context.Context, org *Organization) error
	GetOrganizationByOwnerID(ctx context.Context, ownerID uuid.UUID) (*Organization, error)

	CreateWorkspace(ctx context.Context, ws *Workspace) error
	GetDefaultWorkspaceByOwnerID(ctx context.Context, ownerID uuid.UUID) (*Workspace, error)
}
