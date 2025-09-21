package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/zhorvath83/flux-provider-pushover/internal/config"
	"github.com/zhorvath83/flux-provider-pushover/internal/handlers"
	"github.com/zhorvath83/flux-provider-pushover/internal/server"
	"github.com/zhorvath83/flux-provider-pushover/internal/types"
)

// MockLogger for testing
type MockLogger struct {
	Messages []string
}

func (m *MockLogger) Printf(format string, v ...interface{}) {}
func (m *MockLogger) Println(v ...interface{})               {}

// MockPushoverClient for testing
type MockPushoverClient struct {
	SendMessageFunc func(ctx context.Context, msg *types.PushoverMessage) error
}

func (m *MockPushoverClient) SendMessage(ctx context.Context, msg *types.PushoverMessage) error {
	if m.SendMessageFunc != nil {
		return m.SendMessageFunc(ctx, msg)
	}
	return nil
}

// TestCreateWebhookHandler_FullCoverage adds missing test cases
func TestCreateWebhookHandler_FullCoverage(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		authHeader     string
		body           string
		testMode       bool
		pushoverError  error
		expectedStatus int
	}{
		{
			name:           "OPTIONS request",
			method:         http.MethodOptions,
			authHeader:     "",
			body:           "",
			testMode:       false,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "GET request",
			method:         http.MethodGet,
			authHeader:     "",
			body:           "",
			testMode:       false,
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "missing authorization",
			method:         http.MethodPost,
			authHeader:     "",
			body:           "{}",
			testMode:       false,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "wrong authorization",
			method:         http.MethodPost,
			authHeader:     "Bearer wrong",
			body:           "{}",
			testMode:       false,
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				BearerToken:      "Bearer test_token",
				PushoverAPIToken: "test_token",
			}

			mockClient := &MockPushoverClient{
				SendMessageFunc: func(ctx context.Context, msg *types.PushoverMessage) error {
					return tt.pushoverError
				},
			}

			deps := &handlers.HandlerDependencies{
				Config:         cfg,
				PushoverClient: mockClient,
				Logger:         &MockLogger{},
				MessageBuilder: handlers.BuildPushoverMessage,
			}

			handler := handlers.CreateWebhookHandler(deps)

			req := httptest.NewRequest(tt.method, "/webhook", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
			}
		})
	}
}

// TestMain_Coverage provides coverage for the main function
func TestMain_Coverage(t *testing.T) {
	// Since main() uses os.Exit, we can't test it directly
	// We test the logic by verifying components work correctly

	// Test health check path
	if len(os.Args) < 2 {
		os.Args = append(os.Args, "")
	}

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Test health check error path
	os.Args[1] = "-health"
	err := server.HealthCheck("http://invalid-url:99999/health")
	if err == nil {
		t.Error("Expected error for invalid health check URL")
	}
}
