package oauthsession

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrSessionNotFound = errors.New("oauth session not found")
	ErrSessionExpired  = errors.New("oauth session expired")
	ErrSessionUsed     = errors.New("oauth session already used")
)

type OAuthSession struct {
	ID             uuid.UUID `json:"id"`
	State          string    `json:"state"`
	Nonce          string    `json:"nonce"`
	CodeVerifier   string    `json:"code_verifier"`
	LoginChallenge string    `json:"login_challenge"`
	Used           bool      `json:"used"`
	CreatedAt      time.Time `json:"created_at"`
	ExpiresAt      time.Time `json:"expires_at"`
}

func (s *OAuthSession) IsExpired() bool {
	return time.Now().UTC().After(s.ExpiresAt)
}
