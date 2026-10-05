package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"time"

	"voice-agent-go/internal/config"

	"github.com/segmentio/kafka-go"
)

const (
	KafkaAudioTopic      = "voice_audio_stream"
	KafkaTranscriptTopic = "voice_transcript_stream"
	KafkaLogTopic        = "voice_system_logs"
)

type AudioFrameEvent struct {
	SessionID string `json:"session_id"`
	Channel   string `json:"channel"` // "customer" or "agent"
	Payload   string `json:"payload"` // Base64 encoded audio bytes
	Timestamp int64  `json:"timestamp"`
	EventType string `json:"event_type"` // "start", "media", "stop"
}

type TranscriptEvent struct {
	SessionID string `json:"session_id"`
	Speaker   string `json:"speaker"` // "user" or "agent"
	Text      string `json:"text"`    // Speech text
	Timestamp int64  `json:"timestamp"`
}

type LogEvent struct {
	SessionID string `json:"session_id,omitempty"`
	Component string `json:"component"` // "WEBSOCKET", "TTS", "STT", "PLIVO"
	Level     string `json:"level"`     // "INFO", "WARN", "ERROR"
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

type KafkaAudioService struct {
	writer          *kafka.Writer
	localAudioQueue chan AudioFrameEvent
	localTxQueue    chan TranscriptEvent
	localLogQueue   chan LogEvent
	useFallback     bool
}

var GlobalKafkaAudioService *KafkaAudioService

func InitKafkaAudioService() *KafkaAudioService {
	svc := &KafkaAudioService{
		localAudioQueue: make(chan AudioFrameEvent, 10000),
		localTxQueue:    make(chan TranscriptEvent, 10000),
		localLogQueue:   make(chan LogEvent, 10000),
		useFallback:     true,
	}

	if config.Settings.KafkaBootstrapServers != "" {
		svc.writer = &kafka.Writer{
			Addr:     kafka.TCP(config.Settings.KafkaBootstrapServers),
			Balancer: &kafka.Hash{},
		}
		svc.useFallback = false
		log.Printf("[Kafka Service] Initialized Kafka Producer at %s", config.Settings.KafkaBootstrapServers)
	} else {
		log.Printf("[Kafka Service] Kafka not configured. Offloading events & logs to Async Local Worker Queue.")
	}

	GlobalKafkaAudioService = svc
	return svc
}

func (k *KafkaAudioService) PublishAudioFrame(sessionID string, channel string, data []byte, eventType string) {
	if k == nil {
		return
	}

	event := AudioFrameEvent{
		SessionID: sessionID,
		Channel:   channel,
		Payload:   base64.StdEncoding.EncodeToString(data),
		Timestamp: time.Now().UnixMilli(),
		EventType: eventType,
	}

	if !k.useFallback && k.writer != nil {
		go func() {
			msgBytes, err := json.Marshal(event)
			if err != nil {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			_ = k.writer.WriteMessages(ctx, kafka.Message{
				Topic: KafkaAudioTopic,
				Key:   []byte(sessionID),
				Value: msgBytes,
			})
		}()
	} else {
		select {
		case k.localAudioQueue <- event:
		default:
		}
	}
}

func (k *KafkaAudioService) PublishTranscript(sessionID string, speaker string, text string) {
	if k == nil || text == "" {
		return
	}

	event := TranscriptEvent{
		SessionID: sessionID,
		Speaker:   speaker,
		Text:      text,
		Timestamp: time.Now().UnixMilli(),
	}

	log.Printf("[Kafka Transcript] [%s] %s: %q", sessionID, speaker, text)

	if !k.useFallback && k.writer != nil {
		go func() {
			msgBytes, err := json.Marshal(event)
			if err != nil {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			_ = k.writer.WriteMessages(ctx, kafka.Message{
				Topic: KafkaTranscriptTopic,
				Key:   []byte(sessionID),
				Value: msgBytes,
			})
		}()
	} else {
		select {
		case k.localTxQueue <- event:
		default:
		}
	}
}

func (k *KafkaAudioService) PublishLog(sessionID string, component string, level string, message string) {
	if k == nil || message == "" {
		return
	}

	event := LogEvent{
		SessionID: sessionID,
		Component: component,
		Level:     level,
		Message:   message,
		Timestamp: time.Now().UnixMilli(),
	}

	if !k.useFallback && k.writer != nil {
		go func() {
			msgBytes, err := json.Marshal(event)
			if err != nil {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			_ = k.writer.WriteMessages(ctx, kafka.Message{
				Topic: KafkaLogTopic,
				Key:   []byte(sessionID),
				Value: msgBytes,
			})
		}()
	} else {
		select {
		case k.localLogQueue <- event:
		default:
		}
	}
}

func (k *KafkaAudioService) GetLocalAudioQueue() <-chan AudioFrameEvent {
	return k.localAudioQueue
}

func (k *KafkaAudioService) GetLocalTxQueue() <-chan TranscriptEvent {
	return k.localTxQueue
}

func (k *KafkaAudioService) GetLocalLogQueue() <-chan LogEvent {
	return k.localLogQueue
}

func (k *KafkaAudioService) Close() {
	if k.writer != nil {
		k.writer.Close()
	}
}
