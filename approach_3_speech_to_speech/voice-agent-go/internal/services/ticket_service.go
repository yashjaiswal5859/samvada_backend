// =============================================================================
// Samvada Platform — S2S Ephemeral Call Ticket Validation Service (Phase 3.2)
// =============================================================================
// Cryptographically validates HMAC-signed ephemeral tickets and performs atomic
// single-use nonce consumption from Redis (< 0.1ms verification).
// =============================================================================

package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ErrInvalidTicketFormat = errors.New("invalid call ticket format")
	ErrTicketExpired       = errors.New("call ticket has expired")
	ErrInvalidSignature    = errors.New("invalid call ticket HMAC signature")
	ErrNonceAlreadyUsed    = errors.New("call ticket has already been used or replayed")
)

type VerifiedTicket struct {
	UserID      string
	OrgID       string
	WorkspaceID string
	AgentID     string
	ExpiresAt   int64
	Nonce       string
}

type TicketService struct {
	secret      []byte
	redisClient *redis.Client
}

func NewTicketService(secret string, redisClient *redis.Client) *TicketService {
	if secret == "" {
		secret = "samvada_ticket_secret_key_2026"
	}
	return &TicketService{
		secret:      []byte(secret),
		redisClient: redisClient,
	}
}

// VerifyTicket performs < 0.1ms cryptographic signature verification,
// expiry check, and single-use atomic Redis nonce invalidation.
func (s *TicketService) VerifyTicket(ctx context.Context, ticketStr string) (*VerifiedTicket, error) {
	parts := strings.Split(ticketStr, ".")
	if len(parts) != 2 {
		return nil, ErrInvalidTicketFormat
	}

	payload := parts[0]
	signature := parts[1]

	// 1. Verify HMAC-SHA256 signature locally (sub-millisecond)
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payload))
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		return nil, ErrInvalidSignature
	}

	// 2. Parse payload: sub:org_id:ws_id:agent_id:exp:nonce (or legacy ws_id:agent_id:exp:nonce)
	fields := strings.Split(payload, ":")
	var vt VerifiedTicket

	if len(fields) == 6 {
		vt.UserID = fields[0]
		vt.OrgID = fields[1]
		vt.WorkspaceID = fields[2]
		vt.AgentID = fields[3]
		exp, err := strconv.ParseInt(fields[4], 10, 64)
		if err != nil {
			return nil, ErrInvalidTicketFormat
		}
		vt.ExpiresAt = exp
		vt.Nonce = fields[5]
	} else if len(fields) == 4 {
		vt.WorkspaceID = fields[0]
		vt.AgentID = fields[1]
		exp, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			return nil, ErrInvalidTicketFormat
		}
		vt.ExpiresAt = exp
		vt.Nonce = fields[3]
	} else {
		return nil, ErrInvalidTicketFormat
	}

	// 3. Verify expiry
	if time.Now().Unix() > vt.ExpiresAt {
		return nil, ErrTicketExpired
	}

	// 4. Atomic single-use check & consume via Redis Lua script
	if s.redisClient != nil && vt.Nonce != "" {
		luaScript := `
			local k1 = "samvada:ticket:nonce:" .. KEYS[1]
			local k2 = "ticket:nonce:" .. KEYS[1]
			if redis.call("EXISTS", k1) == 1 then
				redis.call("DEL", k1)
				return 1
			elseif redis.call("EXISTS", k2) == 1 then
				redis.call("DEL", k2)
				return 1
			else
				return 0
			end
		`
		res, err := s.redisClient.Eval(ctx, luaScript, []string{vt.Nonce}).Int()
		if err != nil {
			return nil, fmt.Errorf("redis nonce verification failed: %w", err)
		}
		if res == 0 {
			return nil, ErrNonceAlreadyUsed
		}
	}

	return &vt, nil
}
