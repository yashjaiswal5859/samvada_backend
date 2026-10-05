// =============================================================================
// Samvada Platform — ABS Telephony & Phone Numbers Domain Entity & Repository
// =============================================================================

package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type PhoneNumber struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WorkspaceID     uuid.UUID  `gorm:"type:uuid;not null;index:idx_phone_numbers_workspace" json:"workspace_id"`
	PhoneNumber     string     `gorm:"type:varchar(50);uniqueIndex;not null" json:"phone_number"`
	Type            string     `gorm:"type:varchar(50);default:'toll_free'" json:"type"` // toll_free, local, mobile
	Provider        string     `gorm:"type:varchar(50);not null" json:"provider"`        // twilio, plivo, sip_trunk
	CarrierSID      *string    `gorm:"column:carrier_sid;type:varchar(255)" json:"carrier_sid,omitempty"`
	AssignedAgentID *uuid.UUID `gorm:"type:uuid;index:idx_phone_numbers_assigned_agent" json:"assigned_agent_id,omitempty"`
	Status          string     `gorm:"type:varchar(50);default:'active'" json:"status"` // active, released, pending_verification
	MonthlyCostUSD  float64    `gorm:"type:numeric(10,4);default:1.00" json:"monthly_cost_usd"`
	CreatedAt       time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`

	// Associations
	AssignedAgent *Agent `gorm:"foreignKey:AssignedAgentID" json:"assigned_agent,omitempty"`
}

func (PhoneNumber) TableName() string {
	return "phone_numbers"
}

// TelephonyRepository defines data access methods for Telephony aggregates
type TelephonyRepository interface {
	Create(ctx context.Context, pn *PhoneNumber) error
	GetByID(ctx context.Context, workspaceID, id uuid.UUID) (*PhoneNumber, error)
	GetByNumber(ctx context.Context, number string) (*PhoneNumber, error)
	ListByWorkspace(ctx context.Context, workspaceID uuid.UUID) ([]*PhoneNumber, error)
	AssignAgent(ctx context.Context, workspaceID, id, agentID uuid.UUID) error
	UnassignAgent(ctx context.Context, workspaceID, id uuid.UUID) error
	Delete(ctx context.Context, workspaceID, id uuid.UUID) error
}
