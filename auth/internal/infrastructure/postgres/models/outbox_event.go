package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type OutboxEvent struct {
	ID            uuid.UUID       `gorm:"type:uuid;primaryKey"`
	EventType     string          `gorm:"type:text;not null"`
	AggregateID   string          `gorm:"type:text;not null;index:idx_outbox_events_aggregate"`
	Payload       json.RawMessage `gorm:"type:jsonb;not null"`
	Status        string          `gorm:"type:text;not null;default:'PENDING';index:idx_outbox_events_status"`
	RetryCount    int             `gorm:"type:int;not null;default:0"`
	NextRetryAt   time.Time       `gorm:"type:timestamptz;not null;default:now();index:idx_outbox_events_next_retry"`
	CorrelationID string          `gorm:"type:text"`
	LastError     *string         `gorm:"type:text"`
	CreatedAt     time.Time       `gorm:"type:timestamptz;not null"`
	PublishedAt   *time.Time      `gorm:"type:timestamptz"`
}

func (OutboxEvent) TableName() string {
	return "outbox_events"
}
