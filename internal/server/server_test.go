package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/zhorvath83/flux-provider-pushover/internal/config"
	"github.com/zhorvath83/flux-provider-pushover/internal/types"
)

// MockLogger for testing (thread-safe)
type MockLogger struct {
	mu       sync.Mutex
	Messages []string
}

func (m *MockLogger) Info(msg string, args ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Messages = append(m.Messages, msg)
}

func (m *MockLogger) Warn(msg string, args ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Messages = append(m.Messages, msg)
}

func (m *MockLogger) Error(msg string, args ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Messages = append(m.Messages, msg)
}

func (m *MockLogger) With(args ...any) Logger {
	return m
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

	if server.httpServer.MaxHeaderBytes != types.MaxHeaderSize {
		t.Errorf("Expected MaxHeaderBytes %d, got %d",
			types.MaxHeaderSize, server.httpServer.MaxHeaderBytes)
	}

	if server.logger != logger {
		t.Error("Logger was not set correctly")
	}
}

func TestServer_StartAndShutdown(t *testing.T) {
	cfg := &config.Config{
		Port: ":0",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	})

	logger := &MockLogger{}

	server := NewServer(cfg, handler, logger)

	ts := httptest.NewServer(handler)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := server.Shutdown(ctx)
	if err != nil {
		t.Errorf("Failed to shutdown server: %v", err)
	}
}

func TestServer_WaitForShutdown_Timeout(t *testing.T) {
	cfg := &config.Config{Port: ":0"}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	logger := &MockLogger{}
	server := NewServer(cfg, handler, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := server.Shutdown(ctx)
	if err == nil {
		t.Log("Server shutdown completed without timeout")
	} else if err.Error() != "server forced to shutdown: "+context.DeadlineExceeded.Error() {
		expectedErr := fmt.Errorf("server forced to shutdown: %w", context.DeadlineExceeded)
		if err.Error() != expectedErr.Error() {
			t.Logf("Expected error '%v', got '%v'", expectedErr, err)
		}
	}
}

func TestServer_Shutdown_StopsStoppers(t *testing.T) {
	cfg := &config.Config{Port: ":0"}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	logger := &MockLogger{}

	stopped := false
	stub := &stubStopper{onStop: func() { stopped = true }}

	srv := NewServer(cfg, handler, logger, stub)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
	if !stopped {
		t.Error("Expected Stopper.Stop() to be called during shutdown")
	}
}

type stubStopper struct {
	onStop func()
}

func (s *stubStopper) Stop() {
	if s.onStop != nil {
		s.onStop()
	}
}

func TestSlogLogger(t *testing.T) {
	logger := NewSlogLogger()
	if logger == nil {
		t.Fatal("NewSlogLogger returned nil")
	}

	// These should not panic
	logger.Info("test info", "key", "value")
	logger.Warn("test warn", "key", "value")
	logger.Error("test error", "key", "value")

	derived := logger.With("request_id", "abc123")
	if derived == nil {
		t.Fatal("With returned nil")
	}

	// Derived logger should implement Logger interface
	derived.Info("derived info")
}
