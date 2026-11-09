package events

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type DomainEvent struct {
	EventID       uuid.UUID       `json:"event_id"`
	EventType     EventType       `json:"event_type"`
	AggregateID   string          `json:"aggregate_id"`
	AggregateType string          `json:"aggregate_type"`
	Version       int             `json:"version"`
	OccurredAt    time.Time       `json:"occurred_at"`
	CorrelationID string          `json:"correlation_id"`
	Payload       json.RawMessage `json:"payload"`
}

func NewDomainEvent(eventType EventType, aggregateID string, aggregateType string, version int, correlationID string, payload interface{}) (DomainEvent, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return DomainEvent{}, err
	}

	return DomainEvent{
		EventID:       uuid.New(),
		EventType:     eventType,
		AggregateID:   aggregateID,
		AggregateType: aggregateType,
		Version:       version,
		OccurredAt:    time.Now().UTC(),
		CorrelationID: correlationID,
		Payload:       payloadBytes,
	}, nil
}
