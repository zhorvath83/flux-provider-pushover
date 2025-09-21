package pushover

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/zhorvath83/flux-provider-pushover/internal/types"
)

// HTTPClient interface for dependency injection
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// PushoverClient handles communication with Pushover API
type PushoverClient struct {
	client HTTPClient
	url    string
}

// NewPushoverClient creates a new Pushover client
func NewPushoverClient(client HTTPClient, url string) *PushoverClient {
	return &PushoverClient{
		client: client,
		url:    url,
	}
}

// SendMessage sends a message to Pushover API
func (p *PushoverClient) SendMessage(ctx context.Context, msg *types.PushoverMessage) error {
	if msg == nil {
		return fmt.Errorf("message is nil")
	}

	data := url.Values{}
	data.Set("token", msg.Token)
	data.Set("user", msg.User)
	data.Set("message", msg.Message)
	data.Set("title", msg.Title)

	req, err := http.NewRequestWithContext(ctx, "POST", p.url, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", types.ContentTypeForm)

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("pushover API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Discard response body
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
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
