// =============================================================================
// Samvada Platform — ABS Workspace Domain Entity & Repository
// =============================================================================

package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Workspace struct {
	ID                 uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrgID              uuid.UUID `gorm:"type:uuid;not null;index:idx_workspaces_org_id" json:"org_id"`
	Name               string    `gorm:"type:varchar(255);not null" json:"name"`
	Slug               string    `gorm:"type:varchar(100);not null;index:idx_workspaces_org_slug,unique" json:"slug"`
	Icon               string    `gorm:"type:varchar(32);default:'💼'" json:"icon"`
	Description        *string   `gorm:"type:text" json:"description,omitempty"`
	IsDefault          bool      `gorm:"not null;default:false" json:"is_default"`
	MonthlyMinuteLimit int       `gorm:"not null;default:10000" json:"monthly_minute_limit"`
	CreatedAt          time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt          time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`

	// Associations
	Organization *Organization     `gorm:"foreignKey:OrgID" json:"organization,omitempty"`
	Members      []WorkspaceMember `gorm:"foreignKey:WorkspaceID;constraint:OnDelete:CASCADE" json:"members,omitempty"`
}

func (Workspace) TableName() string {
	return "workspaces"
}

type WorkspaceMember struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WorkspaceID   uuid.UUID `gorm:"type:uuid;not null;index:idx_workspace_members_user_id" json:"workspace_id"`
	UserID        uuid.UUID `gorm:"type:uuid;not null;index:idx_workspace_members_user_id" json:"user_id"`
	WorkspaceRole string    `gorm:"type:varchar(50);not null;default:'viewer'" json:"workspace_role"` // workspace_admin, operator, viewer
	CreatedAt     time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`

	Workspace *Workspace `gorm:"foreignKey:WorkspaceID" json:"workspace,omitempty"`
}

func (WorkspaceMember) TableName() string {
	return "workspace_members"
}

// WorkspaceRepository defines data access methods for Workspace aggregates
type WorkspaceRepository interface {
	Create(ctx context.Context, ws *Workspace) error
	GetByID(ctx context.Context, id uuid.UUID) (*Workspace, error)
	GetBySlug(ctx context.Context, orgID uuid.UUID, slug string) (*Workspace, error)
	ListByOrg(ctx context.Context, orgID uuid.UUID) ([]*Workspace, error)
	Update(ctx context.Context, ws *Workspace) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListForUser(ctx context.Context, userID uuid.UUID) ([]*Workspace, error)

	AddMember(ctx context.Context, member *WorkspaceMember) error
	GetMember(ctx context.Context, workspaceID, userID uuid.UUID) (*WorkspaceMember, error)
	ListMembers(ctx context.Context, workspaceID uuid.UUID) ([]*WorkspaceMember, error)
	UpdateMemberRole(ctx context.Context, workspaceID, userID uuid.UUID, role string) error
	RemoveMember(ctx context.Context, workspaceID, userID uuid.UUID) error
}
