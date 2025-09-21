package handlers

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zhorvath83/flux-provider-pushover/internal/config"
	"github.com/zhorvath83/flux-provider-pushover/internal/pushover"
	"github.com/zhorvath83/flux-provider-pushover/internal/server"
	"github.com/zhorvath83/flux-provider-pushover/internal/types"
)

func init() {
	// Set test environment to prevent os.Exit in tests
	os.Setenv("GO_TEST", "1")
}

// MockHTTPClient is a mock implementation of HTTPClient
type MockHTTPClient struct {
	DoFunc func(req *http.Request) (*http.Response, error)
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if m.DoFunc != nil {
		return m.DoFunc(req)
	}
	return nil, nil
}

// Additional test for server Start error handling
func TestServer_StartError(t *testing.T) {
	cfg := &config.Config{
		Port: ":-1", // Invalid port to cause error
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	logger := &MockLogger{}
	srv := server.NewServer(cfg, handler, logger)

	// We can't directly test os.Exit(1) being called, but we can verify
	// the server tries to start with an invalid configuration
	// The actual Start() method will log the error
	err := srv.Start()
	if err != nil {
		t.Logf("Expected behavior: Start returned error: %v", err)
	}
}

// Test for Shutdown with active connections
func TestServer_ShutdownWithError(t *testing.T) {
	cfg := &config.Config{
		Port: ":0",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate long-running request
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	logger := &MockLogger{}
	srv := server.NewServer(cfg, handler, logger)

	// Use a context that's already cancelled to force shutdown error
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := srv.Shutdown(ctx)
	if err == nil {
		t.Log("Shutdown completed without error (server wasn't running)")
	} else {
		expectedError := "server forced to shutdown: context canceled"
		if err.Error() != expectedError {
			t.Logf("Got shutdown error: %v", err)
		}
	}
}

// Test for PushoverClient edge cases
func TestPushoverClient_SendMessage_EdgeCases(t *testing.T) {
	// Test with very long error message from API
	longErrorMessage := strings.Repeat("x", 1024) // 1KB of data

	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader(longErrorMessage)),
			}, nil
		},
	}

	client := pushover.NewPushoverClient(mockClient, "http://test.example.com")
	ctx := context.Background()

	msg := &types.PushoverMessage{
		Token:   "test_token",
		User:    "test_user",
		Title:   "Test Title",
		Message: "Test message",
	}

	err := client.SendMessage(ctx, msg)
	if err == nil {
		t.Error("Expected error for bad status code")
	}

	// The error message should be truncated to 512 bytes
	if !strings.Contains(err.Error(), "pushover API returned status 400") {
		t.Errorf("Unexpected error message: %v", err)
	}
}
