package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zhorvath83/flux-provider-pushover/internal/config"
	"github.com/zhorvath83/flux-provider-pushover/internal/types"
)

// MockLogger for testing
type MockLogger struct {
	messages []string
}

func (m *MockLogger) Printf(format string, v ...interface{}) {
	// Store formatted messages for verification in tests
	m.messages = append(m.messages, format)
}

func (m *MockLogger) Println(v ...interface{}) {
	// Store messages for verification in tests
	m.messages = append(m.messages, "println")
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

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

func TestCreateRootHandler(t *testing.T) {
	handler := CreateRootHandler()

	req, _ := http.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}

	if !bytes.Equal(rr.Body.Bytes(), types.ResponseRootError) {
		t.Errorf("Expected body %s, got %s", types.ResponseRootError, rr.Body.String())
	}
}

func TestCreateHealthHandler(t *testing.T) {
	handler := CreateHealthHandler()

	req, _ := http.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}

	if !bytes.Equal(rr.Body.Bytes(), types.ResponseHealthy) {
		t.Errorf("Expected body %s, got %s", types.ResponseHealthy, rr.Body.String())
	}
}

func TestCreateWebhookHandler(t *testing.T) {
	tests := []struct {
		name             string
		authHeader       string
		body             interface{}
		pushoverError    error
		expectedStatus   int
		expectedResponse []byte
		testMode         bool
	}{
		{
			name:             "unauthorized request",
			authHeader:       "Bearer wrong_token",
			expectedStatus:   http.StatusUnauthorized,
			expectedResponse: types.ResponseUnauthorized,
		},
		{
			name:             "invalid JSON",
			authHeader:       "Bearer test_token",
			body:             "invalid json",
			expectedStatus:   http.StatusBadRequest,
			expectedResponse: types.ResponseInvalidJSON,
		},
		{
			name:       "valid request in test mode",
			authHeader: "Bearer test_api_token",
			body: types.FluxAlert{
				Severity: "error",
				Message:  "Test message",
			},
			testMode:         true,
			expectedStatus:   http.StatusOK,
			expectedResponse: types.ResponseOK,
		},
		{
			name:       "valid request normal mode",
			authHeader: "Bearer test_token",
			body: types.FluxAlert{
				Severity: "error",
				Message:  "Test message",
			},
			expectedStatus:   http.StatusOK,
			expectedResponse: types.ResponseOK,
		},
		{
			name:       "pushover error",
			authHeader: "Bearer test_token",
			body: types.FluxAlert{
				Severity: "error",
				Message:  "Test message",
			},
			pushoverError:  fmt.Errorf("connection timeout"),
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				PushoverAPIToken: "test_token",
				PushoverUserKey:  "test_user",
				BearerToken:      "Bearer test_token",
			}

			if tt.testMode {
				cfg.PushoverAPIToken = "test_api_token"
				cfg.BearerToken = "Bearer test_api_token"
			}

			mockPushover := &MockPushoverClient{
				SendMessageFunc: func(ctx context.Context, msg *types.PushoverMessage) error {
					return tt.pushoverError
				},
			}

			deps := &HandlerDependencies{
				Config:         cfg,
				PushoverClient: mockPushover,
				Logger:         &MockLogger{},
				MessageBuilder: BuildPushoverMessage,
			}

			handler := CreateWebhookHandler(deps)

			var bodyBytes []byte
			if tt.body != nil {
				if str, ok := tt.body.(string); ok {
					bodyBytes = []byte(str)
				} else {
					bodyBytes, _ = json.Marshal(tt.body)
				}
			}

			req, _ := http.NewRequest("POST", "/webhook", bytes.NewBuffer(bodyBytes))
			req.Header.Set("Authorization", tt.authHeader)
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			if tt.expectedResponse != nil && !bytes.Equal(rr.Body.Bytes(), tt.expectedResponse) {
				t.Errorf("Expected body %s, got %s", tt.expectedResponse, rr.Body.String())
			}
		})
	}
}

func TestCreateWebhookHandler_LargePayload(t *testing.T) {
	cfg := &config.Config{
		PushoverAPIToken: "test_token",
		PushoverUserKey:  "test_user",
		BearerToken:      "Bearer test_token",
	}

	deps := &HandlerDependencies{
		Config:         cfg,
		PushoverClient: &MockPushoverClient{},
		Logger:         &MockLogger{},
		MessageBuilder: BuildPushoverMessage,
	}

	handler := CreateWebhookHandler(deps)

	// Create payload larger than MaxBodySize
	largeMessage := strings.Repeat("x", 2<<20) // 2MB
	alert := types.FluxAlert{
		Message: largeMessage,
	}

	body, _ := json.Marshal(alert)
	req, _ := http.NewRequest("POST", "/webhook", bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer test_token")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d for large payload, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestWriteJSONResponse(t *testing.T) {
	tests := []struct {
		statusCode int
		body       []byte
	}{
		{http.StatusOK, []byte(`{"status":"ok"}`)},
		{http.StatusBadRequest, []byte(`{"error":"bad request"}`)},
		{http.StatusInternalServerError, []byte(`{"error":"internal error"}`)},
	}

	for _, tt := range tests {
		rr := httptest.NewRecorder()
		writeJSONResponse(rr, tt.statusCode, tt.body)

		if rr.Code != tt.statusCode {
			t.Errorf("Expected status %d, got %d", tt.statusCode, rr.Code)
		}

		if contentType := rr.Header().Get("Content-Type"); contentType != types.ContentTypeJSON {
			t.Errorf("Expected Content-Type %s, got %s", types.ContentTypeJSON, contentType)
		}

		if !bytes.Equal(rr.Body.Bytes(), tt.body) {
			t.Errorf("Expected body %s, got %s", tt.body, rr.Body.String())
		}
	}
}

func TestCreateRouter(t *testing.T) {
	cfg := &config.Config{
		PushoverAPIToken: "test_token",
		PushoverUserKey:  "test_user",
		BearerToken:      "Bearer test_token",
	}

	deps := &HandlerDependencies{
		Config:         cfg,
		PushoverClient: &MockPushoverClient{},
		Logger:         &MockLogger{},
		MessageBuilder: BuildPushoverMessage,
	}

	router := CreateRouter(deps)

	// Test each route
	tests := []struct {
		path           string
		method         string
		expectedStatus int
	}{
		{"/", "GET", http.StatusBadRequest},
		{"/health", "GET", http.StatusOK},
		{"/webhook", "POST", http.StatusUnauthorized}, // No auth header
	}

	for _, tt := range tests {
		req, _ := http.NewRequest(tt.method, tt.path, nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != tt.expectedStatus {
			t.Errorf("Path %s: expected status %d, got %d",
				tt.path, tt.expectedStatus, rr.Code)
		}
	}
}

// Benchmark tests
func BenchmarkCreateWebhookHandler(b *testing.B) {
	cfg := &config.Config{
		PushoverAPIToken: "test_api_token",
		PushoverUserKey:  "test_user",
		BearerToken:      "Bearer test_api_token",
	}

	deps := &HandlerDependencies{
		Config:         cfg,
		PushoverClient: &MockPushoverClient{},
		Logger:         &MockLogger{},
		MessageBuilder: BuildPushoverMessage,
	}

	handler := CreateWebhookHandler(deps)

	alert := types.FluxAlert{
		Severity: "error",
		Message:  "Benchmark test message",
	}

	body, _ := json.Marshal(alert)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("POST", "/webhook", bytes.NewBuffer(body))
		req.Header.Set("Authorization", "Bearer test_api_token")
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}
}
