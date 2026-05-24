package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/zhorvath83/flux-provider-pushover/internal/config"
)

func TestServer_Start(t *testing.T) {
	cfg := &config.Config{
		Port: ":0",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	logger := &MockLogger{}
	srv := NewServer(cfg, handler, logger)

	err := srv.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = srv.Shutdown(ctx)
	if err != nil {
		t.Fatalf("Failed to shutdown server: %v", err)
	}
}

func TestServer_Start_WithInvalidPort(t *testing.T) {
	cfg := &config.Config{
		Port: ":-1",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	logger := &MockLogger{}
	srv := NewServer(cfg, handler, logger)

	err := srv.Start()
	if err == nil {
		t.Error("Expected error for invalid port")
	}
}

func TestServer_WaitForShutdown(t *testing.T) {
	cfg := &config.Config{
		Port: ":0",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	logger := &MockLogger{}
	srv := NewServer(cfg, handler, logger)

	go func() {
		err := srv.Start()
		if err != nil {
			t.Logf("Server start error: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	shutdownComplete := make(chan bool)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err := srv.Shutdown(ctx)
		if err != nil {
			t.Logf("Shutdown error: %v", err)
		}
		shutdownComplete <- true
	}()

	select {
	case <-shutdownComplete:
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown timeout")
	}
}

func TestHealthCheck(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	err := HealthCheck(ts.URL + "/health")
	if err != nil {
		t.Errorf("HealthCheck failed: %v", err)
	}

	err = HealthCheck(ts.URL + "/notfound")
	if err == nil {
		t.Error("Expected error for 404 status")
	}

	err = HealthCheck("http://localhost:99999/health")
	if err == nil {
		t.Error("Expected error for unreachable server")
	}
}

func TestHealthCheck_RealServer(t *testing.T) {
	cfg := &config.Config{
		Port: ":0",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	})

	logger := &MockLogger{}
	srv := NewServer(cfg, mux, logger)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	err := HealthCheck(ts.URL + "/health")
	if err != nil {
		t.Errorf("HealthCheck failed: %v", err)
	}

	_ = srv
}

func TestHealthCheck_Timeout(t *testing.T) {
	// Verify that the 2-second health check client timeout works:
	// a server that takes longer than 2s should cause a timeout error.
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer slowServer.Close()

	start := time.Now()
	err := HealthCheck(slowServer.URL + "/health")
	elapsed := time.Since(start)

	if err == nil {
		t.Error("Expected error for slow server, got nil")
	}

	// The error should occur within approximately 2 seconds (the client timeout)
	if elapsed > 3*time.Second {
		t.Errorf("Health check took too long: %v, expected ~2s timeout", elapsed)
	}
}

func TestServer_WaitForShutdown_SignalTriggersShutdown(t *testing.T) {
	sigCh := make(chan os.Signal, 1)
	orig := SignalChan
	defer func() { SignalChan = orig }()
	SignalChan = func() chan os.Signal { return sigCh }

	cfg := &config.Config{Port: ":0"}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	logger := &MockLogger{}
	srv := NewServer(cfg, handler, logger)

	if err := srv.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- srv.WaitForShutdown() }()

	time.Sleep(50 * time.Millisecond)

	sigCh <- syscall.SIGTERM

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Expected nil on graceful shutdown, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("WaitForShutdown did not complete within timeout")
	}
}

func TestDefaultSignalChan(t *testing.T) {
	ch := defaultSignalChan()
	if ch == nil {
		t.Fatal("defaultSignalChan returned nil channel")
	}
	if cap(ch) < 1 {
		t.Errorf("Expected buffered channel with cap >= 1, got cap %d", cap(ch))
	}
	// Clean up signal registration to avoid leaking the handler
	signal.Stop(ch)
}

func TestServer_WaitForShutdown_DelegatesToShutdown(t *testing.T) {
	sigCh := make(chan os.Signal, 1)
	orig := SignalChan
	defer func() { SignalChan = orig }()
	SignalChan = func() chan os.Signal { return sigCh }

	cfg := &config.Config{Port: ":0"}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	logger := &MockLogger{}
	srv := NewServer(cfg, handler, logger)

	if err := srv.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- srv.WaitForShutdown() }()

	// Send signal immediately to trigger graceful shutdown
	sigCh <- syscall.SIGTERM

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Expected nil on graceful shutdown, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("WaitForShutdown did not complete within timeout")
	}
}
