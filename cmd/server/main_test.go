package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zhorvath83/flux-provider-pushover/internal/config"
	"github.com/zhorvath83/flux-provider-pushover/internal/server"
)

// MockLoggerForRun implements server.Logger for testing
type MockLoggerForRun struct {
	Messages []string
}

func (m *MockLoggerForRun) Info(msg string, args ...any) {
	m.Messages = append(m.Messages, msg)
}

func (m *MockLoggerForRun) Warn(msg string, args ...any) {
	m.Messages = append(m.Messages, msg)
}

func (m *MockLoggerForRun) Error(msg string, args ...any) {
	m.Messages = append(m.Messages, msg)
}

func (m *MockLoggerForRun) With(args ...any) server.Logger {
	return m
}

func TestRunApp(t *testing.T) {
	tests := []struct {
		name          string
		configLoader  config.ConfigLoader
		expectError   bool
		errorContains string
	}{
		{
			name: "config validation error",
			configLoader: func() (*config.Config, error) {
				return &config.Config{}, nil
			},
			expectError:   true,
			errorContains: "PUSHOVER_USER_KEY",
		},
		{
			name: "config loader error",
			configLoader: func() (*config.Config, error) {
				return nil, fmt.Errorf("failed to load config")
			},
			expectError:   true,
			errorContains: "failed to load config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := &MockLoggerForRun{}
			err := RunApp(tt.configLoader, logger)

			if tt.expectError {
				if err == nil {
					t.Fatal("Expected error but got nil")
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Fatalf("Expected error to contain '%s' but got '%s'", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestRunApp_StartsAndStops(t *testing.T) {
	configLoader := func() (*config.Config, error) {
		return &config.Config{
			PushoverUserKey:  "test_user",
			PushoverAPIToken: "test_token",
			PushoverURL:      "https://api.pushover.net/1/messages.json",
			Port:             ":0",
			BearerToken:      "Bearer test_token",
		}, nil
	}

	logger := &MockLoggerForRun{}

	appDone := make(chan error, 1)
	go func() {
		appDone <- RunApp(configLoader, logger)
	}()

	// Give the server time to start
	time.Sleep(150 * time.Millisecond)

	// The app should be running — select should time out, not receive from appDone
	select {
	case err := <-appDone:
		t.Fatalf("App exited unexpectedly during startup: %v", err)
	default:
		// App is still running, which is the expected behavior
	}
}

func TestRunApp_InvalidPort(t *testing.T) {
	configLoader := func() (*config.Config, error) {
		return &config.Config{
			PushoverUserKey:  "test_user",
			PushoverAPIToken: "test_token",
			PushoverURL:      "https://api.pushover.net/1/messages.json",
			Port:             ":-1",
			BearerToken:      "Bearer test_token",
		}, nil
	}

	logger := &MockLoggerForRun{}
	err := RunApp(configLoader, logger)
	if err == nil {
		t.Fatal("Expected error for invalid port, got nil")
	}
}

func TestMain_HealthCheckMode(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		}
	}))
	defer ts.Close()

	err := server.HealthCheck(ts.URL + "/health")
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}
}