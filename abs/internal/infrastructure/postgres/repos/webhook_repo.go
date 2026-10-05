// =============================================================================
// Samvada Platform — ABS Integration Repository (GORM PostgreSQL)
// =============================================================================

package repos

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/samvada/abs/internal/domain"
)

type integrationRepo struct {
	db *gorm.DB
}

func NewIntegrationRepository(db *gorm.DB) domain.IntegrationRepository {
	return &integrationRepo{db: db}
}

func (r *integrationRepo) CreateAPIKey(ctx context.Context, key *domain.APIKey) error {
	if key.ID == uuid.Nil {
		key.ID = uuid.New()
	}
	return r.db.WithContext(ctx).Create(key).Error
}

func (r *integrationRepo) GetAPIKeyByHash(ctx context.Context, hash string) (*domain.APIKey, error) {
	var key domain.APIKey
	if err := r.db.WithContext(ctx).First(&key, "hashed_secret = ?", hash).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrAPIKeyNotFound
		}
		return nil, err
	}
	return &key, nil
}

func (r *integrationRepo) ListAPIKeys(ctx context.Context, workspaceID uuid.UUID) ([]*domain.APIKey, error) {
	var keys []*domain.APIKey
	err := r.db.WithContext(ctx).
		Where("workspace_id = ?", workspaceID).
		Order("created_at DESC").
		Find(&keys).Error
	return keys, err
}

func (r *integrationRepo) DeleteAPIKey(ctx context.Context, workspaceID, id uuid.UUID) error {
	res := r.db.WithContext(ctx).
		Delete(&domain.APIKey{}, "id = ? AND workspace_id = ?", id, workspaceID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrAPIKeyNotFound
	}
	return nil
}

func (r *integrationRepo) CreateWebhook(ctx context.Context, wh *domain.Webhook) error {
	if wh.ID == uuid.Nil {
		wh.ID = uuid.New()
	}
	return r.db.WithContext(ctx).Create(wh).Error
}

func (r *integrationRepo) GetWebhookByID(ctx context.Context, workspaceID, id uuid.UUID) (*domain.Webhook, error) {
	var wh domain.Webhook
	if err := r.db.WithContext(ctx).First(&wh, "id = ? AND workspace_id = ?", id, workspaceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrWebhookNotFound
		}
		return nil, err
	}
	return &wh, nil
}

func (r *integrationRepo) ListWebhooks(ctx context.Context, workspaceID uuid.UUID) ([]*domain.Webhook, error) {
	var webhooks []*domain.Webhook
	err := r.db.WithContext(ctx).
		Where("workspace_id = ?", workspaceID).
		Order("created_at DESC").
		Find(&webhooks).Error
	return webhooks, err
}

func (r *integrationRepo) UpdateWebhook(ctx context.Context, wh *domain.Webhook) error {
	return r.db.WithContext(ctx).Save(wh).Error
}

func (r *integrationRepo) DeleteWebhook(ctx context.Context, workspaceID, id uuid.UUID) error {
	res := r.db.WithContext(ctx).
		Delete(&domain.Webhook{}, "id = ? AND workspace_id = ?", id, workspaceID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrWebhookNotFound
	}
	return nil
}

func (r *integrationRepo) CreateOutboxEvent(ctx context.Context, evt *domain.OutboxEvent) error {
	if evt.ID == uuid.Nil {
		evt.ID = uuid.New()
	}
	return r.db.WithContext(ctx).Create(evt).Error
}

func (r *integrationRepo) ListPendingOutboxEvents(ctx context.Context, limit int) ([]*domain.OutboxEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	var events []*domain.OutboxEvent
	err := r.db.WithContext(ctx).
		Where("status = ?", "pending").
		Order("created_at ASC").
		Limit(limit).
		Find(&events).Error
	return events, err
}

func (r *integrationRepo) MarkOutboxProcessed(ctx context.Context, id uuid.UUID) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).
		Model(&domain.OutboxEvent{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":       "published",
			"processed_at": &now,
		}).Error
}

func (r *integrationRepo) MarkOutboxFailed(ctx context.Context, id uuid.UUID, errMsg string) error {
	return r.db.WithContext(ctx).
		Model(&domain.OutboxEvent{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":      "failed",
			"last_error":  errMsg,
			"retry_count": gorm.Expr("retry_count + 1"),
		}).Error
}
