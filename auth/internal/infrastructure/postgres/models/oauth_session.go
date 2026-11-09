package models

import (
	"time"

	"github.com/google/uuid"
)

type OAuthSession struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	State          string    `gorm:"type:varchar(255);not null;index"`
	Nonce          string    `gorm:"type:varchar(255);not null"`
	CodeVerifier   string    `gorm:"type:varchar(255);not null"`
	LoginChallenge string    `gorm:"type:text"`
	Used           bool      `gorm:"type:boolean;not null;default:false"`
	CreatedAt      time.Time `gorm:"not null"`
	ExpiresAt      time.Time `gorm:"not null"`
}

func (OAuthSession) TableName() string {
	return "oauth_sessions"
}
