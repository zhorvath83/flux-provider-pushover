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
	return m.DoFunc(req)
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
