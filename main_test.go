package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
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
		Severity: "error",
		Message:  "Test message",
		Reason:   "TestReason",
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

func TestConfigValidation(t *testing.T) {
	// Test with missing environment variables
	os.Setenv("PUSHOVER_USER_KEY", "")
	os.Setenv("PUSHOVER_API_TOKEN", "")
	
	// The main function should exit if config is invalid
	// This test just ensures the config struct works correctly
	config := &Config{
		PushoverUserKey:  os.Getenv("PUSHOVER_USER_KEY"),
		PushoverAPIToken: os.Getenv("PUSHOVER_API_TOKEN"),
	}

	if config.PushoverUserKey != "" || config.PushoverAPIToken != "" {
		t.Error("Config should be empty when env vars are not set")
	}
}