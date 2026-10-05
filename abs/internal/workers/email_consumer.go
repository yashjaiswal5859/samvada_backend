// =============================================================================
// Samvada Platform — Notification & Email Kafka Consumer Worker
// =============================================================================
// Consumes auth.email.send events from Kafka and handles OTP verification emails
// and system notifications.
// =============================================================================

package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/smtp"
	"os"

	"github.com/IBM/sarama"
	"go.uber.org/zap"
)

type AuthEventEnvelope struct {
	EventID       string      `json:"event_id"`
	EventType     string      `json:"event_type"`
	AggregateID   string      `json:"aggregate_id"`
	AggregateType string      `json:"aggregate_type"`
	Version       int         `json:"version"`
	CorrelationID string      `json:"correlation_id"`
	Payload       interface{} `json:"payload"`
}

type OTPEmailData struct {
	Type  string `json:"type"`
	Email string `json:"email"`
	OTP   string `json:"otp"`
}

type EmailConsumer struct {
	consumerGroup sarama.ConsumerGroup
	topic         string
	smtpHost      string
	smtpPort      string
	smtpUser      string
	smtpPassword  string
	smtpFrom      string
	log           *zap.Logger
	stopChan      chan struct{}
}

func NewEmailConsumer(
	brokers []string,
	groupName string,
	topic string,
	log *zap.Logger,
) (*EmailConsumer, error) {
	if topic == "" {
		topic = "auth.email.send"
	}
	if groupName == "" {
		groupName = "abs-email-worker-group"
	}

	config := sarama.NewConfig()
	config.Consumer.Offsets.Initial = sarama.OffsetNewest
	config.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategyRoundRobin()}

	group, err := sarama.NewConsumerGroup(brokers, groupName, config)
	if err != nil {
		return nil, fmt.Errorf("creating email consumer group: %w", err)
	}

	smtpHost := os.Getenv("SMTP_HOST")
	if smtpHost == "" {
		smtpHost = "localhost"
	}
	smtpPort := os.Getenv("SMTP_PORT")
	if smtpPort == "" {
		smtpPort = "1025" // Default Mailpit SMTP port
	}
	smtpUser := os.Getenv("SMTP_USER")
	smtpPassword := os.Getenv("SMTP_PASSWORD")

	smtpFrom := os.Getenv("SMTP_FROM")
	if smtpFrom == "" {
		if smtpUser != "" {
			smtpFrom = smtpUser
		} else {
			smtpFrom = "no-reply@samvada.ai"
		}
	}

	return &EmailConsumer{
		consumerGroup: group,
		topic:         topic,
		smtpHost:      smtpHost,
		smtpPort:      smtpPort,
		smtpUser:      smtpUser,
		smtpPassword:  smtpPassword,
		smtpFrom:      smtpFrom,
		log:           log.Named("email_consumer"),
		stopChan:      make(chan struct{}),
	}, nil
}

func (w *EmailConsumer) Start(ctx context.Context) {
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
					w.log.Warn("Kafka consumer error in email worker", zap.Error(err))
				}
			}
		}
	}()
	w.log.Info("Email & Notification consumer worker started", zap.String("topic", w.topic))
}

func (w *EmailConsumer) Stop() {
	close(w.stopChan)
	if err := w.consumerGroup.Close(); err != nil {
		w.log.Warn("Error closing email consumer group", zap.Error(err))
	}
	w.log.Info("Email & Notification consumer worker stopped")
}

func (w *EmailConsumer) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (w *EmailConsumer) Cleanup(sarama.ConsumerGroupSession) error { return nil }

func (w *EmailConsumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		w.processMessage(msg.Value)
		session.MarkMessage(msg, "")
	}
	return nil
}

func (w *EmailConsumer) processMessage(payloadBytes []byte) {
	var env AuthEventEnvelope
	if err := json.Unmarshal(payloadBytes, &env); err != nil {
		w.log.Error("Failed to decode auth event envelope", zap.Error(err))
		return
	}

	// Extract email and OTP from payload (supports map or raw string)
	var emailData OTPEmailData
	switch p := env.Payload.(type) {
	case string:
		_ = json.Unmarshal([]byte(p), &emailData)
	case map[string]interface{}:
		payloadJSON, _ := json.Marshal(p)
		_ = json.Unmarshal(payloadJSON, &emailData)
	}

	if emailData.Email == "" {
		w.log.Warn("Skipping email dispatch: missing recipient email in payload",
			zap.String("event_id", env.EventID),
		)
		return
	}

	// 1. Prominent structured banner log for developer observability
	w.log.Info(fmt.Sprintf("\n"+
		"======================================================================\n"+
		"  📧 [EMAIL WORKER] DISPATCHING VERIFICATION EMAIL\n"+
		"  To:       %s\n"+
		"  Subject:  Your Samvada Security Verification Code\n"+
		"  Purpose:  %s\n"+
		"  🔑 OTP:   %s\n"+
		"======================================================================",
		emailData.Email, emailData.Type, emailData.OTP,
	),
		zap.String("event_id", env.EventID),
		zap.String("recipient", emailData.Email),
		zap.String("type", emailData.Type),
		zap.String("otp", emailData.OTP),
	)

	// 2. Dispatch via SMTP (supports authenticated relay like Gmail/SendGrid, or unauthenticated Mailpit)
	smtpAddr := fmt.Sprintf("%s:%s", w.smtpHost, w.smtpPort)
	var auth smtp.Auth
	if w.smtpUser != "" && w.smtpPassword != "" {
		auth = smtp.PlainAuth("", w.smtpUser, w.smtpPassword, w.smtpHost)
	}

	subject := "Your Samvada Verification Code"
	body := fmt.Sprintf("Subject: %s\r\nFrom: %s\r\nTo: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nYour verification code is: %s (expires in 5 minutes).\n\nIf you did not request this, please ignore this email.\n\n— Team Samvada Voice AI",
		subject, w.smtpFrom, emailData.Email, emailData.OTP)

	err := smtp.SendMail(smtpAddr, auth, w.smtpFrom, []string{emailData.Email}, []byte(body))
	if err != nil {
		w.log.Warn("SMTP dispatch failed (email was logged to console)",
			zap.String("smtp_addr", smtpAddr),
			zap.String("to", emailData.Email),
			zap.Error(err),
		)
	} else {
		w.log.Info("SMTP message sent to relay successfully",
			zap.String("smtp_addr", smtpAddr),
			zap.String("to", emailData.Email),
		)
	}
}
