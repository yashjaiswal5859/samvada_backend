// =============================================================================
// Samvada Platform — Reporting Engine High-Throughput Kafka Ingestion Worker
// =============================================================================

package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/reporting/configs"
	"github.com/samvada/reporting/internal/domain"
	"github.com/samvada/reporting/internal/infrastructure/redis"
)

type IngestionWorker struct {
	consumerGroup sarama.ConsumerGroup
	callRepo      domain.CallRepository
	redisClient   *redis.Client
	cfg           *configs.KafkaConfig
	batchCfg      *configs.BatchConfig
	log           *zap.Logger

	mu           sync.Mutex
	eventBuffer  []*domain.CallEvent
	turnBuffer   []*domain.SentimentTurn
	flushTicker  *time.Ticker
	stopChan     chan struct{}
}

func NewIngestionWorker(
	brokers []string,
	groupName string,
	callRepo domain.CallRepository,
	redisClient *redis.Client,
	kafkaCfg *configs.KafkaConfig,
	batchCfg *configs.BatchConfig,
	log *zap.Logger,
) (*IngestionWorker, error) {
	config := sarama.NewConfig()
	config.Version = sarama.V3_5_0_0
	config.Consumer.Offsets.Initial = sarama.OffsetNewest
	config.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategyRoundRobin()}

	group, err := sarama.NewConsumerGroup(brokers, groupName, config)
	if err != nil {
		return nil, fmt.Errorf("creating sarama consumer group: %w", err)
	}

	flushInterval := 5 * time.Second
	if batchCfg != nil && batchCfg.FlushIntervalMS > 0 {
		flushInterval = time.Duration(batchCfg.FlushIntervalMS) * time.Millisecond
	}

	return &IngestionWorker{
		consumerGroup: group,
		callRepo:      callRepo,
		redisClient:   redisClient,
		cfg:           kafkaCfg,
		batchCfg:      batchCfg,
		log:           log.Named("kafka_ingestion_worker"),
		eventBuffer:   make([]*domain.CallEvent, 0, 1000),
		turnBuffer:    make([]*domain.SentimentTurn, 0, 2000),
		flushTicker:   time.NewTicker(flushInterval),
		stopChan:      make(chan struct{}),
	}, nil
}

func (w *IngestionWorker) Start(ctx context.Context) {
	topics := []string{
		w.cfg.Topics.CallStarted,
		w.cfg.Topics.CallCompleted,
		w.cfg.Topics.CallTurns,
	}

	// 1. Start periodic flusher loop
	go w.periodicFlusher(ctx)

	// 2. Start consumer loop
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-w.stopChan:
				return
			default:
				if err := w.consumerGroup.Consume(ctx, topics, w); err != nil {
					w.log.Warn("Kafka consumer group error (reconnecting)", zap.Error(err))
					time.Sleep(2 * time.Second)
				}
			}
		}
	}()

	w.log.Info("Kafka ingestion worker started successfully", zap.Strings("topics", topics))
}

func (w *IngestionWorker) Setup(_ sarama.ConsumerGroupSession) error   { return nil }
func (w *IngestionWorker) Cleanup(_ sarama.ConsumerGroupSession) error { return nil }

func (w *IngestionWorker) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}
			w.handleMessage(session.Context(), msg)
			session.MarkMessage(msg, "")
		case <-session.Context().Done():
			return nil
		}
	}
}

func (w *IngestionWorker) handleMessage(ctx context.Context, msg *sarama.ConsumerMessage) {
	switch msg.Topic {
	case w.cfg.Topics.CallStarted:
		w.handleCallStarted(ctx, msg.Value)
	case w.cfg.Topics.CallCompleted:
		w.handleCallCompleted(ctx, msg.Value)
	case w.cfg.Topics.CallTurns:
		w.handleCallTurn(ctx, msg.Value)
	default:
		w.log.Debug("Unknown topic received", zap.String("topic", msg.Topic))
	}
}

type CallStartedPayload struct {
	CallID      string `json:"call_id"`
	OrgID       string `json:"org_id"`
	WorkspaceID string `json:"workspace_id"`
}

func (w *IngestionWorker) handleCallStarted(ctx context.Context, data []byte) {
	var payload CallStartedPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		w.log.Warn("Failed to unmarshal call.started event", zap.Error(err))
		return
	}

	if w.redisClient != nil && payload.OrgID != "" {
		orgKey := fmt.Sprintf("samvada:live_calls:org:%s", payload.OrgID)
		_, _ = w.redisClient.Incr(ctx, orgKey)
		if payload.WorkspaceID != "" {
			wsKey := fmt.Sprintf("samvada:live_calls:ws:%s", payload.WorkspaceID)
			_, _ = w.redisClient.Incr(ctx, wsKey)
		}
	}
}

