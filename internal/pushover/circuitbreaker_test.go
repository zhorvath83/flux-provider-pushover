package pushover

import (
	"sync"
	"testing"
	"time"
)

func TestCircuitBreaker_ClosedToOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 3,
		Timeout:          1 * time.Second,
		SuccessThreshold: 1,
	})

	if cb.State() != CircuitClosed {
		t.Error("Expected initial state to be closed")
	}

	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if cb.State() != CircuitOpen {
		t.Error("Expected state to be open after 3 failures")
	}
}

func TestCircuitBreaker_OpenToHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 1,
		Timeout:          50 * time.Millisecond,
		SuccessThreshold: 1,
	})

	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Error("Expected open after failure")
	}

	time.Sleep(60 * time.Millisecond)

	if err := cb.Allow(); err != nil {
		t.Error("Expected Allow to succeed after timeout")
	}
	if cb.State() != CircuitHalfOpen {
		t.Error("Expected half-open state after timeout")
	}
}

func TestCircuitBreaker_HalfOpenToClosed(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 1,
		Timeout:          50 * time.Millisecond,
		SuccessThreshold: 2,
	})

	cb.RecordFailure()
	time.Sleep(60 * time.Millisecond)

	if err := cb.Allow(); err != nil {
		t.Fatal("Expected Allow after timeout")
	}
	cb.RecordSuccess()
	if cb.State() != CircuitHalfOpen {
		t.Error("Expected half-open after 1 success (need 2)")
	}

	if err := cb.Allow(); err != nil {
		t.Fatal("Expected Allow for second probe")
	}
	cb.RecordSuccess()
	if cb.State() != CircuitClosed {
		t.Error("Expected closed after 2 successes")
	}
}

func TestCircuitBreaker_HalfOpenToOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 1,
		Timeout:          50 * time.Millisecond,
		SuccessThreshold: 2,
	})

	cb.RecordFailure()
	time.Sleep(60 * time.Millisecond)
	if err := cb.Allow(); err != nil {
		t.Fatal("Expected Allow after timeout")
	}

	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Error("Expected open after failure in half-open")
	}
}

func TestCircuitBreaker_OpenRejects(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 1,
		Timeout:          1 * time.Hour,
		SuccessThreshold: 1,
	})

	cb.RecordFailure()
	if err := cb.Allow(); err == nil {
		t.Error("Expected Allow to reject when open")
	}
}

func TestCircuitBreaker_Concurrent(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 5,
		Timeout:          1 * time.Second,
		SuccessThreshold: 2,
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cb.Allow()
			cb.RecordSuccess()
		}()
	}
	wg.Wait()

	if cb.State() != CircuitClosed {
		t.Errorf("Expected closed after all successes, got %d", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenLimitsProbes(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 1,
		Timeout:          50 * time.Millisecond,
		SuccessThreshold: 2,
	})

	cb.RecordFailure()
	time.Sleep(60 * time.Millisecond)

	// First two probes should be allowed (SuccessThreshold = 2)
	if err := cb.Allow(); err != nil {
		t.Fatalf("First probe should be allowed: %v", err)
	}
	if err := cb.Allow(); err != nil {
		t.Fatalf("Second probe should be allowed: %v", err)
	}

	// Third probe should be rejected — 2 probes already in-flight covers SuccessThreshold
	if err := cb.Allow(); err == nil {
		t.Error("Expected third probe to be rejected while 2 in-flight")
	}

	// Complete one probe with failure — circuit reopens, halfOpenInFlight resets
	cb.RecordFailure()
	time.Sleep(60 * time.Millisecond)

	// After timeout, should allow probes again
	if err := cb.Allow(); err != nil {
		t.Fatalf("Should allow after circuit re-enters half-open: %v", err)
	}
}

func TestCircuitBreaker_SuccessDecrementsFailureCount(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 3,
		Timeout:          1 * time.Second,
		SuccessThreshold: 1,
	})

	// 2 failures — not enough to open
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != CircuitClosed {
		t.Error("Expected closed after 2 failures (threshold is 3)")
	}

	// 1 success decrements failure count instead of resetting
	cb.RecordSuccess()
	if cb.State() != CircuitClosed {
		t.Error("Expected still closed after success")
	}

	// Now 2 more failures should open (count goes 1→2→3)
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Error("Expected open after 3 total failures")
	}
}