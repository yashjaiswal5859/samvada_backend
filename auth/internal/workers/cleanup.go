package workers

import (
	"context"
	"time"

	"github.com/ridesafety/auth-service/internal/domain/pendingauth"
	"github.com/ridesafety/auth-service/internal/infrastructure/postgres/models"
	"github.com/ridesafety/auth-service/internal/infrastructure/postgres/repository"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type CleanupWorker struct {
	db              *gorm.DB
	pendingAuthRepo pendingauth.Repository
	outboxRepo      repository.OutboxRepository
	logger          *zap.Logger
	interval        time.Duration
	retentionPeriod time.Duration
}

func NewCleanupWorker(
	db *gorm.DB,
	pendingAuthRepo pendingauth.Repository,
	outboxRepo repository.OutboxRepository,
	logger *zap.Logger,
	interval time.Duration,
	retentionPeriod time.Duration,
) *CleanupWorker {
	if interval == 0 {
		interval = 15 * time.Minute
	}
	if retentionPeriod == 0 {
		retentionPeriod = 24 * time.Hour
	}
	return &CleanupWorker{
		db:              db,
		pendingAuthRepo: pendingAuthRepo,
		outboxRepo:      outboxRepo,
		logger:          logger,
		interval:        interval,
		retentionPeriod: retentionPeriod,
	}
}

func (w *CleanupWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	w.logger.Info("Cleanup Worker started")

	// Run once immediately on start
	w.runCleanup(ctx)

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Cleanup Worker stopping")
			return
		case <-ticker.C:
			w.runCleanup(ctx)
		}
	}
}

func (w *CleanupWorker) runCleanup(ctx context.Context) {
	w.logger.Info("Running periodic database cleanup...")

	deletedPending, err := w.pendingAuthRepo.DeleteExpired(ctx)
	if err != nil {
		w.logger.Error("Failed to clean expired pending auths", zap.Error(err))
	} else if deletedPending > 0 {
		w.logger.Info("Deleted expired pending auth records", zap.Int64("count", deletedPending))
	}

	deletedOutbox, err := w.outboxRepo.DeletePublishedBefore(ctx, w.retentionPeriod)
	if err != nil {
		w.logger.Error("Failed to clean published outbox events", zap.Error(err))
	} else if deletedOutbox > 0 {
		w.logger.Info("Deleted published outbox records", zap.Int64("count", deletedOutbox))
	}

	now := time.Now().UTC()
	res := w.db.WithContext(ctx).Where("expires_at < ?", now).Delete(&models.IdempotencyKey{})
	if res.Error != nil {
		w.logger.Error("Failed to clean expired idempotency keys", zap.Error(res.Error))
	} else if res.RowsAffected > 0 {
		w.logger.Info("Deleted expired idempotency keys", zap.Int64("count", res.RowsAffected))
	}

	resSess := w.db.WithContext(ctx).Where("expires_at < ? OR used = true", now).Delete(&models.OAuthSession{})
	if resSess.Error != nil {
		w.logger.Error("Failed to clean expired oauth sessions", zap.Error(resSess.Error))
	} else if resSess.RowsAffected > 0 {
		w.logger.Info("Deleted expired oauth sessions", zap.Int64("count", resSess.RowsAffected))
	}
}
