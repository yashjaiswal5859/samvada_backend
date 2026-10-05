// =============================================================================
// Samvada Platform — ABS Kafka Infrastructure Adapter (Producer)
// =============================================================================

package kafka

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	"go.uber.org/zap"
)

// Producer wraps Sarama's SyncProducer for publishing domain events.
type Producer struct {
	syncProducer sarama.SyncProducer
	log          *zap.Logger
}

// NewProducer initializes a Kafka sync producer with at-least-once delivery guarantees.
func NewProducer(brokers []string, log *zap.Logger) (*Producer, error) {
	cfg := sarama.NewConfig()
	cfg.Producer.RequiredAcks = sarama.WaitForAll // High durability
	cfg.Producer.Retry.Max = 5
	cfg.Producer.Retry.Backoff = 100 * time.Millisecond
	cfg.Producer.Return.Successes = true
	cfg.Producer.Partitioner = sarama.NewHashPartitioner // Partition by message key

	producer, err := sarama.NewSyncProducer(brokers, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka producer: %w", err)
	}

	log.Info("Connected to Kafka producer successfully", zap.Strings("brokers", brokers))

	return &Producer{
		syncProducer: producer,
		log:          log.Named("kafka_producer"),
	}, nil
}

// Close gracefully flushes and closes the producer.
func (p *Producer) Close() error {
	return p.syncProducer.Close()
}

// Publish sends a raw message with a key to the designated Kafka topic.
func (p *Producer) Publish(topic string, key string, payload []byte) (int32, int64, error) {
	msg := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(key),
		Value: sarama.ByteEncoder(payload),
		Headers: []sarama.RecordHeader{
			{
				Key:   []byte("producer"),
				Value: []byte("samvada-abs"),
			},
			{
				Key:   []byte("timestamp"),
				Value: []byte(time.Now().UTC().Format(time.RFC3339Nano)),
			},
		},
	}

	partition, offset, err := p.syncProducer.SendMessage(msg)
	if err != nil {
		p.log.Error("Failed to publish message to Kafka",
			zap.String("topic", topic),
			zap.String("key", key),
			zap.Error(err),
		)
		return 0, 0, err
	}

	p.log.Debug("Message published to Kafka",
		zap.String("topic", topic),
		zap.String("key", key),
		zap.Int32("partition", partition),
		zap.Int64("offset", offset),
	)

	return partition, offset, nil
}

// PublishJSON serializes an object to JSON and publishes it to Kafka.
func (p *Producer) PublishJSON(topic string, key string, obj interface{}) (int32, int64, error) {
	bytes, err := json.Marshal(obj)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to marshal JSON payload: %w", err)
	}
	return p.Publish(topic, key, bytes)
}
