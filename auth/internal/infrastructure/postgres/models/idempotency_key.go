package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type IdempotencyKey struct {
	ID             uuid.UUID       `gorm:"type:uuid;primaryKey"`
	IdempotencyKey string          `gorm:"type:text;uniqueIndex;not null"`
	RequestHash    string          `gorm:"type:text"`
	ResponseBody   json.RawMessage `gorm:"type:jsonb"`
	StatusCode     int             `gorm:"type:int"`
	ExpiresAt      time.Time       `gorm:"type:timestamptz;not null;index:idx_idempotency_keys_expires"`
	CreatedAt      time.Time       `gorm:"type:timestamptz;not null"`
}

func (IdempotencyKey) TableName() string {
	return "idempotency_keys"
}
