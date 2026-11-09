package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/ridesafety/auth-service/internal/infrastructure/postgres/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OutboxRepository interface {
	FindPending(ctx context.Context, limit int) ([]models.OutboxEvent, error)
	LockEvents(ctx context.Context, ids []uuid.UUID) ([]models.OutboxEvent, error)
	MarkPublished(ctx context.Context, ids []uuid.UUID) error
	MarkFailed(ctx context.Context, id uuid.UUID, lastErr string, nextRetryAt time.Time) error
	DeletePublishedBefore(ctx context.Context, retention time.Duration) (int64, error)
}

type outboxRepository struct {
	db *gorm.DB
}

func NewOutboxRepository(db *gorm.DB) OutboxRepository {
	return &outboxRepository{
		db: db,
	}
}

func (r *outboxRepository) FindPending(ctx context.Context, limit int) ([]models.OutboxEvent, error) {
	var events []models.OutboxEvent
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).
		Where("status = ? AND next_retry_at <= ?", "PENDING", now).
		Order("created_at asc").
		Limit(limit).
		Find(&events).Error
	return events, err
}

func (r *outboxRepository) LockEvents(ctx context.Context, ids []uuid.UUID) ([]models.OutboxEvent, error) {
	var events []models.OutboxEvent
	if len(ids) == 0 {
		return events, nil
	}

	err := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Where("id IN ? AND status = ?", ids, "PENDING").
		Find(&events).Error
	return events, err
}

func (r *outboxRepository) MarkPublished(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	now := time.Now().UTC()
	return r.db.WithContext(ctx).
		Model(&models.OutboxEvent{}).
		Where("id IN ?", ids).
		Updates(map[string]interface{}{
			"status":       "PUBLISHED",
			"published_at": now,
		}).Error
}

func (r *outboxRepository) MarkFailed(ctx context.Context, id uuid.UUID, lastErr string, nextRetryAt time.Time) error {
	return r.db.WithContext(ctx).
		Model(&models.OutboxEvent{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":        "PENDING",
			"retry_count":   gorm.Expr("retry_count + 1"),
			"last_error":    lastErr,
			"next_retry_at": nextRetryAt,
		}).Error
}

func (r *outboxRepository) DeletePublishedBefore(ctx context.Context, retention time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-retention)
	res := r.db.WithContext(ctx).
		Where("status = ? AND published_at < ?", "PUBLISHED", cutoff).
		Delete(&models.OutboxEvent{})
	return res.RowsAffected, res.Error
}
