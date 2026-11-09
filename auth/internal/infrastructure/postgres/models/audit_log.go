package models

import (
	"time"

	"github.com/google/uuid"
)

type AuditLog struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey"`
	KratosID      string    `gorm:"type:text;index:idx_audit_logs_kratos_id"`
	EventType     string    `gorm:"type:text;not null"`
	IPAddress     string    `gorm:"type:text"`
	UserAgent     string    `gorm:"type:text"`
	CorrelationID string    `gorm:"type:text"`
	CreatedAt     time.Time `gorm:"type:timestamptz;not null;default:now()"`
}

func (AuditLog) TableName() string {
	return "audit_logs"
}
