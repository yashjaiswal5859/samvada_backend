// =============================================================================
// Samvada Platform — Circuit Breaker Pattern for gRPC & Downstream Clients (Phase 6.3)
// =============================================================================

package circuitbreaker

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrCircuitOpen = errors.New("circuit breaker is open: downstream service unavailable")
)

type State string

const (
	StateClosed   State = "CLOSED"
	StateOpen     State = "OPEN"
	StateHalfOpen State = "HALF_OPEN"
)

type Config struct {
	FailureThreshold int           // Consecutive failures to open circuit (e.g. 5)
	SuccessThreshold int           // Consecutive successes in half-open to close (e.g. 2)
	Timeout          time.Duration // Duration circuit stays open before half-open probe (e.g. 5s)
}

type CircuitBreaker struct {
	mu               sync.Mutex
	state            State
	failureThreshold int
	successThreshold int
	timeout          time.Duration
	consecutiveFails int
	consecutiveWins  int
	lastStateChange  time.Time
}

func NewCircuitBreaker(cfg Config) *CircuitBreaker {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.SuccessThreshold <= 0 {
		cfg.SuccessThreshold = 2
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}

	return &CircuitBreaker{
		state:            StateClosed,
		failureThreshold: cfg.FailureThreshold,
		successThreshold: cfg.SuccessThreshold,
		timeout:          cfg.Timeout,
		lastStateChange:  time.Now(),
	}
}

// Execute runs the protected operation fn through the circuit breaker state machine.
func (cb *CircuitBreaker) Execute(fn func() error) error {
	cb.mu.Lock()

	now := time.Now()
	if cb.state == StateOpen {
		if now.Sub(cb.lastStateChange) > cb.timeout {
			cb.state = StateHalfOpen
			cb.consecutiveWins = 0
			cb.lastStateChange = now
		} else {
			cb.mu.Unlock()
			return ErrCircuitOpen
		}
	}

	cb.mu.Unlock()

	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.consecutiveFails++
		cb.consecutiveWins = 0

		if cb.state == StateHalfOpen || cb.consecutiveFails >= cb.failureThreshold {
			cb.state = StateOpen
			cb.lastStateChange = time.Now()
		}
		return err
	}

	// Success
	cb.consecutiveFails = 0
	if cb.state == StateHalfOpen {
		cb.consecutiveWins++
		if cb.consecutiveWins >= cb.successThreshold {
			cb.state = StateClosed
			cb.lastStateChange = time.Now()
		}
	}

	return nil
}

func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}
