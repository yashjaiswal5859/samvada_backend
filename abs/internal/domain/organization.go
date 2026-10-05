// =============================================================================
// Samvada Platform — ABS Organization Domain Entity & Repository
// =============================================================================

package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Organization struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name            string    `gorm:"type:varchar(255);not null" json:"name"`
	Slug            string    `gorm:"type:varchar(100);uniqueIndex;not null" json:"slug"`
	BillingCurrency string    `gorm:"type:varchar(3);default:'USD'" json:"billing_currency"`
	BillingEmail    string    `gorm:"type:varchar(255);not null" json:"billing_email"`
	TaxID           *string   `gorm:"type:varchar(100)" json:"tax_id,omitempty"`
	LogoURL         *string   `gorm:"type:text" json:"logo_url,omitempty"`
	CreatedAt       time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt       time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`

	// Associations
	Members    []OrgMember `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE" json:"members,omitempty"`
	Workspaces []Workspace `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE" json:"workspaces,omitempty"`
}

func (Organization) TableName() string {
	return "organizations"
}

type OrgMember struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrgID     uuid.UUID  `gorm:"type:uuid;not null;index:idx_org_members_user_id" json:"org_id"`
	UserID    uuid.UUID  `gorm:"type:uuid;not null;index:idx_org_members_user_id" json:"user_id"`
	Email     string     `gorm:"type:varchar(255);not null" json:"email"`
	Role      string     `gorm:"type:varchar(50);not null;default:'member'" json:"role"` // owner, admin, member
	Status    string     `gorm:"type:varchar(50);default:'active'" json:"status"`        // active, invited, suspended
	InvitedAt time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"invited_at"`
	JoinedAt  *time.Time `json:"joined_at,omitempty"`

	Organization *Organization `gorm:"foreignKey:OrgID" json:"organization,omitempty"`
}

func (OrgMember) TableName() string {
	return "org_members"
}

// OrgRepository defines data access methods for Organization aggregates
type OrgRepository interface {
	Create(ctx context.Context, org *Organization) error
	GetByID(ctx context.Context, id uuid.UUID) (*Organization, error)
	GetBySlug(ctx context.Context, slug string) (*Organization, error)
	Update(ctx context.Context, org *Organization) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListForUser(ctx context.Context, userID uuid.UUID) ([]*Organization, error)

	AddMember(ctx context.Context, member *OrgMember) error
	GetMember(ctx context.Context, orgID, userID uuid.UUID) (*OrgMember, error)
	ListMembers(ctx context.Context, orgID uuid.UUID) ([]*OrgMember, error)
	UpdateMemberRole(ctx context.Context, orgID, userID uuid.UUID, newRole string) error
	RemoveMember(ctx context.Context, orgID, userID uuid.UUID) error
}
