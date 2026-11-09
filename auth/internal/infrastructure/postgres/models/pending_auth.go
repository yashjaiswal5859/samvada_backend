package models

import (
	"time"

	"github.com/google/uuid"
)

type PendingAuth struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey"`
	Email        string    `gorm:"type:text;not null;index:idx_pending_auth_email"`
	OTPHash      string    `gorm:"type:text;not null"`
	Purpose      string    `gorm:"type:text;not null"`
	ExpiresAt    time.Time `gorm:"type:timestamptz;not null;index:idx_pending_auth_expires_at"`
	AttemptCount int       `gorm:"type:int;not null;default:0"`
	MaxAttempts  int       `gorm:"type:int;not null;default:5"`
	Metadata     string    `gorm:"type:text"`
	CreatedAt    time.Time `gorm:"type:timestamptz;not null"`
	UpdatedAt    time.Time `gorm:"type:timestamptz;not null"`
}

func (PendingAuth) TableName() string {
	return "pending_auth"
}
