// =============================================================================
// Samvada Platform — ABS Customer Webhook Delivery Kafka Consumer Worker
// =============================================================================
// Consumes abs.webhook.event messages from Kafka and delivers signed HTTP POST
// requests to customer endpoints.
// =============================================================================

package workers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/IBM/sarama"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/domain"
)

type WebhookEventPayload struct {
	EventID     uuid.UUID   `json:"event_id"`
	WorkspaceID uuid.UUID   `json:"workspace_id"`
	EventType   string      `json:"event_type"`
	TargetURL   string      `json:"target_url"`
	Secret      string      `json:"secret"`
	Data        interface{} `json:"data"`
}

type WebhookConsumer struct {
	consumerGroup sarama.ConsumerGroup
	repo          domain.IntegrationRepository
	httpClient    *http.Client
	topic         string
	log           *zap.Logger
	stopChan      chan struct{}
}

func NewWebhookConsumer(
	brokers []string,
	groupName string,
	topic string,
	repo domain.IntegrationRepository,
	log *zap.Logger,
) (*WebhookConsumer, error) {
	if topic == "" {
		topic = "abs.webhook.event"
	}
	if groupName == "" {
		groupName = "abs-webhook-worker-group"
	}

	config := sarama.NewConfig()
	config.Consumer.Offsets.Initial = sarama.OffsetNewest
	config.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategyRoundRobin()}

	group, err := sarama.NewConsumerGroup(brokers, groupName, config)
	if err != nil {
		return nil, fmt.Errorf("creating webhook consumer group: %w", err)
	}

	return &WebhookConsumer{
		consumerGroup: group,
		repo:          repo,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		topic:    topic,
		log:      log.Named("webhook_consumer"),
		stopChan: make(chan struct{}),
	}, nil
}

func (w *WebhookConsumer) Start(ctx context.Context) {
	topics := []string{w.topic}
	go func() {
		for {
			select {
			case <-w.stopChan:
				return
			case <-ctx.Done():
				return
			default:
				if err := w.consumerGroup.Consume(ctx, topics, w); err != nil {
					w.log.Warn("Kafka consumer error in webhook worker", zap.Error(err))
				}
			}
		}
	}()
	w.log.Info("Webhook consumer worker started", zap.String("topic", w.topic))
}

func (w *WebhookConsumer) Stop() {
	close(w.stopChan)
	if err := w.consumerGroup.Close(); err != nil {
		w.log.Warn("Error closing webhook consumer group", zap.Error(err))
	}
	w.log.Info("Webhook consumer worker stopped")
}

func (w *WebhookConsumer) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (w *WebhookConsumer) Cleanup(sarama.ConsumerGroupSession) error { return nil }

func (w *WebhookConsumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		w.processMessage(session.Context(), msg.Value)
		session.MarkMessage(msg, "")
	}
	return nil
}

func (w *WebhookConsumer) processMessage(ctx context.Context, payload []byte) {
	var ev WebhookEventPayload
	if err := json.Unmarshal(payload, &ev); err != nil {
		w.log.Error("Failed to decode webhook event payload", zap.Error(err))
		return
	}

	if ev.TargetURL == "" {
		w.log.Warn("Skipping webhook delivery: missing target_url", zap.String("event_id", ev.EventID.String()))
		return
	}

	bodyBytes, err := json.Marshal(ev.Data)
	if err != nil {
		bodyBytes = payload
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ev.TargetURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		w.log.Error("Failed to create webhook HTTP request", zap.Error(err))
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Samvada-Event", ev.EventType)
	req.Header.Set("X-Samvada-Delivery", ev.EventID.String())

	if ev.Secret != "" {
		mac := hmac.New(sha256.New, []byte(ev.Secret))
		mac.Write(bodyBytes)
		req.Header.Set("X-Samvada-Signature-256", hex.EncodeToString(mac.Sum(nil)))
	}

	resp, err := w.httpClient.Do(req)
	if err != nil {
		w.log.Warn("Webhook delivery failed",
			zap.String("event_id", ev.EventID.String()),
			zap.String("target_url", ev.TargetURL),
			zap.Error(err),
		)
		return
	}
	defer resp.Body.Close()

	w.log.Info("Webhook delivered successfully",
		zap.String("event_id", ev.EventID.String()),
		zap.String("target_url", ev.TargetURL),
		zap.Int("status_code", resp.StatusCode),
	)
}
