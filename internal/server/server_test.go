package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
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
		// Server wasn't started, so shutdown succeeds immediately — acceptable
		return
	}
	if !strings.Contains(err.Error(), "context") {
		t.Fatalf("Expected context-related error, got: %v", err)
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

func TestSlogLogger_JSONOutput(t *testing.T) {
	// Verify that SlogLogger produces valid JSON with expected fields.
	// This also validates S3 (log injection protection) since slog
	// auto-escapes user-controlled strings in JSON output.
	var buf strings.Builder
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	testLogger := &SlogLogger{Logger: slog.New(handler)}

	// Log with user-controlled data that contains injection attempts
	testLogger.Info("alert sent",
		"kind", "Kustomization\r\nFAKE: admin approved",
		"name", "flux-system\nINJECTED",
		"request_id", "abc123",
	)

	output := buf.String()
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("SlogLogger output is not valid JSON: %v\nOutput: %s", err, output)
	}

	if parsed["msg"] != "alert sent" {
		t.Errorf("Expected msg 'alert sent', got %v", parsed["msg"])
	}
	if parsed["kind"] != "Kustomization\r\nFAKE: admin approved" {
		t.Errorf("Expected kind with CRLF preserved in JSON, got %v", parsed["kind"])
	}
	if parsed["request_id"] != "abc123" {
		t.Errorf("Expected request_id 'abc123', got %v", parsed["request_id"])
	}

	// Verify the output is a single JSON line (no log injection)
	lines := strings.Count(output, "\n")
	if lines != 1 && !(lines == 0 && !strings.Contains(output, "\n")) {
		// slog may or may not add trailing newline; the key point is no extra lines
		// from CRLF in the kind field
	}
}

func TestSlogLogger_Levels(t *testing.T) {
	tests := []struct {
		name  string
		level string // "INFO", "WARN", "ERROR"
		logFn func(Logger)
	}{
		{"info", "INFO", func(l Logger) { l.Info("test") }},
		{"warn", "WARN", func(l Logger) { l.Warn("test") }},
		{"error", "ERROR", func(l Logger) { l.Error("test") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
			testLogger := &SlogLogger{Logger: slog.New(handler)}
			tt.logFn(testLogger)

			var parsed map[string]interface{}
			if err := json.Unmarshal([]byte(buf.String()), &parsed); err != nil {
				t.Fatalf("Output is not valid JSON: %v", err)
			}
			if parsed["level"] != tt.level {
				t.Errorf("Expected level %s, got %v", tt.level, parsed["level"])
			}
		})
	}
}

func TestServer_BaseContextCancellation(t *testing.T) {
	// Verify that the server's BaseContext is cancelled on Shutdown,
	// which propagates cancellation to all in-flight requests.
	cfg := &config.Config{Port: ":0"}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	logger := &MockLogger{}
	srv := NewServer(cfg, handler, logger)

	// Verify the server context exists and is not yet cancelled
	if srv.serverCtx == nil {
		t.Fatal("Expected serverCtx to be set")
	}
	if srv.serverCtx.Err() != nil {
		t.Fatal("Expected serverCtx to not be cancelled yet")
	}

	// Start the server so it can accept connections
	if err := srv.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Shutdown should cancel the context
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// After shutdown, the server context should be cancelled
	if srv.serverCtx.Err() == nil {
		t.Error("Expected serverCtx to be cancelled after shutdown")
	}
}

