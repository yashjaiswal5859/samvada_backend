// =============================================================================
// Samvada Platform — ABS Webhook, API Key & Outbox Domain Entity & Repository
// =============================================================================

package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/datatypes"
)

type APIKey struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WorkspaceID  uuid.UUID      `gorm:"type:uuid;not null;index:idx_api_keys_workspace" json:"workspace_id"`
	Name         string         `gorm:"type:varchar(255);not null" json:"name"`
	KeyPrefix    string         `gorm:"column:key_prefix;type:varchar(32);not null;index:idx_api_keys_prefix" json:"key_prefix"`
	HashedSecret string         `gorm:"column:hashed_secret;type:varchar(255);not null" json:"-"`
	Scopes       pq.StringArray `gorm:"type:text[];default:'{\"calls:read\", \"calls:write\", \"agents:read\"}'" json:"scopes"`
	LastUsedAt   *time.Time     `json:"last_used_at,omitempty"`
	LastUsedIP   *string        `gorm:"column:last_used_ip;type:varchar(50)" json:"last_used_ip,omitempty"`
	ExpiresAt    *time.Time     `json:"expires_at,omitempty"`
	CreatedAt    time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
}

func (APIKey) TableName() string {
	return "api_keys"
}

type Webhook struct {
	ID                  uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WorkspaceID         uuid.UUID      `gorm:"type:uuid;not null;index:idx_webhooks_workspace" json:"workspace_id"`
	URL                 string         `gorm:"type:text;not null" json:"url"`
	SigningSecret       string         `gorm:"column:signing_secret;type:varchar(255);not null" json:"-"`
	Events              pq.StringArray `gorm:"type:text[];not null" json:"events"`
	IsActive            bool           `gorm:"not null;default:true" json:"is_active"`
	ConsecutiveFailures int            `gorm:"column:consecutive_failures;not null;default:0" json:"consecutive_failures"`
	CreatedAt           time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
}

func (Webhook) TableName() string {
	return "webhooks"
}

type OutboxEvent struct {
	ID            uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	EventID       uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex" json:"event_id"`
	EventType     string         `gorm:"type:varchar(100);not null" json:"event_type"`
	AggregateType string         `gorm:"type:varchar(50);not null" json:"aggregate_type"`
	AggregateID   string         `gorm:"type:varchar(255);not null" json:"aggregate_id"`
	Payload       datatypes.JSON `gorm:"type:jsonb;not null" json:"payload"`
	Status        string         `gorm:"type:varchar(50);not null;default:'pending'" json:"status"`
	RetryCount    int            `gorm:"not null;default:0" json:"retry_count"`
	LastError     *string        `gorm:"type:text" json:"last_error,omitempty"`
	ProcessedAt   *time.Time     `json:"processed_at,omitempty"`
	CreatedAt     time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
}

func (OutboxEvent) TableName() string {
	return "outbox_events"
}

// IntegrationRepository defines data access methods for APIKeys, Webhooks and Outbox
type IntegrationRepository interface {
	CreateAPIKey(ctx context.Context, key *APIKey) error
	GetAPIKeyByHash(ctx context.Context, hash string) (*APIKey, error)
	ListAPIKeys(ctx context.Context, workspaceID uuid.UUID) ([]*APIKey, error)
	DeleteAPIKey(ctx context.Context, workspaceID, id uuid.UUID) error

	CreateWebhook(ctx context.Context, wh *Webhook) error
	GetWebhookByID(ctx context.Context, workspaceID, id uuid.UUID) (*Webhook, error)
	ListWebhooks(ctx context.Context, workspaceID uuid.UUID) ([]*Webhook, error)
	UpdateWebhook(ctx context.Context, wh *Webhook) error
	DeleteWebhook(ctx context.Context, workspaceID, id uuid.UUID) error

	CreateOutboxEvent(ctx context.Context, evt *OutboxEvent) error
	ListPendingOutboxEvents(ctx context.Context, limit int) ([]*OutboxEvent, error)
	MarkOutboxProcessed(ctx context.Context, id uuid.UUID) error
	MarkOutboxFailed(ctx context.Context, id uuid.UUID, errMsg string) error
}
