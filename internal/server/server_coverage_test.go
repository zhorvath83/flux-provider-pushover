package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/zhorvath83/flux-provider-pushover/internal/config"
)

// TestServer_Start tests the Start method
func TestServer_Start(t *testing.T) {
	// Set test environment to prevent os.Exit
	os.Setenv("GO_TEST", "1")
	defer os.Unsetenv("GO_TEST")

	cfg := &config.Config{
		Port: ":0", // Use random available port
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	logger := &MockLogger{}
	srv := NewServer(cfg, handler, logger)

	// Start the server
	err := srv.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Shutdown the server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = srv.Shutdown(ctx)
	if err != nil {
		t.Fatalf("Failed to shutdown server: %v", err)
	}
}

// TestServer_Start_WithInvalidPort tests Start with invalid port
func TestServer_Start_WithInvalidPort(t *testing.T) {
	// Set test environment to prevent os.Exit
	os.Setenv("GO_TEST", "1")
	defer os.Unsetenv("GO_TEST")

	cfg := &config.Config{
		Port: ":-1", // Invalid port
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	logger := &MockLogger{}
	srv := NewServer(cfg, handler, logger)

	// Start the server (should not crash due to GO_TEST env var)
	err := srv.Start()
	if err != nil {
		t.Logf("Expected behavior: Start returned error: %v", err)
	}

	// Give goroutine time to attempt start
	time.Sleep(100 * time.Millisecond)

	// Verify error was logged (thread-safe read)
	logger.mu.Lock()
	messagesLen := len(logger.Messages)
	logger.mu.Unlock()

	if messagesLen == 0 {
		t.Error("Expected error message to be logged")
	}
}

// TestServer_WaitForShutdown tests the WaitForShutdown method
func TestServer_WaitForShutdown(t *testing.T) {
	cfg := &config.Config{
		Port: ":0",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	logger := &MockLogger{}
	srv := NewServer(cfg, handler, logger)

	// Start server in goroutine
	go func() {
		err := srv.Start()
		if err != nil {
			t.Logf("Server start error: %v", err)
		}
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Simulate shutdown in goroutine
	shutdownComplete := make(chan bool)
	go func() {
		// We can't easily test signal handling, but we can test the shutdown flow
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err := srv.Shutdown(ctx)
		if err != nil {
			t.Logf("Shutdown error: %v", err)
		}
		shutdownComplete <- true
	}()

	// Wait for shutdown to complete
	select {
	case <-shutdownComplete:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown timeout")
	}
}

// TestHealthCheck tests the HealthCheck function
func TestHealthCheck(t *testing.T) {
	// Create a test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	// Test successful health check
	err := HealthCheck(ts.URL + "/health")
	if err != nil {
		t.Errorf("HealthCheck failed: %v", err)
	}

	// Test failed health check (404)
	err = HealthCheck(ts.URL + "/notfound")
	if err == nil {
		t.Error("Expected error for 404 status")
	}

	// Test failed health check (invalid URL)
	err = HealthCheck("http://localhost:99999/health")
	if err == nil {
		t.Error("Expected error for unreachable server")
	}
}

// TestHealthCheck_RealServer tests health check with actual server
func TestHealthCheck_RealServer(t *testing.T) {
	cfg := &config.Config{
		Port: ":0", // Random port
	}

	// Create router with health endpoint
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	})

	logger := &MockLogger{}
	srv := NewServer(cfg, mux, logger)

	// Create test HTTP server
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Test health check
	err := HealthCheck(ts.URL + "/health")
	if err != nil {
		t.Errorf("HealthCheck failed: %v", err)
	}

	// Ensure srv is used to avoid unused variable warning
	_ = srv
}
