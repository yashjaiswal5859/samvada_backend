// =============================================================================
// Samvada Platform — ABS Kafka Infrastructure Adapter (Consumer Group Base)
// =============================================================================

package kafka

import (
	"context"
	"fmt"

	"github.com/IBM/sarama"
	"go.uber.org/zap"
)

// MessageHandler is the signature for handling incoming Kafka messages.
type MessageHandler func(ctx context.Context, msg *sarama.ConsumerMessage) error

// ConsumerGroup wraps a Sarama ConsumerGroup for managing background event consumers.
type ConsumerGroup struct {
	group   sarama.ConsumerGroup
	topics  []string
	handler MessageHandler
	log     *zap.Logger
}

type groupHandler struct {
	handler MessageHandler
	log     *zap.Logger
}

func (h *groupHandler) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (h *groupHandler) Cleanup(sarama.ConsumerGroupSession) error { return nil }

func (h *groupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		ctx := session.Context()
		if err := h.handler(ctx, msg); err != nil {
			h.log.Error("Error handling Kafka message",
				zap.String("topic", msg.Topic),
				zap.Int32("partition", msg.Partition),
				zap.Int64("offset", msg.Offset),
				zap.Error(err),
			)
			// Depending on strategy, can retry or mark message
		}
		session.MarkMessage(msg, "")
	}
	return nil
}

// NewConsumerGroup initializes a Kafka ConsumerGroup for the given topics.
func NewConsumerGroup(brokers []string, groupID string, topics []string, handler MessageHandler, log *zap.Logger) (*ConsumerGroup, error) {
	cfg := sarama.NewConfig()
	cfg.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategyRoundRobin()}
	cfg.Consumer.Offsets.Initial = sarama.OffsetOldest
	cfg.Consumer.Return.Errors = true

	group, err := sarama.NewConsumerGroup(brokers, groupID, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka consumer group '%s': %w", groupID, err)
	}

	return &ConsumerGroup{
		group:   group,
		topics:  topics,
		handler: handler,
		log:     log.Named("kafka_consumer"),
	}, nil
}

// Start begins the consume loop in a blocking manner (call in a goroutine).
func (cg *ConsumerGroup) Start(ctx context.Context) {
	gh := &groupHandler{handler: cg.handler, log: cg.log}
	for {
		if err := cg.group.Consume(ctx, cg.topics, gh); err != nil {
			if ctx.Err() != nil {
				return
			}
			cg.log.Error("Kafka consumer group error", zap.Error(err))
		}
		if ctx.Err() != nil {
			return
		}
	}
}

// Close closes the underlying consumer group.
func (cg *ConsumerGroup) Close() error {
	return cg.group.Close()
}
