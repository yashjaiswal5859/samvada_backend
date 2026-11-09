package eventbus

import (
	"context"
	"time"

	"github.com/ridesafety/auth-service/internal/domain/events"
	"github.com/ridesafety/auth-service/internal/infrastructure/postgres/models"
	"gorm.io/gorm"
)

type txKey struct{}

func WithTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

func GetTx(ctx context.Context) (*gorm.DB, bool) {
	tx, ok := ctx.Value(txKey{}).(*gorm.DB)
	return tx, ok
}

type outboxEventBus struct {
	db *gorm.DB
}

func NewOutboxEventBus(db *gorm.DB) EventBus {
	return &outboxEventBus{
		db: db,
	}
}

func (eb *outboxEventBus) Publish(ctx context.Context, event events.DomainEvent) error {
	tx, ok := ctx.Value(txKey{}).(*gorm.DB)
	if !ok {
		tx = eb.db
	}

	outboxEv := models.OutboxEvent{
		ID:            event.EventID,
		EventType:     string(event.EventType),
		AggregateID:   event.AggregateID,
		Payload:       event.Payload,
		Status:        "PENDING",
		RetryCount:    0,
		NextRetryAt:   time.Now().UTC(),
		CorrelationID: event.CorrelationID,
		CreatedAt:     event.OccurredAt,
	}

	return tx.WithContext(ctx).Create(&outboxEv).Error
}
