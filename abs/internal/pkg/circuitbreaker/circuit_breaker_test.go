// =============================================================================
// Samvada Platform — Circuit Breaker Unit Tests (Phase 6.3)
// =============================================================================

package circuitbreaker_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/samvada/abs/internal/pkg/circuitbreaker"
)

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	cb := circuitbreaker.NewCircuitBreaker(circuitbreaker.Config{
		FailureThreshold: 2,
		SuccessThreshold: 1,
		Timeout:          50 * time.Millisecond,
	})

	assert.Equal(t, circuitbreaker.StateClosed, cb.State())

	// 1. Success in closed state
	err := cb.Execute(func() error { return nil })
	assert.NoError(t, err)
	assert.Equal(t, circuitbreaker.StateClosed, cb.State())

	// 2. First failure: remains closed
	errDummy := errors.New("remote service timeout")
	_ = cb.Execute(func() error { return errDummy })
	assert.Equal(t, circuitbreaker.StateClosed, cb.State())

	// 3. Second failure: reaches threshold (2) -> trips to OPEN
	_ = cb.Execute(func() error { return errDummy })
	assert.Equal(t, circuitbreaker.StateOpen, cb.State())

	// 4. Calls in OPEN state fail immediately with ErrCircuitOpen
	called := false
	err = cb.Execute(func() error {
		called = true
		return nil
	})
	assert.ErrorIs(t, err, circuitbreaker.ErrCircuitOpen)
	assert.False(t, called)

	// 5. Wait for timeout -> next call probes in HALF_OPEN
	time.Sleep(60 * time.Millisecond)

	err = cb.Execute(func() error {
		return nil // Success probe
	})
	assert.NoError(t, err)
	assert.Equal(t, circuitbreaker.StateClosed, cb.State())
}
