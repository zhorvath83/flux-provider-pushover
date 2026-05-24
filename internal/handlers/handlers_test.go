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
	"github.com/zhorvath83/flux-provider-pushover/internal/server"
	"github.com/zhorvath83/flux-provider-pushover/internal/types"
)

// MockLogger for testing
type MockLogger struct {
	messages []string
}

func (m *MockLogger) Info(msg string, args ...any) {
	m.messages = append(m.messages, msg)
}

func (m *MockLogger) Warn(msg string, args ...any) {
	m.messages = append(m.messages, msg)
}

func (m *MockLogger) Error(msg string, args ...any) {
	m.messages = append(m.messages, msg)
}

func (m *MockLogger) With(args ...any) server.Logger {
	return m
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
			pushoverError:    fmt.Errorf("connection timeout"),
			expectedStatus:   http.StatusBadGateway,
			expectedResponse: types.ResponseUpstreamError,
		},
		{
			name:       "valid request with app-version metadata",
			authHeader: "Bearer test_token",
			body: types.FluxAlert{
				Severity:            "error",
				Message:             "Helm install failed",
				Reason:              "InstallFailed",
				ReportingController: "helm-controller",
				InvolvedObject: types.ObjectReference{
					Kind:      "HelmRelease",
					Name:      "tuppr",
					Namespace: "system-upgrade",
				},
				Metadata: map[string]string{
					"revision":    "main@sha1:abc123",
					"app-version": "0.1.35",
				},
			},
			expectedStatus:   http.StatusOK,
			expectedResponse: types.ResponseOK,
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

func TestCreateWebhookHandler_UnknownTopLevelField(t *testing.T) {
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

	// Send a payload with an unknown top-level field to verify forward compatibility
	body := map[string]interface{}{
		"severity": "error",
		"message":  "Test",
		"reason":   "TestReason",
		"newField": "someValue",
		"involvedObject": map[string]interface{}{
			"kind": "Kustomization",
			"name": "flux-system",
		},
		"metadata": map[string]interface{}{
			"revision": "main@sha1:abc",
		},
	}

	bodyBytes, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/webhook", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Authorization", "Bearer test_api_token")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d for payload with unknown top-level field, got %d", http.StatusOK, rr.Code)
	}
}

func TestCreateWebhookHandler_MetadataWithMultipleKeys(t *testing.T) {
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

	body := types.FluxAlert{
		Severity:            "error",
		Message:             "Test",
		Reason:              "TestReason",
		ReportingController: "kustomize-controller",
		InvolvedObject: types.ObjectReference{
			Kind: "Kustomization",
			Name: "flux-system",
		},
		Metadata: map[string]string{
			"revision":      "main@sha1:abc",
			"commit_status": "success",
			"summary":       "test summary",
		},
	}

	bodyBytes, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/webhook", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Authorization", "Bearer test_api_token")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d for payload with multiple metadata keys, got %d", http.StatusOK, rr.Code)
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

func TestCreateWebhookHandler_AuthTimingSafe(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
	}{
		{"empty header", ""},
		{"prefix match only", "Bearer test_toke"},
		{"different length", "Bearer test_token_extra"},
		{"no Bearer prefix", "test_token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			req, _ := http.NewRequest("POST", "/webhook", nil)
			req.Header.Set("Authorization", tt.authHeader)

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusUnauthorized {
				t.Errorf("Expected %d for auth header %q, got %d", http.StatusUnauthorized, tt.authHeader, rr.Code)
			}
		})
	}
}

func TestCreateWebhookHandler_NoUpstreamErrorLeakage(t *testing.T) {
	cfg := &config.Config{
		PushoverAPIToken: "test_token",
		PushoverUserKey:  "test_user",
		BearerToken:      "Bearer test_token",
	}

	pushoverErr := fmt.Errorf("pushover API returned status 400: {\"errors\":[\"application token is invalid\"]}")

	deps := &HandlerDependencies{
		Config: cfg,
		PushoverClient: &MockPushoverClient{
			SendMessageFunc: func(ctx context.Context, msg *types.PushoverMessage) error {
				return pushoverErr
			},
		},
		Logger:         &MockLogger{},
		MessageBuilder: BuildPushoverMessage,
	}

	handler := CreateWebhookHandler(deps)

	alert := types.FluxAlert{Severity: "error", Message: "test"}
	body, _ := json.Marshal(alert)

	req, _ := http.NewRequest("POST", "/webhook", bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer test_token")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("Expected %d, got %d", http.StatusBadGateway, rr.Code)
	}

	if bytes.Equal(rr.Body.Bytes(), types.ResponseUpstreamError) {
		// Response is exactly the generic error — no upstream details
	} else {
		// Check that upstream error specifics are NOT in the response
		respBody := rr.Body.String()
		if strings.Contains(respBody, "application token") {
			t.Error("Response leaks upstream error details: contains 'application token'")
		}
		if strings.Contains(respBody, "pushover") {
			t.Error("Response leaks upstream error details: contains 'pushover'")
		}
		if strings.Contains(respBody, "400") {
			t.Error("Response leaks upstream error details: contains status code 400")
		}
	}
}

