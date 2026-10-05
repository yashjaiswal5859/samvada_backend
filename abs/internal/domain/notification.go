// =============================================================================
// Samvada Platform — ABS Notification Preferences Domain Entity & Interface
// =============================================================================

package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type NotificationPreferences struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID           uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	WorkspaceID      uuid.UUID `gorm:"type:uuid;not null;index" json:"workspace_id"`
	EmailEnabled     bool      `gorm:"default:true" json:"email_enabled"`
	InAppEnabled     bool      `gorm:"default:true" json:"in_app_enabled"`
	SlackEnabled     bool      `gorm:"default:false" json:"slack_enabled"`
	SlackWebhookURL  string    `gorm:"type:text;default:''" json:"slack_webhook_url"`
	CallAlerts       bool      `gorm:"default:true" json:"call_alerts"`
	BillingAlerts    bool      `gorm:"default:true" json:"billing_alerts"`
	CampaignAlerts   bool      `gorm:"default:true" json:"campaign_alerts"`
	QuietHoursStart  string    `gorm:"type:varchar(10);default:'22:00'" json:"quiet_hours_start"`
	QuietHoursEnd    string    `gorm:"type:varchar(10);default:'08:00'" json:"quiet_hours_end"`
	DigestFrequency  string    `gorm:"type:varchar(50);default:'daily'" json:"digest_frequency"`
	CreatedAt        time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (NotificationPreferences) TableName() string {
	return "notification_preferences"
}

type NotificationRepository interface {
	GetPreferences(ctx context.Context, userID, workspaceID uuid.UUID) (*NotificationPreferences, error)
	UpsertPreferences(ctx context.Context, prefs *NotificationPreferences) error
}
