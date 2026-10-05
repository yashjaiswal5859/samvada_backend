// =============================================================================
// Samvada Platform — PostgreSQL Notification Repository Implementation
// =============================================================================

package repos

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/samvada/abs/internal/domain"
)

type NotificationRepository struct {
	db *gorm.DB
}

func NewNotificationRepository(db *gorm.DB) *NotificationRepository {
	return &NotificationRepository{db: db}
}

func (r *NotificationRepository) GetPreferences(ctx context.Context, userID, workspaceID uuid.UUID) (*domain.NotificationPreferences, error) {
	var prefs domain.NotificationPreferences
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND workspace_id = ?", userID, workspaceID).
		First(&prefs).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &prefs, nil
}

func (r *NotificationRepository) UpsertPreferences(ctx context.Context, prefs *domain.NotificationPreferences) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "workspace_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"email_enabled", "in_app_enabled", "slack_enabled", "slack_webhook_url",
			"call_alerts", "billing_alerts", "campaign_alerts",
			"quiet_hours_start", "quiet_hours_end", "digest_frequency", "updated_at",
		}),
	}).Create(prefs).Error
}
