package kafka

import (
	"context"
	"fmt"

	"github.com/IBM/sarama"
)

type Consumer interface {
	Consume(ctx context.Context, topics []string, handler sarama.ConsumerGroupHandler) error
	Close() error
}

type kafkaConsumer struct {
	group sarama.ConsumerGroup
}

func NewKafkaConsumer(brokers []string, groupID string, clientID string) (Consumer, error) {
	config := sarama.NewConfig()
	config.Consumer.Return.Errors = true
	config.Consumer.Offsets.Initial = sarama.OffsetOldest
	config.ClientID = clientID

	group, err := sarama.NewConsumerGroup(brokers, groupID, config)
	if err != nil {
		return nil, fmt.Errorf("failed to start Sarama consumer group: %w", err)
	}

	return &kafkaConsumer{
		group: group,
	}, nil
}

func (c *kafkaConsumer) Consume(ctx context.Context, topics []string, handler sarama.ConsumerGroupHandler) error {
	for {
		if err := c.group.Consume(ctx, topics, handler); err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

func (c *kafkaConsumer) Close() error {
	return c.group.Close()
}
