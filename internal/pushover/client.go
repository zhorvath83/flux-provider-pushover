package pushover

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/zhorvath83/flux-provider-pushover/internal/types"
)

// RetryConfig configures the retry behavior for Pushover API calls.
type RetryConfig struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
	MaxAttempts  int
}

// DefaultRetryConfig provides sensible defaults for retry behavior.
var DefaultRetryConfig = RetryConfig{
	InitialDelay: 200 * time.Millisecond,
	MaxDelay:     2 * time.Second,
	Multiplier:   2.0,
	MaxAttempts:  3,
}

// HTTPClient interface for dependency injection
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// PushoverClient handles communication with Pushover API
type PushoverClient struct {
	client   HTTPClient
	url      string
	retryCfg RetryConfig
	circuit  *CircuitBreaker
}

// NewPushoverClient creates a new Pushover client
func NewPushoverClient(client HTTPClient, url string) *PushoverClient {
	return &PushoverClient{
		client:   client,
		url:      url,
		retryCfg: DefaultRetryConfig,
		circuit:  NewCircuitBreaker(DefaultCircuitBreakerConfig),
	}
}

// SendMessage sends a message to Pushover API with retry on transient failures.
func (p *PushoverClient) SendMessage(ctx context.Context, msg *types.PushoverMessage) error {
	if msg == nil {
		return fmt.Errorf("message is nil")
	}

	if err := p.circuit.Allow(); err != nil {
		return err
	}

	data := url.Values{}
	data.Set("token", msg.Token)
	data.Set("user", msg.User)
	data.Set("message", msg.Message)
	data.Set("title", msg.Title)

	var lastErr error
	for attempt := 0; attempt < p.retryCfg.MaxAttempts; attempt++ {
		if attempt > 0 {
			delay := p.backoffDelay(attempt)
			select {
			case <-ctx.Done():
				p.circuit.Release()
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		lastErr = p.doSend(ctx, data)
		if lastErr == nil {
			p.circuit.RecordSuccess()
			return nil
		}
		if !isRetryable(lastErr) {
			p.circuit.RecordFailure()
			return lastErr
		}
	}
	p.circuit.RecordFailure()
	return lastErr
}

// doSend performs a single attempt to send the message.
func (p *PushoverClient) doSend(ctx context.Context, data url.Values) error {
	req, err := http.NewRequestWithContext(ctx, "POST", p.url, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", types.ContentTypeForm)

	resp, err := p.client.Do(req)
	if err != nil {
		return &networkError{err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return nil
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))

	if isRetryableStatus(resp.StatusCode) {
		return &retryableError{status: resp.StatusCode, body: string(body)}
	}
	return fmt.Errorf("pushover API returned status %d: %s", resp.StatusCode, string(body))
}

// backoffDelay calculates the delay with exponential backoff and jitter.
func (p *PushoverClient) backoffDelay(attempt int) time.Duration {
	delay := float64(p.retryCfg.InitialDelay) * math.Pow(p.retryCfg.Multiplier, float64(attempt-1))
	if delay > float64(p.retryCfg.MaxDelay) {
		delay = float64(p.retryCfg.MaxDelay)
	}
	// Subtract up to 50% random jitter to avoid thundering herd on retries.
	jitter := delay * 0.5 * rand.Float64()
	delay = delay - jitter
	return time.Duration(delay)
}

// isRetryable determines if an error is transient and worth retrying.
func isRetryable(err error) bool {
	var re *retryableError
	var ne *networkError
	return errors.As(err, &re) || errors.As(err, &ne)
}

// networkError wraps a network-level error from HTTP client.
type networkError struct {
	err error
}

func (e *networkError) Error() string {
	return fmt.Sprintf("failed to send request: %s", e.err)
}

func (e *networkError) Unwrap() error {
	return e.err
}

// isRetryableStatus checks if an HTTP status code indicates a transient failure.
func isRetryableStatus(code int) bool {
	return code == http.StatusTooManyRequests || code >= 500
}

// retryableError marks an error as retryable.
type retryableError struct {
	status int
	body   string
}

func (e *retryableError) Error() string {
	return fmt.Sprintf("pushover API returned status %d: %s", e.status, e.body)
}

// CreateOptimizedHTTPClient creates an optimized HTTP client
func CreateOptimizedHTTPClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}