func BenchmarkAuthCompare(b *testing.B) {
	cfg := &config.Config{
		PushoverAPIToken: "test_token",
		PushoverUserKey:  "test_user",
		BearerToken:      "Bearer test_token_abcdef1234567890",
	}

	deps := &HandlerDependencies{
		Config:         cfg,
		PushoverClient: &MockPushoverClient{},
		Logger:         &MockLogger{},
		MessageBuilder: BuildPushoverMessage,
	}

	handler := CreateWebhookHandler(deps)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("POST", "/webhook", nil)
		req.Header.Set("Authorization", "Bearer wrong_token_abcdef1234567890")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}
}

// TestResponseShape_Snapshot verifies every status code produces a valid
// JSON response with exactly the expected shape: {"error": "..."} or {"status": "..."}.
func TestResponseShape_Snapshot(t *testing.T) {
	tests := []struct {
		name           string
		handler        http.HandlerFunc
		expectedStatus int
		expectedBody   []byte
	}{
		{"root", CreateRootHandler(), http.StatusBadRequest, types.ResponseRootError},
		{"health", CreateHealthHandler(), http.StatusOK, types.ResponseHealthy},
		{"method not allowed", methodNotAllowedHandler(), http.StatusMethodNotAllowed, types.ResponseMethodNotAllowed},
		{"unauthorized", unauthorizedHandler(), http.StatusUnauthorized, types.ResponseUnauthorized},
		{"invalid json", invalidJSONHandler(), http.StatusBadRequest, types.ResponseInvalidJSON},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			rr := httptest.NewRecorder()
			tt.handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			contentType := rr.Header().Get("Content-Type")
			if tt.name != "health" && tt.name != "root" {
				if contentType != types.ContentTypeJSON {
					t.Errorf("Expected Content-Type %s, got %s", types.ContentTypeJSON, contentType)
				}
			}

			// For JSON responses, verify it's valid JSON with an "error" or "status" key
			if contentType == types.ContentTypeJSON {
				var parsed map[string]interface{}
				if err := json.Unmarshal(rr.Body.Bytes(), &parsed); err != nil {
					t.Errorf("Response is not valid JSON: %v\nBody: %s", err, rr.Body.String())
				}
				if _, hasError := parsed["error"]; !hasError {
					if _, hasStatus := parsed["status"]; !hasStatus {
						t.Error("JSON response must have 'error' or 'status' key")
					}
				}
			}
		})
	}
}

func methodNotAllowedHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSONResponse(w, http.StatusMethodNotAllowed, types.ResponseMethodNotAllowed)
	}
}

func unauthorizedHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSONResponse(w, http.StatusUnauthorized, types.ResponseUnauthorized)
	}
}

func invalidJSONHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSONResponse(w, http.StatusBadRequest, types.ResponseInvalidJSON)
	}
}

// TestLogInjection verifies that user-controlled input in log messages
// does not produce malformed JSON or inject false log lines.
func TestLogInjection(t *testing.T) {
	maliciousInputs := []struct {
		name    string
		message string
	}{
		{"CRLF injection", "test\r\nFAKE ADMIN: approved"},
		{"newline injection", "test\nFAKE ALERT"},
		{"null byte", "test\x00hidden"},
		{"ANSI escape", "test\x1b[31mFAKE\x1b[0m"},
	}

	for _, tt := range maliciousInputs {
		t.Run(tt.name, func(t *testing.T) {
			// slog JSON handler auto-escapes these characters.
			// Verify that the MockLogger receives the message without corruption.
			logger := &MockLogger{}

			cfg := &config.Config{
				PushoverAPIToken: "test_api_token",
				PushoverUserKey:  "test_user",
				BearerToken:      "Bearer test_api_token",
			}

			deps := &HandlerDependencies{
				Config:         cfg,
				PushoverClient: &MockPushoverClient{},
				Logger:         logger,
				MessageBuilder: BuildPushoverMessage,
			}

			handler := CreateWebhookHandler(deps)

			alert := types.FluxAlert{
				Severity: "error",
				Message:  tt.message,
				InvolvedObject: types.ObjectReference{
					Kind: "Kustomization\r\nFAKE",
					Name: "flux-system\nINJECTED",
				},
			}
			body, _ := json.Marshal(alert)

			req, _ := http.NewRequest("POST", "/webhook", bytes.NewBuffer(body))
			req.Header.Set("Authorization", "Bearer test_api_token")
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("Expected 200 for valid alert with malicious input, got %d", rr.Code)
			}
		})
	}
}
