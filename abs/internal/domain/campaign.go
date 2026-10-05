// =============================================================================
// Samvada Platform — ABS Campaign, Audience & Contact Domain Entity & Repository
// =============================================================================

package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type Audience struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WorkspaceID  uuid.UUID `gorm:"type:uuid;not null;index:idx_audiences_workspace_id" json:"workspace_id"`
	Name         string    `gorm:"type:varchar(255);not null" json:"name"`
	Description  *string   `gorm:"type:text" json:"description,omitempty"`
	ContactCount int       `gorm:"not null;default:0" json:"contact_count"`
	CreatedAt    time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`

	// Associations
	Contacts []Contact `gorm:"foreignKey:AudienceID;constraint:OnDelete:CASCADE" json:"contacts,omitempty"`
}

func (Audience) TableName() string {
	return "audiences"
}

type Contact struct {
	ID              uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	AudienceID      uuid.UUID      `gorm:"type:uuid;not null;index:idx_contacts_audience_id" json:"audience_id"`
	Name            string         `gorm:"type:varchar(255);not null" json:"name"`
	Phone           string         `gorm:"type:varchar(50);not null" json:"phone"`
	Email           *string        `gorm:"type:varchar(255)" json:"email,omitempty"`
	Company         *string        `gorm:"type:varchar(255)" json:"company,omitempty"`
	CustomVariables datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"custom_variables"`
	Status          string         `gorm:"type:varchar(50);not null;default:'active'" json:"status"` // active, do_not_call, invalid
	LastCalledAt    *time.Time     `json:"last_called_at,omitempty"`
	CreatedAt       time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
}

func (Contact) TableName() string {
	return "contacts"
}

type Campaign struct {
	ID                   uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WorkspaceID          uuid.UUID  `gorm:"type:uuid;not null;index:idx_campaigns_workspace_id" json:"workspace_id"`
	AgentID              uuid.UUID  `gorm:"type:uuid;not null" json:"agent_id"`
	AudienceID           uuid.UUID  `gorm:"type:uuid;not null" json:"audience_id"`
	Name                 string     `gorm:"type:varchar(255);not null" json:"name"`
	Status               string     `gorm:"type:varchar(50);not null;default:'scheduled'" json:"status"` // draft, scheduled, running, paused, completed, cancelled
	ConcurrentCallsLimit int        `gorm:"not null;default:10" json:"concurrent_calls_limit"`
	ScheduledAt          *time.Time `json:"scheduled_at,omitempty"`
	StartedAt            *time.Time `json:"started_at,omitempty"`
	CompletedAt          *time.Time `json:"completed_at,omitempty"`
	TotalContacts        int        `gorm:"not null;default:0" json:"total_contacts"`
	CompletedCalls       int        `gorm:"not null;default:0" json:"completed_calls"`
	SuccessfulCalls      int        `gorm:"not null;default:0" json:"successful_calls"`
	FailedCalls          int        `gorm:"not null;default:0" json:"failed_calls"`
	CreatedAt            time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt            time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`

	// Associations
	Agent    *Agent    `gorm:"foreignKey:AgentID" json:"agent,omitempty"`
	Audience *Audience `gorm:"foreignKey:AudienceID" json:"audience,omitempty"`
}

func (Campaign) TableName() string {
	return "campaigns"
}

// CampaignRepository defines data access methods for Campaign aggregates
type CampaignRepository interface {
	CreateAudience(ctx context.Context, aud *Audience) error
	GetAudienceByID(ctx context.Context, workspaceID, id uuid.UUID) (*Audience, error)
	ListAudiences(ctx context.Context, workspaceID uuid.UUID) ([]*Audience, error)
	DeleteAudience(ctx context.Context, workspaceID, id uuid.UUID) error
	AddContacts(ctx context.Context, contacts []*Contact) error
	ListContacts(ctx context.Context, audienceID uuid.UUID, limit, offset int) ([]*Contact, int64, error)
	DeleteContact(ctx context.Context, audienceID, contactID uuid.UUID) error
	GetNextPendingContacts(ctx context.Context, audienceID uuid.UUID, limit int) ([]*Contact, error)
	UpdateContactLastCalled(ctx context.Context, contactID uuid.UUID, status string) error

	CreateCampaign(ctx context.Context, cmp *Campaign) error
	GetCampaignByID(ctx context.Context, workspaceID, id uuid.UUID) (*Campaign, error)
	ListCampaigns(ctx context.Context, workspaceID uuid.UUID) ([]*Campaign, error)
	ListRunningCampaigns(ctx context.Context) ([]*Campaign, error)
	UpdateCampaignStatus(ctx context.Context, workspaceID, id uuid.UUID, status string) error
	IncrementCampaignStats(ctx context.Context, campaignID uuid.UUID, completed, successful, failed int) error
	DeleteCampaign(ctx context.Context, workspaceID, id uuid.UUID) error
}

type CampaignDispatchEvent struct {
	EventID     uuid.UUID `json:"event_id"`
	CampaignID  uuid.UUID `json:"campaign_id"`
	AgentID     uuid.UUID `json:"agent_id"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	ContactID   uuid.UUID `json:"contact_id"`
	PhoneNumber string    `json:"phone_number"`
	Timestamp   time.Time `json:"timestamp"`
}
