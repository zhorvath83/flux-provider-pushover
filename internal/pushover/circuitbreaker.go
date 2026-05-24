package pushover

import (
	"fmt"
	"sync"
	"time"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	CircuitClosed   CircuitState = iota // Normal operation
	CircuitOpen                          // Failing, reject requests
	CircuitHalfOpen                      // Testing if service recovered
)

// CircuitBreakerConfig configures the circuit breaker behavior.
type CircuitBreakerConfig struct {
	FailureThreshold int           // Failures to trigger open
	Timeout          time.Duration // Time before half-open
	SuccessThreshold int           // Successes in half-open to close
}

// DefaultCircuitBreakerConfig provides sensible defaults.
var DefaultCircuitBreakerConfig = CircuitBreakerConfig{
	FailureThreshold: 5,
	Timeout:          30 * time.Second,
	SuccessThreshold: 2,
}

// CircuitBreaker implements the circuit breaker pattern.
type CircuitBreaker struct {
	mu               sync.Mutex
	cfg              CircuitBreakerConfig
	state            CircuitState
	failureCount     int
	successCount     int
	lastFailureTime  time.Time
	halfOpenInFlight int
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{cfg: cfg}
}

// Allow checks if a request is allowed through the circuit breaker.
// In half-open state, at most SuccessThreshold concurrent probes are allowed.
func (cb *CircuitBreaker) Allow() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return nil
	case CircuitOpen:
		if time.Since(cb.lastFailureTime) > cb.cfg.Timeout {
			cb.state = CircuitHalfOpen
			cb.successCount = 0
			cb.halfOpenInFlight = 0
		} else {
			return fmt.Errorf("circuit breaker is open")
		}
		fallthrough
	case CircuitHalfOpen:
		if cb.successCount+cb.halfOpenInFlight >= cb.cfg.SuccessThreshold {
			return fmt.Errorf("circuit breaker is half-open, probing in progress")
		}
		cb.halfOpenInFlight++
		return nil
	default:
		return nil
	}
}

// RecordSuccess records a successful operation.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitHalfOpen:
		if cb.halfOpenInFlight > 0 {
			cb.halfOpenInFlight--
		}
		cb.successCount++
		if cb.successCount >= cb.cfg.SuccessThreshold {
			cb.state = CircuitClosed
			cb.failureCount = 0
			cb.halfOpenInFlight = 0
		}
	case CircuitClosed:
		if cb.failureCount > 0 {
			cb.failureCount--
		}
	}
}

// RecordFailure records a failed operation.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.lastFailureTime = time.Now()

	switch cb.state {
	case CircuitClosed:
		cb.failureCount++
		if cb.failureCount >= cb.cfg.FailureThreshold {
			cb.state = CircuitOpen
		}
	case CircuitHalfOpen:
		if cb.halfOpenInFlight > 0 {
			cb.halfOpenInFlight--
		}
		cb.state = CircuitOpen
		cb.successCount = 0
	}
}

// Release decrements the half-open in-flight counter without recording
// success or failure. Call this when a probe is abandoned due to external
// cancellation (e.g., context timeout), not due to service failure.
func (cb *CircuitBreaker) Release() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.state == CircuitHalfOpen && cb.halfOpenInFlight > 0 {
		cb.halfOpenInFlight--
	}
}

// State returns the current circuit state (for testing/monitoring).
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}