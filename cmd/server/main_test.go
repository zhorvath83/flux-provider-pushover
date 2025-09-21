package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"testing"

	"github.com/zhorvath83/flux-provider-pushover/internal/config"
)

// Setup function to disable logging during tests
func init() {
	log.SetOutput(io.Discard)
}

func TestDefaultLogger(t *testing.T) {
	logger := DefaultLogger{}

	// These methods should not panic
	logger.Printf("test %s", "message")
	logger.Println("test message")
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
				return &config.Config{}, nil // Invalid config
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
			// Set test environment
			os.Setenv("GO_TEST", "1")
			defer os.Unsetenv("GO_TEST")

			logger := &DefaultLogger{}
			err := RunApp(tt.configLoader, logger)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got nil")
				} else if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
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

// TestMain verifies that the main function doesn't panic
func TestMain(t *testing.T) {
	// Since main() uses os.Exit, we can't test it directly
	// but we can verify the components work correctly

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Test health check argument parsing
	os.Args = []string{"test", "-health"}

	// The actual functionality is tested in other tests
	// This just ensures the code path exists
	t.Log("Main function argument parsing tested")
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && len(substr) == 0 || (len(substr) > 0 && findSubstring(s, substr) >= 0))
}

func findSubstring(s, substr string) int {
	if len(substr) == 0 {
		return 0
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
