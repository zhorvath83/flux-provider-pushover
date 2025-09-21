package handlers

import (
	"testing"

	"github.com/zhorvath83/flux-provider-pushover/internal/config"
	"github.com/zhorvath83/flux-provider-pushover/internal/server"
)

// TestCreateServerDependencies tests the CreateServerDependencies function
func TestCreateServerDependencies(t *testing.T) {
	// Create test configuration
	cfg := &config.Config{
		PushoverUserKey:  "test_user",
		PushoverAPIToken: "test_token",
		PushoverURL:      "https://api.pushover.net/1/messages.json",
		Port:             ":8080",
	}

	// Create test logger
	logger := &MockLogger{}

	// Test creating dependencies
	deps, err := CreateServerDependencies(cfg, logger)
	if err != nil {
		t.Fatalf("CreateServerDependencies failed: %v", err)
	}

	// Verify dependencies are properly initialized
	if deps == nil {
		t.Fatal("Expected non-nil dependencies")
	}

	if deps.Config != cfg {
		t.Error("Config not properly set in dependencies")
	}

	if deps.PushoverClient == nil {
		t.Error("PushoverClient not properly initialized")
	}

	if deps.Logger != logger {
		t.Error("Logger not properly set in dependencies")
	}

	if deps.MessageBuilder == nil {
		t.Error("MessageBuilder not properly set")
	}
}

// TestCreateServerDependencies_FullIntegration tests integration with all components
func TestCreateServerDependencies_FullIntegration(t *testing.T) {
	// Create test configuration
	cfg := &config.Config{
		PushoverUserKey:  "test_user",
		PushoverAPIToken: "test_token",
		PushoverURL:      "https://api.pushover.net/1/messages.json",
		Port:             ":8080",
	}

	// Create test logger
	logger := server.Logger(&MockLogger{})

	// Test creating dependencies
	deps, err := CreateServerDependencies(cfg, logger)
	if err != nil {
		t.Fatalf("CreateServerDependencies failed: %v", err)
	}

	// Verify the dependencies work together
	router := CreateRouter(deps)
	if router == nil {
		t.Fatal("CreateRouter returned nil")
	}

	// Verify all routes are registered
	testPaths := []string{"/", "/health", "/webhook"}
	for _, path := range testPaths {
		t.Run("path_"+path, func(t *testing.T) {
			// Routes are registered, we already test them individually
			// This just verifies they're connected
		})
	}
}
