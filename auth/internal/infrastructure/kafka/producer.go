package kafka

import (
	"context"
	"fmt"

	"github.com/IBM/sarama"
)

type Producer interface {
	SendMessage(ctx context.Context, topic string, key string, value []byte) (partition int32, offset int64, err error)
	Close() error
}

type kafkaProducer struct {
	syncProducer sarama.SyncProducer
}

func NewKafkaProducer(brokers []string, clientID string) (Producer, error) {
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Return.Successes = true
	config.Producer.Idempotent = true
	config.Net.MaxOpenRequests = 1
	config.Producer.Compression = sarama.CompressionSnappy
	config.ClientID = clientID

	producer, err := sarama.NewSyncProducer(brokers, config)
	if err != nil {
		return nil, fmt.Errorf("failed to start Sarama sync producer: %w", err)
	}

	return &kafkaProducer{
		syncProducer: producer,
	}, nil
}

func (p *kafkaProducer) SendMessage(ctx context.Context, topic string, key string, value []byte) (partition int32, offset int64, err error) {
	msg := &sarama.ProducerMessage{
		Topic: topic,
		Value: sarama.ByteEncoder(value),
	}
	if key != "" {
		msg.Key = sarama.StringEncoder(key)
	}
	return p.syncProducer.SendMessage(msg)
}

func (p *kafkaProducer) Close() error {
	return p.syncProducer.Close()
}
