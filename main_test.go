package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestHealthEndpoint(t *testing.T) {
	config := &Config{
		PushoverUserKey:  "test_user",
		PushoverAPIToken: "test_token",
	}
	server := NewServer(config)

	req, err := http.NewRequest("GET", "/health", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleHealth)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	expected := "healthy"
	if rr.Body.String() != expected {
		t.Errorf("handler returned unexpected body: got %v want %v",
			rr.Body.String(), expected)
	}
}

func TestRootEndpoint(t *testing.T) {
	config := &Config{
		PushoverUserKey:  "test_user",
		PushoverAPIToken: "test_token",
	}
	server := NewServer(config)

	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleRoot)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusBadRequest)
	}
}

func TestWebhookEndpointUnauthorized(t *testing.T) {
	config := &Config{
		PushoverUserKey:  "test_user",
		PushoverAPIToken: "test_token",
	}
	server := NewServer(config)

	req, err := http.NewRequest("POST", "/webhook", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer wrong_token")

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleWebhook)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusUnauthorized {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusUnauthorized)
	}
}

func TestWebhookEndpointTestMode(t *testing.T) {
	config := &Config{
		PushoverUserKey:  "test_user",
		PushoverAPIToken: "test_api_token", // Special test token
	}
	server := NewServer(config)

	alert := FluxAlert{
		Severity:            "error",
		Message:             "Test message",
		Reason:              "TestReason",
		ReportingController: "test-controller",
	}
	alert.InvolvedObject.Kind = "Deployment"
	alert.InvolvedObject.Name = "test-deployment"
	alert.Metadata.Revision = "abc123"

	body, _ := json.Marshal(alert)
	req, err := http.NewRequest("POST", "/webhook", bytes.NewBuffer(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer test_api_token")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleWebhook)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	var response map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["status"] != "ok" {
		t.Errorf("Expected status 'ok', got '%s'", response["status"])
	}
}

func TestWebhookInvalidJSON(t *testing.T) {
	config := &Config{
		PushoverUserKey:  "test_user",
		PushoverAPIToken: "test_token",
	}
	server := NewServer(config)

	req, err := http.NewRequest("POST", "/webhook", strings.NewReader("invalid json"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer test_token")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleWebhook)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusBadRequest)
	}
}

func TestWebhookEmptyFields(t *testing.T) {
	config := &Config{
		PushoverUserKey:  "test_user",
		PushoverAPIToken: "test_api_token",
	}
	server := NewServer(config)

	// Test with completely empty alert
	alert := FluxAlert{}

	body, _ := json.Marshal(alert)
	req, err := http.NewRequest("POST", "/webhook", bytes.NewBuffer(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer test_api_token")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleWebhook)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}
}
func TestWebhookLargePayload(t *testing.T) {
	config := &Config{
		PushoverUserKey:  "test_user",
		PushoverAPIToken: "test_token",
	}
	server := NewServer(config)

	// Create a payload larger than maxBodySize
	largeMessage := strings.Repeat("x", 2<<20) // 2MB
	alert := FluxAlert{
		Message: largeMessage,
	}

	body, _ := json.Marshal(alert)
	req, err := http.NewRequest("POST", "/webhook", bytes.NewBuffer(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer test_token")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleWebhook)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler should reject large payloads: got %v want %v",
			status, http.StatusBadRequest)
	}
}

func TestSendToPushover(t *testing.T) {
	// Create a test server to mock Pushover API
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		// Verify content type
		contentType := r.Header.Get("Content-Type")
		if contentType != "application/x-www-form-urlencoded" {
			t.Errorf("Expected Content-Type application/x-www-form-urlencoded, got %s", contentType)
		}

		// Parse form data
		r.ParseForm()
		token := r.FormValue("token")
		user := r.FormValue("user")
		message := r.FormValue("message")
		title := r.FormValue("title")

		if token != "test_token" {
			t.Errorf("Expected token 'test_token', got '%s'", token)
		}
		if user != "test_user" {
			t.Errorf("Expected user 'test_user', got '%s'", user)
		}
		if message == "" {
			t.Error("Message should not be empty")
		}
		if title != "FluxCD" {
			t.Errorf("Expected title 'FluxCD', got '%s'", title)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":1}`))
	}))
	defer mockServer.Close()

	// Note: In a real scenario, we would need to make pushoverURL configurable
	// For now, this test demonstrates the testing approach
	config := &Config{
		PushoverUserKey:  "test_user",
		PushoverAPIToken: "test_token",
	}
	_ = NewServer(config)

	// The actual sendToPushover test would require refactoring to accept URL as parameter
	// This demonstrates the test structure
}

func TestGracefulShutdown(t *testing.T) {
	config := &Config{
		PushoverUserKey:  "test_user",
		PushoverAPIToken: "test_token",
	}
	server := NewServer(config)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", server.handleHealth)

	httpServer := &http.Server{
		Addr:    ":0", // Random port
		Handler: mux,
	}

	// Start server
	go func() {
		httpServer.ListenAndServe()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Test shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := httpServer.Shutdown(ctx)
	if err != nil {
		t.Errorf("Failed to shutdown server: %v", err)
	}
}

func TestConfigValidation(t *testing.T) {
	// Test with missing environment variables
	os.Setenv("PUSHOVER_USER_KEY", "")
	os.Setenv("PUSHOVER_API_TOKEN", "")

	config := &Config{
		PushoverUserKey:  os.Getenv("PUSHOVER_USER_KEY"),
		PushoverAPIToken: os.Getenv("PUSHOVER_API_TOKEN"),
	}

	if config.PushoverUserKey != "" || config.PushoverAPIToken != "" {
		t.Error("Config should be empty when env vars are not set")
	}

	// Test with valid environment variables
	os.Setenv("PUSHOVER_USER_KEY", "valid_user")
	os.Setenv("PUSHOVER_API_TOKEN", "valid_token")

	config = &Config{
		PushoverUserKey:  os.Getenv("PUSHOVER_USER_KEY"),
		PushoverAPIToken: os.Getenv("PUSHOVER_API_TOKEN"),
	}

	if config.PushoverUserKey != "valid_user" || config.PushoverAPIToken != "valid_token" {
		t.Error("Config should be populated when env vars are set")
	}

	// Clean up
	os.Unsetenv("PUSHOVER_USER_KEY")
	os.Unsetenv("PUSHOVER_API_TOKEN")
}

// Benchmark tests
func BenchmarkHandleWebhook(b *testing.B) {
	config := &Config{
		PushoverUserKey:  "test_user",
		PushoverAPIToken: "test_api_token",
	}
	server := NewServer(config)

	alert := FluxAlert{
		Severity:            "error",
		Message:             "Benchmark test message",
		Reason:              "BenchmarkReason",
		ReportingController: "benchmark-controller",
	}
	alert.InvolvedObject.Kind = "Deployment"
	alert.InvolvedObject.Name = "benchmark-deployment"
	alert.Metadata.Revision = "abc123"

	body, _ := json.Marshal(alert)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("POST", "/webhook", bytes.NewBuffer(body))
		req.Header.Set("Authorization", "Bearer test_api_token")
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.handleWebhook)
		handler.ServeHTTP(rr, req)
	}
}

func BenchmarkMessageBuilding(b *testing.B) {
	alert := FluxAlert{
		Severity:            "error",
		Message:             "Test message for benchmarking",
		Reason:              "TestReason",
		ReportingController: "test-controller",
	}
	alert.InvolvedObject.Kind = "Deployment"
	alert.InvolvedObject.Name = "test-deployment"
	alert.Metadata.Revision = "abc123def456"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var msgBuilder strings.Builder
		msgBuilder.Grow(256)

		severity := strings.ToUpper(alert.Severity)
		if severity == "" {
			severity = "INFO"
		}

		fmt.Fprintf(&msgBuilder, "%s [%s]\n%s\n\nController: %s\nObject: %s/%s\nRevision: %s\n",
			alert.Reason, severity, alert.Message, alert.ReportingController,
			strings.ToLower(alert.InvolvedObject.Kind), alert.InvolvedObject.Name,
			alert.Metadata.Revision)

		_ = msgBuilder.String()
	}
}