func (w *IngestionWorker) handleCallCompleted(ctx context.Context, data []byte) {
	var ev domain.CallEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		w.log.Warn("Failed to unmarshal call.completed event", zap.Error(err))
		return
	}

	if ev.CallID == uuid.Nil {
		ev.CallID = uuid.New()
	}
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Now().UTC()
	}

	if w.redisClient != nil && ev.OrgID != uuid.Nil {
		orgKey := fmt.Sprintf("samvada:live_calls:org:%s", ev.OrgID.String())
		_, _ = w.redisClient.Decr(ctx, orgKey)
		if ev.WorkspaceID != uuid.Nil {
			wsKey := fmt.Sprintf("samvada:live_calls:ws:%s", ev.WorkspaceID.String())
			_, _ = w.redisClient.Decr(ctx, wsKey)
		}
	}

	w.mu.Lock()
	w.eventBuffer = append(w.eventBuffer, &ev)
	shouldFlush := len(w.eventBuffer) >= 1000
	w.mu.Unlock()

	if shouldFlush {
		w.FlushEvents(ctx)
	}
}

func (w *IngestionWorker) handleCallTurn(ctx context.Context, data []byte) {
	var turn domain.SentimentTurn
	if err := json.Unmarshal(data, &turn); err != nil {
		w.log.Warn("Failed to unmarshal call.turn event", zap.Error(err))
		return
	}

	if turn.Timestamp.IsZero() {
		turn.Timestamp = time.Now().UTC()
	}

	w.mu.Lock()
	w.turnBuffer = append(w.turnBuffer, &turn)
	shouldFlush := len(w.turnBuffer) >= 1000
	w.mu.Unlock()

	if shouldFlush {
		w.FlushTurns(ctx)
	}
}

func (w *IngestionWorker) periodicFlusher(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			w.FlushAll(context.Background())
			return
		case <-w.stopChan:
			w.FlushAll(context.Background())
			return
		case <-w.flushTicker.C:
			w.FlushAll(ctx)
		}
	}
}

func (w *IngestionWorker) FlushAll(ctx context.Context) {
	w.FlushEvents(ctx)
	w.FlushTurns(ctx)
}

func (w *IngestionWorker) FlushEvents(ctx context.Context) {
	w.mu.Lock()
	if len(w.eventBuffer) == 0 {
		w.mu.Unlock()
		return
	}
	eventsToFlush := w.eventBuffer
	w.eventBuffer = make([]*domain.CallEvent, 0, 1000)
	w.mu.Unlock()

	if err := w.callRepo.BatchInsertCallEvents(ctx, eventsToFlush); err != nil {
		w.log.Error("Failed to flush call events batch to ClickHouse",
			zap.Int("count", len(eventsToFlush)),
			zap.Error(err),
		)
		// Re-queue events on error
		w.mu.Lock()
		w.eventBuffer = append(eventsToFlush, w.eventBuffer...)
		w.mu.Unlock()
		return
	}

	w.log.Debug("Flushed call events batch to ClickHouse", zap.Int("count", len(eventsToFlush)))
}

func (w *IngestionWorker) FlushTurns(ctx context.Context) {
	w.mu.Lock()
	if len(w.turnBuffer) == 0 {
		w.mu.Unlock()
		return
	}
	turnsToFlush := w.turnBuffer
	w.turnBuffer = make([]*domain.SentimentTurn, 0, 2000)
	w.mu.Unlock()

	if err := w.callRepo.BatchInsertSentimentTurns(ctx, turnsToFlush); err != nil {
		w.log.Error("Failed to flush sentiment turns batch to ClickHouse",
			zap.Int("count", len(turnsToFlush)),
			zap.Error(err),
		)
		w.mu.Lock()
		w.turnBuffer = append(turnsToFlush, w.turnBuffer...)
		w.mu.Unlock()
		return
	}

	w.log.Debug("Flushed sentiment turns batch to ClickHouse", zap.Int("count", len(turnsToFlush)))
}

func (w *IngestionWorker) Close() error {
	close(w.stopChan)
	w.flushTicker.Stop()
	w.FlushAll(context.Background())
	return w.consumerGroup.Close()
}
