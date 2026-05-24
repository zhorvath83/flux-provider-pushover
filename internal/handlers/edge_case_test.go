package handlers

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/zhorvath83/flux-provider-pushover/internal/config"
	"github.com/zhorvath83/flux-provider-pushover/internal/pushover"
	"github.com/zhorvath83/flux-provider-pushover/internal/server"
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
	return nil, nil
}

func TestServer_StartError(t *testing.T) {
	cfg := &config.Config{
		Port: ":-1",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	logger := &MockLogger{}
	srv := server.NewServer(cfg, handler, logger)

	err := srv.Start()
	if err == nil {
		t.Error("Expected error for invalid port")
	}
}

func TestServer_ShutdownWithError(t *testing.T) {
	cfg := &config.Config{
		Port: ":0",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	logger := &MockLogger{}
	srv := server.NewServer(cfg, handler, logger)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

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

func TestPushoverClient_SendMessage_EdgeCases(t *testing.T) {
	longErrorMessage := strings.Repeat("x", 1024)

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

	if !strings.Contains(err.Error(), "pushover API returned status 400") {
		t.Errorf("Unexpected error message: %v", err)
	}
}
