package circuitbreaker

import (
	"errors"
	"sync"
	"time"
)

// State represents the circuit breaker's operating state.
type State int

const (
	StateClosed   State = iota // normal operation
	StateOpen                  // rejecting calls
	StateHalfOpen              // probing with one call
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF_OPEN"
	default:
		return "UNKNOWN"
	}
}

var ErrCircuitOpen = errors.New("circuit breaker is open")

// CircuitBreaker protects a downstream service from cascading failures.
// After maxFailures consecutive failures, it opens and blocks calls for resetTimeout.
// After resetTimeout, it allows one probe call; success closes it, failure re-opens it.
type CircuitBreaker struct {
	mu              sync.Mutex
	state           State
	consecutiveFails int
	maxFailures     int
	resetTimeout    time.Duration
	openedAt        time.Time
}

func New(maxFailures int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:        StateClosed,
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
	}
}

// Call executes fn within the circuit breaker's protection.
func (cb *CircuitBreaker) Call(fn func() error) error {
	cb.mu.Lock()
	switch cb.state {
	case StateOpen:
		if time.Since(cb.openedAt) >= cb.resetTimeout {
			cb.state = StateHalfOpen
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
		cb.onFailure()
	} else {
		cb.onSuccess()
	}
	return err
}

func (cb *CircuitBreaker) onFailure() {
	cb.consecutiveFails++
	if cb.state == StateHalfOpen || cb.consecutiveFails >= cb.maxFailures {
		cb.state = StateOpen
		cb.openedAt = time.Now()
		cb.consecutiveFails = 0
	}
}

func (cb *CircuitBreaker) onSuccess() {
	cb.state = StateClosed
	cb.consecutiveFails = 0
}

func (cb *CircuitBreaker) CurrentState() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}
