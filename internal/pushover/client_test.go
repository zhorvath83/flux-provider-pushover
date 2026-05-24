package pushover

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/zhorvath83/flux-provider-pushover/internal/types"
)

// MockHTTPClient is a mock implementation of HTTPClient
type MockHTTPClient struct {
	DoFunc func(req *http.Request) (*http.Response, error)
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if m.DoFunc != nil {
		return m.DoFunc(req)
	}
	return nil, fmt.Errorf("MockHTTPClient.DoFunc not set")
}

func TestNewPushoverClient(t *testing.T) {
	mockClient := &MockHTTPClient{}
	url := "http://test.example.com"

	client := NewPushoverClient(mockClient, url)

	if client.client != mockClient {
		t.Error("Client was not set correctly")
	}

	if client.url != url {
		t.Errorf("URL was not set correctly: expected %s, got %s", url, client.url)
	}
}

func TestPushoverClient_SendMessage(t *testing.T) {
	tests := []struct {
		name          string
		msg           *types.PushoverMessage
		mockResponse  *http.Response
		mockError     error
		expectedError bool
		errorContains string
	}{
		{
			name: "successful send",
			msg: &types.PushoverMessage{
				Token:   "test_token",
				User:    "test_user",
				Title:   "Test Title",
				Message: "Test message",
			},
			mockResponse: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"status":1}`)),
			},
			expectedError: false,
		},
		{
			name:          "nil message",
			msg:           nil,
			expectedError: true,
			errorContains: "message is nil",
		},
		{
			name: "API error response",
			msg: &types.PushoverMessage{
				Token:   "test_token",
				User:    "test_user",
				Title:   "Test Title",
				Message: "Test message",
			},
			mockResponse: &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader(`{"error":"Invalid token"}`)),
			},
			expectedError: true,
			errorContains: "pushover API returned status 400",
		},
		{
			name: "network error",
			msg: &types.PushoverMessage{
				Token:   "test_token",
				User:    "test_user",
				Title:   "Test Title",
				Message: "Test message",
			},
			mockError:     fmt.Errorf("network error"),
			expectedError: true,
			errorContains: "failed to send request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockHTTPClient{
				DoFunc: func(req *http.Request) (*http.Response, error) {
					if tt.mockError != nil {
						return nil, tt.mockError
					}

					// Verify request properties
					if req.Method != "POST" {
						t.Errorf("Expected POST method, got %s", req.Method)
					}

					if req.Header.Get("Content-Type") != types.ContentTypeForm {
						t.Errorf("Expected Content-Type %s, got %s",
							types.ContentTypeForm, req.Header.Get("Content-Type"))
					}

					// Parse form data if message is not nil
					if tt.msg != nil {
						body, _ := io.ReadAll(req.Body)
						if !strings.Contains(string(body), "token="+tt.msg.Token) {
							t.Error("Token not found in request body")
						}
						if !strings.Contains(string(body), "user="+tt.msg.User) {
							t.Error("User not found in request body")
						}
					}

					return tt.mockResponse, nil
				},
			}

			client := NewPushoverClient(mockClient, "http://test.example.com")
			client.retryCfg = RetryConfig{MaxAttempts: 1}
			ctx := context.Background()

			err := client.SendMessage(ctx, tt.msg)

			if tt.expectedError {
				if err == nil {
					t.Error("Expected error but got nil")
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error containing '%s', got '%s'",
						tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestPushoverClient_SendMessage_Context(t *testing.T) {
	// Test with cancelled context
	msg := &types.PushoverMessage{
		Token:   "test_token",
		User:    "test_user",
		Title:   "Test Title",
		Message: "Test message",
	}

	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			// Verify context is passed through
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			default:
				t.Error("Context should have been cancelled")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		},
	}

	client := NewPushoverClient(mockClient, "http://test.example.com")
	client.retryCfg = RetryConfig{MaxAttempts: 1}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := client.SendMessage(ctx, msg)
	if err == nil {
		t.Error("Expected context cancellation error")
	}
}

func TestCreateOptimizedHTTPClient(t *testing.T) {
	timeout := 5 * time.Second
	client := CreateOptimizedHTTPClient(timeout)

	if client.Timeout != timeout {
		t.Errorf("Expected timeout %v, got %v", timeout, client.Timeout)
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Expected http.Transport")
	}

	if transport.MaxIdleConns != 10 {
		t.Errorf("Expected MaxIdleConns 10, got %d", transport.MaxIdleConns)
	}

	if transport.MaxIdleConnsPerHost != 2 {
		t.Errorf("Expected MaxIdleConnsPerHost 2, got %d", transport.MaxIdleConnsPerHost)
	}

	if transport.DisableCompression != true {
		t.Error("Expected DisableCompression to be true")
	}
}

// Benchmark tests
func BenchmarkPushoverClient_SendMessage(b *testing.B) {
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"status":1}`)),
			}, nil
		},
	}

	client := NewPushoverClient(mockClient, "http://test.example.com")
	client.retryCfg = RetryConfig{MaxAttempts: 1}
	ctx := context.Background()

	msg := &types.PushoverMessage{
		Token:   "test_token",
		User:    "test_user",
		Title:   "Test Title",
		Message: "Benchmark test message",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = client.SendMessage(ctx, msg)
	}
}