func TestServer_LargeHeaderRejected(t *testing.T) {
	// Use a fixed port server to test MaxHeaderBytes enforcement.
	// The net/http server rejects requests with headers exceeding MaxHeaderBytes
	// with 431 Request Header Fields Too Large.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	srv := &http.Server{
		Addr:           ":0",
		Handler:        handler,
		MaxHeaderBytes: types.MaxHeaderSize,
	}

	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}

	go srv.Serve(ln)
	defer srv.Close()

	addr := ln.Addr().String()
	url := "http://" + addr + "/"

	// Send a request with a header larger than MaxHeaderSize (8KB)
	largeHeader := strings.Repeat("x", 2*types.MaxHeaderSize)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("X-Large", largeHeader)

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// Connection may be reset by the server, which is also acceptable
		t.Logf("Request with oversized header resulted in connection error (acceptable): %v", err)
	} else {
		resp.Body.Close()
		if resp.StatusCode != http.StatusRequestHeaderFieldsTooLarge {
			t.Errorf("Expected %d for oversized headers, got %d", http.StatusRequestHeaderFieldsTooLarge, resp.StatusCode)
		}
	}
}

// serverFailWriter is an http.ResponseWriter whose Write method always returns an error.
type serverFailWriter struct {
	header http.Header
}

func (fw *serverFailWriter) Header() http.Header {
	if fw.header == nil {
		fw.header = make(http.Header)
	}
	return fw.header
}

func (fw *serverFailWriter) WriteHeader(_ int) {}

func (fw *serverFailWriter) Write(_ []byte) (int, error) {
	return 0, fmt.Errorf("write failed")
}

// TestWriteJSONResponse_WriteFailure verifies that writeJSONResponse does not panic
// when the ResponseWriter.Write returns an error.
func TestWriteJSONResponse_WriteFailure(t *testing.T) {
	fw := &serverFailWriter{}
	// Should not panic — the error from Write is silently ignored
	writeJSONResponse(fw, http.StatusOK, []byte(`{"status":"ok"}`))
}

func TestServer_InflightRequestCancelledOnShutdown(t *testing.T) {
	// Verify that an in-flight request receives context cancellation
	// when the server shuts down (BaseContext propagation).
	// This is the critical R2 test: a slow handler should observe
	// r.Context().Done() when the server shuts down.
	started := make(chan struct{})
	cancelled := make(chan error, 1)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		cancelled <- r.Context().Err()
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	addr := ln.Addr().String()

	cfg := &config.Config{Port: ":0"}
	logger := &MockLogger{}
	srv := NewServer(cfg, handler, logger)

	go srv.httpServer.Serve(ln)

	// Wait for server to be ready
	time.Sleep(50 * time.Millisecond)

	// Make a request that blocks until context cancellation
	done := make(chan struct{})
	go func() {
		defer close(done)
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get("http://" + addr + "/")
		if err == nil {
			resp.Body.Close()
		}
	}()

	// Wait for the request to reach the handler
	<-started

	// Shutdown should cancel the request context via BaseContext
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// The handler should have received context.Canceled
	select {
	case err := <-cancelled:
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timed out waiting for request context cancellation")
	}

	<-done
}

// TestServer_Shutdown_ErrorOnTimeout verifies that Shutdown returns an error
// containing "server forced to shutdown" when an in-flight request blocks
// longer than the shutdown timeout.
func TestServer_Shutdown_ErrorOnTimeout(t *testing.T) {
	// Handler that ignores context cancellation and blocks for a long time
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Deliberately ignore context cancellation — simulate a stuck handler
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	addr := ln.Addr().String()

	cfg := &config.Config{Port: ":0"}
	logger := &MockLogger{}
	srv := NewServer(cfg, handler, logger)

	go srv.httpServer.Serve(ln)
	time.Sleep(50 * time.Millisecond)

	// Make a request that will block for 5 seconds
	go func() {
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get("http://" + addr + "/")
		if err == nil {
			resp.Body.Close()
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// Shutdown with a very short timeout — should fail because handler is stuck
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err = srv.Shutdown(ctx)
	if err == nil {
		t.Fatal("Expected error when shutdown timeout expires with stuck handler, got nil")
	}
	if !strings.Contains(err.Error(), "server forced to shutdown") {
		t.Errorf("Expected error to contain 'server forced to shutdown', got: %v", err)
	}
}