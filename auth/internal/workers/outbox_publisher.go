package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/ridesafety/auth-service/internal/domain/events"
	"github.com/ridesafety/auth-service/internal/infrastructure/kafka"
	"github.com/ridesafety/auth-service/internal/infrastructure/postgres/models"
	"github.com/ridesafety/auth-service/internal/infrastructure/postgres/repository"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type OutboxPublisher struct {
	db              *gorm.DB
	outboxRepo      repository.OutboxRepository
	kafkaProducer   kafka.Producer
	logger          *zap.Logger
	topicOTP              string
	topicRegister         string
	topicLogin            string
	topicLogout           string
	topicTenantProvision  string
	backoffSchedule       []time.Duration
}

func NewOutboxPublisher(
	db *gorm.DB,
	outboxRepo repository.OutboxRepository,
	kafkaProducer kafka.Producer,
	logger *zap.Logger,
	topicOTP string,
	topicRegister string,
	topicLogin string,
	topicLogout string,
	topicTenantProvision string,
) *OutboxPublisher {
	if topicTenantProvision == "" {
		topicTenantProvision = "auth.onboarding.completed"
	}
	return &OutboxPublisher{
		db:                   db,
		outboxRepo:           outboxRepo,
		kafkaProducer:        kafkaProducer,
		logger:               logger,
		topicOTP:             topicOTP,
		topicRegister:        topicRegister,
		topicLogin:           topicLogin,
		topicLogout:          topicLogout,
		topicTenantProvision: topicTenantProvision,
		backoffSchedule: []time.Duration{
			1 * time.Second,
			2 * time.Second,
			5 * time.Second,
			10 * time.Second,
			30 * time.Second,
			60 * time.Second,
		},
	}
}

func (p *OutboxPublisher) Start(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	p.logger.Info("Outbox Publisher worker started")

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("Outbox Publisher worker stopping")
			return
		case <-ticker.C:
			p.processPendingEvents(ctx)
		}
	}
}

func (p *OutboxPublisher) processPendingEvents(ctx context.Context) {
	// 1. Fetch pending outbox events (limit 20)
	dbEvents, err := p.outboxRepo.FindPending(ctx, 20)
	if err != nil {
		p.logger.Error("Failed to fetch pending outbox events", zap.Error(err))
		return
	}

	if len(dbEvents) == 0 {
		return
	}

	for _, dbEv := range dbEvents {
		p.processSingleEvent(ctx, dbEv)
	}
}

func (p *OutboxPublisher) processSingleEvent(ctx context.Context, dbEv models.OutboxEvent) {
	// Start transaction and lock the row to avoid duplicate processing in case of multiple replicas
	tx := p.db.Begin()
	if tx.Error != nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	txRepo := repository.NewOutboxRepository(tx)
	locked, err := txRepo.LockEvents(ctx, []uuid.UUID{dbEv.ID})
	if err != nil {
		tx.Rollback()
		return
	}

	if len(locked) == 0 {
		// Event already locked/processing by another worker
		tx.Rollback()
		return
	}

	// 2. Map event to topic
	var topic string
	switch events.EventType(dbEv.EventType) {
	case events.OTPRequested:
		topic = p.topicOTP
	case events.UserRegistered:
		topic = p.topicRegister
	case events.UserLoggedIn:
		topic = p.topicLogin
	case events.UserLoggedOut:
		topic = p.topicLogout
	case events.TenantProvisionRequested, events.UserOnboarded:
		topic = p.topicTenantProvision
	default:
		p.logger.Warn("Unknown event type, skipping", zap.String("type", dbEv.EventType))
		tx.Rollback()
		return
	}

	// Build the Kafka message (envelope as per PRD)
	envelope := map[string]interface{}{
		"event_id":       dbEv.ID.String(),
		"event_type":     dbEv.EventType,
		"aggregate_id":   dbEv.AggregateID,
		"aggregate_type": "USER",
		"version":        1,
		"occurred_at":    dbEv.CreatedAt,
		"correlation_id": dbEv.CorrelationID,
		"payload":        dbEv.Payload,
	}

	envBytes, err := json.Marshal(envelope)
	if err != nil {
		p.logger.Error("Failed to marshal event envelope", zap.Error(err))
		tx.Rollback()
		return
	}

	// 3. Publish to Kafka
	_, _, err = p.kafkaProducer.SendMessage(ctx, topic, dbEv.AggregateID, envBytes)
	if err != nil {
		// Handle failure and retry/DLQ logic
		p.logger.Error("Failed to publish event to Kafka", zap.String("event_id", dbEv.ID.String()), zap.Error(err))

		retryCount := dbEv.RetryCount
		if retryCount >= len(p.backoffSchedule) {
			// Max retries exceeded, send to DLQ
			dlqTopic := fmt.Sprintf("%s.dlq", topic)
			p.logger.Warn("Max retries exceeded, sending event to DLQ", zap.String("event_id", dbEv.ID.String()), zap.String("dlq", dlqTopic))
			_, _, dlqErr := p.kafkaProducer.SendMessage(ctx, dlqTopic, dbEv.AggregateID, envBytes)

			var lastErr string
			if dlqErr != nil {
				lastErr = fmt.Sprintf("Kafka publish err: %v; DLQ err: %v", err, dlqErr)
			} else {
				lastErr = fmt.Sprintf("Kafka publish err: %v; Sent to DLQ", err)
			}

			// Mark event status as FAILED
			_ = tx.Model(&models.OutboxEvent{}).Where("id = ?", dbEv.ID).Updates(map[string]interface{}{
				"status":     "FAILED",
				"last_error": lastErr,
			}).Error
		} else {
			// Schedule retry
			backoff := p.backoffSchedule[retryCount]
			nextRetry := time.Now().UTC().Add(backoff)
			lastErrStr := err.Error()

			_ = txRepo.MarkFailed(ctx, dbEv.ID, lastErrStr, nextRetry)
		}

		_ = tx.Commit().Error
		return
	}

	// 4. Mark published
	if err := txRepo.MarkPublished(ctx, []uuid.UUID{dbEv.ID}); err != nil {
		p.logger.Error("Failed to mark outbox event as published", zap.Error(err))
		tx.Rollback()
		return
	}

	if err := tx.Commit().Error; err != nil {
		p.logger.Error("Failed to commit outbox event status change", zap.Error(err))
	}
}
