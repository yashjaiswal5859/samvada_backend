// =============================================================================
// Samvada Platform — ABS Third-Party Connector Integration Domain Entity
// =============================================================================

package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type ConnectorIntegration struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WorkspaceID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"workspace_id"`
	Provider        string     `gorm:"type:varchar(50);not null" json:"provider"` // hubspot, salesforce, zapier, twilio, slack
	Name            string     `gorm:"type:varchar(100);not null" json:"name"`
	IsConnected     bool       `gorm:"default:false" json:"is_connected"`
	CredentialsJSON string     `gorm:"type:jsonb;default:'{}'" json:"credentials_json,omitempty"`
	ConfigJSON      string     `gorm:"type:jsonb;default:'{}'" json:"config_json,omitempty"`
	LastSyncedAt    *time.Time `json:"last_synced_at,omitempty"`
	CreatedAt       time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

func (ConnectorIntegration) TableName() string {
	return "integrations"
}

type ConnectorRepository interface {
	ListByWorkspace(ctx context.Context, workspaceID uuid.UUID) ([]ConnectorIntegration, error)
	GetByProvider(ctx context.Context, workspaceID uuid.UUID, provider string) (*ConnectorIntegration, error)
	GetByID(ctx context.Context, workspaceID, id uuid.UUID) (*ConnectorIntegration, error)
	Upsert(ctx context.Context, integration *ConnectorIntegration) error
	Toggle(ctx context.Context, workspaceID, id uuid.UUID, connected bool) error
	UpdateCredentials(ctx context.Context, workspaceID, id uuid.UUID, credsJSON string) error
}
