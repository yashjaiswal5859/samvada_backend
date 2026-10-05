// =============================================================================
// Samvada Platform — Phase 3 S2S Integration & Security Tests
// =============================================================================

package services_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"voice-agent-go/internal/services"
)

func TestTicketService_HMACAndExpiryValidation(t *testing.T) {
	secret := "samvada_ticket_secret_key_2026"
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	defer rdb.Close()

	ctxPing, cancelPing := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancelPing()
	if err := rdb.Ping(ctxPing).Err(); err != nil {
		t.Skip("Skipping test: Redis is not reachable on localhost:6379:", err)
	}

	ticketService := services.NewTicketService(secret, rdb)

	userID := uuid.New().String()
	orgID := uuid.New().String()
	wsID := uuid.New().String()
	agentID := uuid.New().String()
	nonce := uuid.New().String()
	exp := time.Now().Add(60 * time.Second).Unix()

	// 1. Valid Ticket
	payload := fmt.Sprintf("%s:%s:%s:%s:%d:%s", userID, orgID, wsID, agentID, exp, nonce)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	validTicket := fmt.Sprintf("%s.%s", payload, sig)

	// Pre-seed Redis single-use nonce
	ctx := context.Background()
	err := rdb.Set(ctx, "samvada:ticket:nonce:"+nonce, "valid", 60*time.Second).Err()
	require.NoError(t, err)

	vt, err := ticketService.VerifyTicket(ctx, validTicket)
	require.NoError(t, err)
	assert.Equal(t, userID, vt.UserID)
	assert.Equal(t, orgID, vt.OrgID)
	assert.Equal(t, wsID, vt.WorkspaceID)
	assert.Equal(t, agentID, vt.AgentID)
	assert.Equal(t, nonce, vt.Nonce)

	// 2. Replay Attack Prevention (Nonce already consumed atomically)
	_, err = ticketService.VerifyTicket(ctx, validTicket)
	assert.ErrorIs(t, err, services.ErrNonceAlreadyUsed, "Reusing a consumed ticket must be rejected")

	// 3. Forged Signature Detection
	forgedTicket := fmt.Sprintf("%s.badsignature1234567890abcdef", payload)
	_, err = ticketService.VerifyTicket(ctx, forgedTicket)
	assert.ErrorIs(t, err, services.ErrInvalidSignature, "Forged signature must be rejected")

	// 4. Expired Ticket Rejection
	pastExp := time.Now().Add(-10 * time.Second).Unix()
	expiredPayload := fmt.Sprintf("%s:%s:%s:%s:%d:%s", userID, orgID, wsID, agentID, pastExp, uuid.New().String())
	mac2 := hmac.New(sha256.New, []byte(secret))
	mac2.Write([]byte(expiredPayload))
	sig2 := hex.EncodeToString(mac2.Sum(nil))
	expiredTicket := fmt.Sprintf("%s.%s", expiredPayload, sig2)

	_, err = ticketService.VerifyTicket(ctx, expiredTicket)
	assert.ErrorIs(t, err, services.ErrTicketExpired, "Expired ticket must be rejected")
}

func TestTelemetryEmitter_EventFormatting(t *testing.T) {
	conn, err := net.DialTimeout("tcp", "localhost:9092", 500*time.Millisecond)
	if err != nil {
		t.Skip("Skipping test: Kafka is not reachable on localhost:9092:", err)
	}
	_ = conn.Close()

	emitter := services.NewTelemetryEmitter([]string{"localhost:9092"})
	defer emitter.Close()

	ctx := context.Background()
	callID := uuid.New().String()
	orgID := uuid.New().String()
	wsID := uuid.New().String()
	agentID := uuid.New().String()

	// 1. Emit call started
	err = emitter.EmitCallStarted(ctx, services.S2SCallStartedEvent{
		CallID:       callID,
		OrgID:        orgID,
		WorkspaceID:  wsID,
		AgentID:      agentID,
		Direction:    "inbound",
		CallerNumber: "+15551234567",
		CalledNumber: "+15559876543",
		CustomerName: "Dr. Jane Doe",
		StartedAt:    time.Now().UTC(),
	})
	assert.NoError(t, err)

	// 2. Emit sentiment turns
	err = emitter.EmitSentimentTurn(ctx, services.S2SSentimentTurnEvent{
		CallID:         callID,
		OrgID:          orgID,
		WorkspaceID:    wsID,
		AgentID:        agentID,
		TurnIndex:      1,
		Speaker:        "customer",
		Timestamp:      time.Now().UTC(),
		Text:           "I want to check my account balance please.",
		Sentiment:      "neutral",
		Emotion:        "neutral",
		SentimentScore: 0.5,
		LatencyMS:      85,
	})
	assert.NoError(t, err)

	// 3. Emit call completed
	err = emitter.EmitCallCompleted(ctx, services.S2SCallCompletedEvent{
		CallID:           callID,
		OrgID:            orgID,
		WorkspaceID:      wsID,
		AgentID:          agentID,
		Direction:        "inbound",
		CallerNumber:     "+15551234567",
		CalledNumber:     "+15559876543",
		CustomerName:     "Dr. Jane Doe",
		StartedAt:        time.Now().UTC().Add(-65 * time.Second),
		EndedAt:          time.Now().UTC(),
		DurationSec:      65,
		BillableSec:      120,
		Disposition:      "completed",
		EvaluationScore:  95,
		STTLatencyMS:     115,
		LLMLatencyMS:     165,
		TTSLatencyMS:     90,
		E2ELatencyMS:     370,
		CostTelephonyUSD: 0.003,
		CostSTTUSD:       0.002,
		CostLLMUSD:       0.004,
		CostTTSUSD:       0.002,
		TotalCostUSD:     0.011,
		TurnCount:        4,
		OverallSentiment: "positive",
		CreatedAt:        time.Now().UTC(),
	})
	assert.NoError(t, err)
}
