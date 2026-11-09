package pendingauth

import (
	"time"

	"github.com/google/uuid"
)

type Purpose string

const (
	PurposeRegister      Purpose = "REGISTER"
	PurposeRegisterAdmin Purpose = "REGISTER_ADMIN"
	PurposeLogin         Purpose = "LOGIN"
)

type PendingAuth struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	OTPHash      string    `json:"-"`
	Purpose      Purpose   `json:"purpose"`
	ExpiresAt    time.Time `json:"expires_at"`
	AttemptCount int       `json:"attempt_count"`
	MaxAttempts  int       `json:"max_attempts"`
	Metadata     string    `json:"metadata"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (pa *PendingAuth) IsExpired() bool {
	return time.Now().UTC().After(pa.ExpiresAt)
}

func (pa *PendingAuth) IncrementAttempts() {
	pa.AttemptCount++
}

func (pa *PendingAuth) HasExceededAttempts() bool {
	return pa.AttemptCount >= pa.MaxAttempts
}
