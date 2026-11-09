package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey"`
	KratosID    string         `gorm:"type:text;uniqueIndex;not null"`
	Email       string         `gorm:"type:text;uniqueIndex;not null"`
	DisplayName string         `gorm:"type:text"`
	PhoneNumber *string        `gorm:"type:text"`
	IsActive    bool           `gorm:"type:boolean;not null;default:true"`
	CreatedAt   time.Time      `gorm:"type:timestamptz;not null"`
	UpdatedAt   time.Time      `gorm:"type:timestamptz;not null"`
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

func (User) TableName() string {
	return "users"
}
