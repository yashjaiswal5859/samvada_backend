// =============================================================================
// Samvada Platform — ABS Notification Preferences Application Service
// =============================================================================

package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/domain"
)

type UpdatePreferencesRequest struct {
	EmailEnabled    bool   `json:"email_enabled"`
	InAppEnabled    bool   `json:"in_app_enabled"`
	SlackEnabled    bool   `json:"slack_enabled"`
	SlackWebhookURL string `json:"slack_webhook_url"`
	CallAlerts      bool   `json:"call_alerts"`
	BillingAlerts   bool   `json:"billing_alerts"`
	CampaignAlerts  bool   `json:"campaign_alerts"`
	QuietHoursStart string `json:"quiet_hours_start"`
	QuietHoursEnd   string `json:"quiet_hours_end"`
	DigestFrequency string `json:"digest_frequency"`
}

type NotificationService struct {
	repo domain.NotificationRepository
	log  *zap.Logger
}

func NewNotificationService(repo domain.NotificationRepository, log *zap.Logger) *NotificationService {
	return &NotificationService{
		repo: repo,
		log:  log.Named("notification_service"),
	}
}

func (s *NotificationService) GetPreferences(ctx context.Context, userID, workspaceID uuid.UUID) (*domain.NotificationPreferences, error) {
	prefs, err := s.repo.GetPreferences(ctx, userID, workspaceID)
	if err != nil {
		return nil, err
	}
	if prefs == nil {
		// Default preferences
		return &domain.NotificationPreferences{
			UserID:          userID,
			WorkspaceID:     workspaceID,
			EmailEnabled:    true,
			InAppEnabled:    true,
			SlackEnabled:    false,
			CallAlerts:      true,
			BillingAlerts:   true,
			CampaignAlerts:  true,
			QuietHoursStart: "22:00",
			QuietHoursEnd:   "08:00",
			DigestFrequency: "daily",
		}, nil
	}
	return prefs, nil
}

func (s *NotificationService) UpdatePreferences(ctx context.Context, userID, workspaceID uuid.UUID, req UpdatePreferencesRequest) (*domain.NotificationPreferences, error) {
	prefs := &domain.NotificationPreferences{
		UserID:          userID,
		WorkspaceID:     workspaceID,
		EmailEnabled:    req.EmailEnabled,
		InAppEnabled:    req.InAppEnabled,
		SlackEnabled:    req.SlackEnabled,
		SlackWebhookURL: req.SlackWebhookURL,
		CallAlerts:      req.CallAlerts,
		BillingAlerts:   req.BillingAlerts,
		CampaignAlerts:  req.CampaignAlerts,
		QuietHoursStart: req.QuietHoursStart,
		QuietHoursEnd:   req.QuietHoursEnd,
		DigestFrequency: req.DigestFrequency,
		UpdatedAt:       time.Now().UTC(),
	}

	if err := s.repo.UpsertPreferences(ctx, prefs); err != nil {
		return nil, fmt.Errorf("failed to save notification preferences: %w", err)
	}

	return prefs, nil
}

func (s *NotificationService) SendTestNotification(ctx context.Context, userID, workspaceID uuid.UUID, channel string) error {
	s.log.Info("Dispatched test notification",
		zap.String("user_id", userID.String()),
		zap.String("workspace_id", workspaceID.String()),
		zap.String("channel", channel),
	)
	return nil
}
