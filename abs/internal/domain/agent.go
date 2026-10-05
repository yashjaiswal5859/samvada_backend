// =============================================================================
// Samvada Platform — ABS Agent & Version Domain Entity & Repository
// =============================================================================

package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type Agent struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WorkspaceID      uuid.UUID `gorm:"type:uuid;not null;index:idx_agents_workspace_id" json:"workspace_id"`
	Name             string    `gorm:"type:varchar(255);not null" json:"name"`
	Role             string    `gorm:"type:varchar(255);not null" json:"role"`
	Description      *string   `gorm:"type:text" json:"description,omitempty"`
	AvatarURL        *string   `gorm:"type:text" json:"avatar_url,omitempty"`
	SystemPrompt     string    `gorm:"type:text;not null" json:"system_prompt"`
	VoiceID          string    `gorm:"type:varchar(100);not null" json:"voice_id"`
	VoiceProvider    string    `gorm:"type:varchar(50);default:'sarvam'" json:"voice_provider"`
	VoiceName        string    `gorm:"type:varchar(100);not null" json:"voice_name"`
	Language         string    `gorm:"type:varchar(50);default:'en-US'" json:"language"`
	Model            string    `gorm:"type:varchar(100);default:'gemini-2.5-flash'" json:"model"`
	Status           string    `gorm:"type:varchar(50);default:'draft'" json:"status"` // draft, active, archived
	ActiveVersionNum int       `gorm:"default:1" json:"active_version_num"`
	AssignedPhone    *string   `gorm:"type:varchar(50)" json:"assigned_phone,omitempty"`
	CreatedAt        time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt        time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`

	// Associations
	Workspace *Workspace     `gorm:"foreignKey:WorkspaceID" json:"workspace,omitempty"`
	Versions  []AgentVersion `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE" json:"versions,omitempty"`
}

func (Agent) TableName() string {
	return "agents"
}

type AgentVersion struct {
	ID            uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	AgentID       uuid.UUID      `gorm:"type:uuid;not null;index:idx_agent_versions_agent_id" json:"agent_id"`
	VersionNum    int            `gorm:"not null" json:"version_num"`
	SystemPrompt  string         `gorm:"type:text;not null" json:"system_prompt"`
	Model         string         `gorm:"type:varchar(100);not null" json:"model"`
	VoiceID       string         `gorm:"type:varchar(100);not null" json:"voice_id"`
	Language      string         `gorm:"type:varchar(50);not null" json:"language"`
	ToolsConfig   datatypes.JSON `gorm:"type:jsonb;default:'[]'" json:"tools_config"`
	PublishedBy   uuid.UUID      `gorm:"type:uuid;not null" json:"published_by"`
	ChangeSummary *string        `gorm:"type:text" json:"change_summary,omitempty"`
	CreatedAt     time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`

	Agent *Agent `gorm:"foreignKey:AgentID" json:"agent,omitempty"`
}

func (AgentVersion) TableName() string {
	return "agent_versions"
}

// AgentRepository defines data access methods for Agent aggregates
type AgentRepository interface {
	Create(ctx context.Context, agent *Agent) error
	GetByID(ctx context.Context, workspaceID, id uuid.UUID) (*Agent, error)
	ListByWorkspace(ctx context.Context, workspaceID uuid.UUID) ([]*Agent, error)
	Update(ctx context.Context, agent *Agent) error
	Delete(ctx context.Context, workspaceID, id uuid.UUID) error
	CountByWorkspace(ctx context.Context, workspaceID uuid.UUID) (int64, error)

	CreateVersion(ctx context.Context, version *AgentVersion) error
	GetVersion(ctx context.Context, agentID uuid.UUID, versionNum int) (*AgentVersion, error)
	ListVersions(ctx context.Context, agentID uuid.UUID) ([]*AgentVersion, error)
	GetLatestVersion(ctx context.Context, agentID uuid.UUID) (*AgentVersion, error)
}
