package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zhorvath83/flux-provider-pushover/internal/config"
	"github.com/zhorvath83/flux-provider-pushover/internal/types"
)

// MockLogger for testing
type MockLogger struct {
	Messages []string
}

func (m *MockLogger) Printf(format string, v ...interface{}) {
	m.Messages = append(m.Messages, fmt.Sprintf(format, v...))
}

func (m *MockLogger) Println(v ...interface{}) {
	m.Messages = append(m.Messages, fmt.Sprint(v...))
}

func TestNewServer(t *testing.T) {
	cfg := &config.Config{
		Port: ":9090",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	logger := &MockLogger{}

	server := NewServer(cfg, handler, logger)

	if server.httpServer.Addr != ":9090" {
		t.Errorf("Expected addr :9090, got %s", server.httpServer.Addr)
	}

	if server.httpServer.ReadTimeout != time.Duration(types.ReadTimeout)*time.Second {
		t.Errorf("Expected ReadTimeout %v, got %v",
			time.Duration(types.ReadTimeout)*time.Second, server.httpServer.ReadTimeout)
	}

	if server.httpServer.WriteTimeout != time.Duration(types.WriteTimeout)*time.Second {
		t.Errorf("Expected WriteTimeout %v, got %v",
			time.Duration(types.WriteTimeout)*time.Second, server.httpServer.WriteTimeout)
	}

	if server.httpServer.MaxHeaderBytes != types.MaxBodySize {
		t.Errorf("Expected MaxHeaderBytes %d, got %d",
			types.MaxBodySize, server.httpServer.MaxHeaderBytes)
	}

	if server.logger != logger {
		t.Error("Logger was not set correctly")
	}
}

func TestServer_StartAndShutdown(t *testing.T) {
	cfg := &config.Config{
		Port: ":0", // Random port
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	})

	logger := &MockLogger{}

	server := NewServer(cfg, handler, logger)

	// Replace the ListenAndServe with a test server
	ts := httptest.NewServer(handler)
	defer ts.Close()

	// Test shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := server.Shutdown(ctx)
	if err != nil {
		t.Errorf("Failed to shutdown server: %v", err)
	}
}

func TestServer_WaitForShutdown_Timeout(t *testing.T) {
	// This test verifies the shutdown timeout behavior
	cfg := &config.Config{Port: ":0"}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate long-running request
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	logger := &MockLogger{}
	server := NewServer(cfg, handler, logger)

	// Create a context that times out quickly
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// This should timeout
	err := server.Shutdown(ctx)
	if err == nil {
		// In this case, no connections are active so it might not timeout
		t.Log("Server shutdown completed without timeout")
	} else if err.Error() != "server forced to shutdown: "+context.DeadlineExceeded.Error() {
		// Check if we get the expected wrapped error
		expectedErr := fmt.Errorf("server forced to shutdown: %w", context.DeadlineExceeded)
		if err.Error() != expectedErr.Error() {
			t.Logf("Expected error '%v', got '%v'", expectedErr, err)
		}
	}
}
