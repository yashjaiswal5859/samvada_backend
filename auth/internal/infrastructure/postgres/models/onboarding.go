package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type StringArray = pq.StringArray

type Organization struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	Name      string    `gorm:"type:text;not null"`
	OwnerID   uuid.UUID `gorm:"type:uuid;not null"`
	CreatedAt time.Time `gorm:"type:timestamptz;not null"`
	UpdatedAt time.Time `gorm:"type:timestamptz;not null"`
}

func (Organization) TableName() string {
	return "organizations"
}

type Workspace struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey"`
	Name      string     `gorm:"type:text;not null"`
	OrgID     *uuid.UUID `gorm:"type:uuid"`
	OwnerID   uuid.UUID  `gorm:"type:uuid;not null"`
	IsDefault bool       `gorm:"type:boolean;not null;default:true"`
	CreatedAt time.Time  `gorm:"type:timestamptz;not null"`
	UpdatedAt time.Time  `gorm:"type:timestamptz;not null"`
}

func (Workspace) TableName() string {
	return "workspaces"
}

type UserProfile struct {
	ID                  uuid.UUID      `gorm:"type:uuid;primaryKey"`
	UserID              uuid.UUID      `gorm:"type:uuid;uniqueIndex;not null"`
	FirstName           string         `gorm:"type:text;not null"`
	LastName            *string        `gorm:"type:text"`
	AccountType         string         `gorm:"type:text;not null"`
	Industries          pq.StringArray `gorm:"type:text[]"`
	OnboardingCompleted bool           `gorm:"type:boolean;not null;default:false"`
	CreatedAt           time.Time      `gorm:"type:timestamptz;not null"`
	UpdatedAt           time.Time      `gorm:"type:timestamptz;not null"`
}

func (UserProfile) TableName() string {
	return "user_profiles"
}
