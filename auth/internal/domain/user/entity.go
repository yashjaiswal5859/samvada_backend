package user

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID          uuid.UUID  `json:"id"`
	KratosID    string     `json:"kratos_id"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name"`
	PhoneNumber *string    `json:"phone_number"`
	IsActive    bool       `json:"is_active"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
}