func TestPushoverClient_Retry_SuccessAfter5xx(t *testing.T) {
	callCount := 0
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			callCount++
			if callCount < 3 {
				return &http.Response{
					StatusCode: http.StatusServiceUnavailable,
					Body:       io.NopCloser(strings.NewReader("temporarily unavailable")),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"status":1}`)),
			}, nil
		},
	}

	client := NewPushoverClient(mockClient, "http://test.example.com")
	client.retryCfg = RetryConfig{
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     5 * time.Millisecond,
		Multiplier:   2.0,
		MaxAttempts:  3,
	}

	msg := &types.PushoverMessage{Token: "t", User: "u", Title: "T", Message: "M"}
	err := client.SendMessage(context.Background(), msg)
	if err != nil {
		t.Errorf("Expected success after retry, got: %v", err)
	}
	if callCount != 3 {
		t.Errorf("Expected 3 attempts, got %d", callCount)
	}
}

func TestPushoverClient_Retry_Exhausted(t *testing.T) {
	callCount := 0
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			callCount++
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader("error")),
			}, nil
		},
	}

	client := NewPushoverClient(mockClient, "http://test.example.com")
	client.retryCfg = RetryConfig{
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     5 * time.Millisecond,
		Multiplier:   2.0,
		MaxAttempts:  3,
	}

	msg := &types.PushoverMessage{Token: "t", User: "u", Title: "T", Message: "M"}
	err := client.SendMessage(context.Background(), msg)
	if err == nil {
		t.Error("Expected error after exhausted retries")
	}
	if callCount != 3 {
		t.Errorf("Expected 3 attempts, got %d", callCount)
	}
}

func TestPushoverClient_Retry_4xxNoRetry(t *testing.T) {
	callCount := 0
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			callCount++
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader(`{"errors":["invalid token"]}`)),
			}, nil
		},
	}

	client := NewPushoverClient(mockClient, "http://test.example.com")
	client.retryCfg = RetryConfig{
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     5 * time.Millisecond,
		Multiplier:   2.0,
		MaxAttempts:  3,
	}

	msg := &types.PushoverMessage{Token: "t", User: "u", Title: "T", Message: "M"}
	err := client.SendMessage(context.Background(), msg)
	if err == nil {
		t.Error("Expected error for 400")
	}
	if callCount != 1 {
		t.Errorf("4xx should not be retried, expected 1 attempt, got %d", callCount)
	}
}

func TestPushoverClient_Retry_CancelledContext(t *testing.T) {
	callCount := 0
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			callCount++
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader("error")),
			}, nil
		},
	}

	client := NewPushoverClient(mockClient, "http://test.example.com")
	client.retryCfg = RetryConfig{
		InitialDelay: 50 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
		MaxAttempts:  5,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	msg := &types.PushoverMessage{Token: "t", User: "u", Title: "T", Message: "M"}
	err := client.SendMessage(ctx, msg)
	if err == nil {
		t.Error("Expected error due to context cancellation")
	}
}

func TestIsRetryable_WrappedError(t *testing.T) {
	// errors.As should match even when the error is wrapped
	inner := &networkError{err: fmt.Errorf("connection refused")}
	wrapped := fmt.Errorf("pushover call failed: %w", inner)

	if !isRetryable(wrapped) {
		t.Error("Expected wrapped networkError to be retryable")
	}

	nonRetryable := fmt.Errorf("some other error: %w", fmt.Errorf("not retryable"))
	if isRetryable(nonRetryable) {
		t.Error("Expected non-retryable wrapped error to not be retryable")
	}
}

func TestIsRetryable_DirectTypes(t *testing.T) {
	if !isRetryable(&networkError{err: fmt.Errorf("timeout")}) {
		t.Error("networkError should be retryable")
	}
	if !isRetryable(&retryableError{status: 500, body: "error"}) {
		t.Error("retryableError should be retryable")
	}
	if isRetryable(fmt.Errorf("plain error")) {
		t.Error("plain error should not be retryable")
	}
}

func TestRetryableError_Error(t *testing.T) {
	err := &retryableError{status: 503, body: "service unavailable"}
	expected := "pushover API returned status 503: service unavailable"
	if err.Error() != expected {
		t.Errorf("Expected %q, got %q", expected, err.Error())
	}
}

func TestBackoffDelay(t *testing.T) {
	cfg := RetryConfig{
		InitialDelay: 200 * time.Millisecond,
		MaxDelay:     2 * time.Second,
		Multiplier:   2.0,
		MaxAttempts:  5,
	}
	client := NewPushoverClient(&MockHTTPClient{}, "http://test.example.com")
	client.retryCfg = cfg

	tests := []struct {
		attempt    int
		minDelayMs int64
		maxDelayMs int64
	}{
		{1, 100, 200},  // 200ms * 2^0 = 200ms, -50% jitter → 100-200ms
		{2, 200, 400},  // 200ms * 2^1 = 400ms, -50% jitter → 200-400ms
		{3, 400, 800},  // 200ms * 2^2 = 800ms, -50% jitter → 400-800ms
		{4, 800, 2000}, // 200ms * 2^3 = 1600ms, capped at 2000, → 800-2000ms
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
			delay := client.backoffDelay(tt.attempt)
			minDelay := time.Duration(tt.minDelayMs) * time.Millisecond
			maxDelay := time.Duration(tt.maxDelayMs) * time.Millisecond

			if delay < minDelay {
				t.Errorf("Attempt %d: delay %v is below minimum %v", tt.attempt, delay, minDelay)
			}
			if delay > maxDelay {
				t.Errorf("Attempt %d: delay %v exceeds maximum %v", tt.attempt, delay, maxDelay)
			}
		})
	}
}

func TestBackoffDelay_CappedAtMax(t *testing.T) {
	cfg := RetryConfig{
		InitialDelay: 1 * time.Second,
		MaxDelay:     2 * time.Second,
		Multiplier:   2.0,
		MaxAttempts:  10,
	}
	client := NewPushoverClient(&MockHTTPClient{}, "http://test.example.com")
	client.retryCfg = cfg

	for attempt := 1; attempt <= 10; attempt++ {
		delay := client.backoffDelay(attempt)
		if delay > cfg.MaxDelay {
			t.Errorf("Attempt %d: delay %v exceeds max %v", attempt, delay, cfg.MaxDelay)
		}
	}
}

func TestIsRetryableStatus(t *testing.T) {
	tests := []struct {
		code       int
		retryable  bool
	}{
		{http.StatusOK, false},
		{http.StatusBadRequest, false},
		{http.StatusUnauthorized, false},
		{http.StatusForbidden, false},
		{http.StatusNotFound, false},
		{http.StatusTooManyRequests, true},
		{http.StatusInternalServerError, true},
		{http.StatusBadGateway, true},
		{http.StatusServiceUnavailable, true},
		{http.StatusGatewayTimeout, true},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d", tt.code), func(t *testing.T) {
			result := isRetryableStatus(tt.code)
			if result != tt.retryable {
				t.Errorf("isRetryableStatus(%d) = %v, want %v", tt.code, result, tt.retryable)
			}
		})
	}
}

func TestPushoverClient_CircuitBreakerIntegration(t *testing.T) {
	callCount := 0
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			callCount++
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Body:       io.NopCloser(strings.NewReader("unavailable")),
			}, nil
		},
	}

	client := NewPushoverClient(mockClient, "http://test.example.com")
	client.retryCfg = RetryConfig{
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     5 * time.Millisecond,
		Multiplier:   2.0,
		MaxAttempts:  3,
	}

	msg := &types.PushoverMessage{Token: "t", User: "u", Title: "T", Message: "M"}

	// Exhaust retries 5 times to trigger circuit breaker (FailureThreshold=5)
	for i := 0; i < 5; i++ {
		client.SendMessage(context.Background(), msg)
	}

	callCount = 0

	// Circuit should now be open — calls should be rejected immediately
	err := client.SendMessage(context.Background(), msg)
	if err == nil {
		t.Error("Expected circuit breaker to reject request")
	}
	if callCount > 0 {
		t.Errorf("Expected no HTTP calls when circuit is open, got %d", callCount)
	}
}

func TestNetworkError_ErrorAndUnwrap(t *testing.T) {
	inner := fmt.Errorf("connection refused")
	err := &networkError{err: inner}

	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("Expected error to contain 'connection refused', got %q", err.Error())
	}

	if err.Unwrap() != inner {
		t.Error("Unwrap should return the inner error")
	}
}

func TestPushoverClient_ContextCancellationReleasesCircuit(t *testing.T) {
	// When a request is cancelled during retry delay, the circuit breaker
	// should release the probe slot (not leak halfOpenInFlight).
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader("error")),
			}, nil
		},
	}

	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 1,
		Timeout:          50 * time.Millisecond,
		SuccessThreshold: 2,
	})

	client := NewPushoverClient(mockClient, "http://test.example.com")
	client.circuit = cb
	client.retryCfg = RetryConfig{
		InitialDelay: 50 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
		MaxAttempts:  3,
	}

	// Open the circuit
	cb.RecordFailure()

	// Wait for half-open transition
	time.Sleep(60 * time.Millisecond)

	// Now we're in half-open. Allow a probe.
	if err := cb.Allow(); err != nil {
		t.Fatalf("Expected Allow in half-open: %v", err)
	}

	// Release the probe (simulating context cancellation)
	cb.Release()

	// Should still be able to allow another probe (halfOpenInFlight was decremented)
	if err := cb.Allow(); err != nil {
		t.Errorf("Expected Allow after Release: %v", err)
	}

	// Without Release, the second Allow would be blocked because
	// successCount(0) + halfOpenInFlight(1) >= SuccessThreshold(2) would be false,
	// but after Release halfOpenInFlight=0, so Allow succeeds again.
}

func TestPushoverClient_SendMessage_InvalidURL(t *testing.T) {
	client := NewPushoverClient(&MockHTTPClient{}, "http://\x00invalid")
	client.retryCfg = RetryConfig{MaxAttempts: 1}

	msg := &types.PushoverMessage{Token: "t", User: "u", Title: "T", Message: "M"}
	err := client.SendMessage(context.Background(), msg)
	if err == nil {
		t.Fatal("Expected error for invalid URL")
	}
	if !strings.Contains(err.Error(), "failed to create request") {
		t.Errorf("Expected 'failed to create request' error, got: %v", err)
	}
}

func TestPushoverClient_ContextCancelDuringRetryCallsRelease(t *testing.T) {
	// Verify that SendMessage calls circuit.Release() when context is
	// cancelled during a retry delay, preventing halfOpenInFlight leak.
	callCount := 0
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			callCount++
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader("error")),
			}, nil
		},
	}

	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 1,
		Timeout:          50 * time.Millisecond,
		SuccessThreshold: 2,
	})

	client := NewPushoverClient(mockClient, "http://test.example.com")
	client.circuit = cb
	client.retryCfg = RetryConfig{
		InitialDelay: 200 * time.Millisecond,
		MaxDelay:     500 * time.Millisecond,
		Multiplier:   2.0,
		MaxAttempts:  3,
	}

	// Open the circuit
	cb.RecordFailure()
	time.Sleep(60 * time.Millisecond) // Wait for half-open

	// Send a message with a short context timeout — will cancel during retry delay
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	msg := &types.PushoverMessage{Token: "t", User: "u", Title: "T", Message: "M"}
	err := client.SendMessage(ctx, msg)
	if err == nil {
		t.Error("Expected error from context cancellation")
	}

	// The circuit should still allow probes (halfOpenInFlight was released)
	if err := cb.Allow(); err != nil {
		t.Errorf("Expected circuit to still allow probes after context cancellation: %v", err)
	}
}
