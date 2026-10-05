package models

import (
	"time"

	"github.com/google/uuid"
)

type Tenant struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	Name      string    `gorm:"type:varchar(255);not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

type User struct {
	ID             uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	TenantID       uuid.UUID `gorm:"type:uuid;not null;index"`
	Email          string    `gorm:"type:varchar(255);unique;not null"`
	HashedPassword string    `gorm:"type:varchar(255);not null"`
	Role           string    `gorm:"type:varchar(50);not null;default:'agent'"`
	IsActive       bool      `gorm:"type:boolean;not null;default:true"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Call struct {
	ID          uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	TenantID    uuid.UUID  `gorm:"type:uuid;not null;index"`
	SessionID   string     `gorm:"type:varchar(255);unique;not null;index"`
	Direction   string     `gorm:"type:varchar(50)"`
	Status      string     `gorm:"type:varchar(50)"`
	Outcome     string     `gorm:"type:varchar(255)"`
	StartedAt   *time.Time `gorm:"type:timestamp"`
	EndedAt     *time.Time `gorm:"type:timestamp"`
	DurationSec int        `gorm:"type:integer"`
	CreatedAt   time.Time
}

type ConversationHistory struct {
	ID        uint      `gorm:"primary_key;autoIncrement"`
	CallID    uuid.UUID `gorm:"type:uuid;not null;index"`
	Role      string    `gorm:"type:varchar(50);not null"`
	Content   string    `gorm:"type:text;not null"`
	Timestamp time.Time `gorm:"type:timestamp;not null;default:now()"`
	IsFinal   bool      `gorm:"type:boolean;not null;default:true"`
}

type EmotionTimeline struct {
	ID        uint      `gorm:"primary_key;autoIncrement"`
	CallID    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex"`
	Timeline  string    `gorm:"type:jsonb;not null"` // JSON format timeline
	Timestamp time.Time `gorm:"type:timestamp;not null;default:now()"`
}

type ToolExecution struct {
	ID           uint      `gorm:"primary_key;autoIncrement"`
	CallID       uuid.UUID `gorm:"type:uuid;not null;index"`
	ToolName     string    `gorm:"type:varchar(255);not null"`
	InputParams  string    `gorm:"type:jsonb"`
	OutputResult string    `gorm:"type:jsonb"`
	Status       string    `gorm:"type:varchar(50)"`
	Timestamp    time.Time `gorm:"type:timestamp;not null;default:now()"`
}

type CustomerProfile struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	TenantID  uuid.UUID `gorm:"type:uuid;not null;index"`
	Phone     string    `gorm:"type:varchar(50);not null;index"`
	Name      string    `gorm:"type:varchar(255)"`
	Notes     string    `gorm:"type:text"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

type KnowledgeDocument struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	TenantID  uuid.UUID `gorm:"type:uuid;not null;index"`
	Title     string    `gorm:"type:varchar(255);not null"`
	Content   string    `gorm:"type:text;not null"`
	Source    string    `gorm:"type:varchar(255)"`
	CreatedAt time.Time
}

type CustomerMemory struct {
	ID           uint      `gorm:"primary_key;autoIncrement"`
	TenantID     uuid.UUID `gorm:"type:uuid;not null;index"`
	Phone        string    `gorm:"type:varchar(50);not null;index"`
	MemoryText   string    `gorm:"type:text;not null"`
	SourceCallID uuid.UUID `gorm:"type:uuid;not null;index"`
	CreatedAt    time.Time `gorm:"type:timestamp;not null;default:now()"`
}

type EscalationRecord struct {
	ID               uint      `gorm:"primary_key;autoIncrement"`
	SessionID        string    `gorm:"type:varchar(255);not null;index"`
	Phone            string    `gorm:"type:varchar(50);not null;index"`
	CustomerName     string    `gorm:"type:varchar(255)"`
	FrustrationLevel string    `gorm:"type:varchar(50);not null"` // e.g. "HIGH", "MEDIUM", "LOW"
	Emotion          string    `gorm:"type:varchar(50);not null"` // "frustrated", "angry"
	Reason           string    `gorm:"type:text;not null"`
	CSATRating       float64   `gorm:"type:decimal(3,2)"`
	Status           string    `gorm:"type:varchar(50);not null;default:'PENDING_CALL_BACK'"` // "PENDING_CALL_BACK", "COMPLETED"
	CreatedAt        time.Time `gorm:"type:timestamp;not null;default:now()"`
}
