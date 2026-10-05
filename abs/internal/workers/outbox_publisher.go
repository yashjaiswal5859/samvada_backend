// =============================================================================
// Samvada Platform — ABS Outbox Event Publisher Worker (Phase 4.3)
// =============================================================================

package workers

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/datatypes"

	"github.com/samvada/abs/internal/domain"
	"github.com/samvada/abs/internal/infrastructure/kafka"
)

type OutboxPublisher struct {
	repo     domain.IntegrationRepository
	producer *kafka.Producer
	pollFreq time.Duration
	stopChan chan struct{}
	log      *zap.Logger
}

func NewOutboxPublisher(
	repo domain.IntegrationRepository,
	producer *kafka.Producer,
	pollFreq time.Duration,
	log *zap.Logger,
) *OutboxPublisher {
	if pollFreq <= 0 {
		pollFreq = 500 * time.Millisecond
	}
	return &OutboxPublisher{
		repo:     repo,
		producer: producer,
		pollFreq: pollFreq,
		stopChan: make(chan struct{}),
		log:      log.Named("outbox_publisher"),
	}
}

func (p *OutboxPublisher) Start(ctx context.Context) {
	ticker := time.NewTicker(p.pollFreq)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-p.stopChan:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.processPendingEvents(ctx)
			}
		}
	}()
	p.log.Info("ABS Outbox Event Publisher started", zap.Duration("poll_interval", p.pollFreq))
}

func (p *OutboxPublisher) Stop() {
	close(p.stopChan)
	p.log.Info("ABS Outbox Event Publisher stopped")
}

func (p *OutboxPublisher) processPendingEvents(ctx context.Context) {
	events, err := p.repo.ListPendingOutboxEvents(ctx, 50)
	if err != nil || len(events) == 0 {
		return
	}

	for _, ev := range events {
		topic := ev.EventType
		partitionKey := ev.AggregateID

		if p.producer != nil {
			_, _, err := p.producer.Publish(topic, partitionKey, []byte(ev.Payload))
			if err != nil {
				p.log.Warn("Failed to publish outbox event to Kafka",
					zap.String("event_id", ev.ID.String()),
					zap.String("topic", topic),
					zap.Error(err),
				)
				_ = p.repo.MarkOutboxFailed(ctx, ev.ID, err.Error())
				continue
			}
		}

		_ = p.repo.MarkOutboxProcessed(ctx, ev.ID)
		p.log.Debug("Outbox event published", zap.String("event_id", ev.ID.String()), zap.String("type", ev.EventType))
	}
}

// QueueEvent helper function to record event in outbox table
func QueueEvent(ctx context.Context, repo domain.IntegrationRepository, eventType, aggregateType, aggregateID string, payload interface{}) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	ev := &domain.OutboxEvent{
		ID:            uuid.New(),
		EventID:       uuid.New(),
		EventType:     eventType,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		Payload:       datatypes.JSON(payloadBytes),
		Status:        "pending",
		RetryCount:    0,
		CreatedAt:     time.Now().UTC(),
	}
	return repo.CreateOutboxEvent(ctx, ev)
}
