package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zhorvath83/flux-provider-pushover/internal/config"
	"github.com/zhorvath83/flux-provider-pushover/internal/types"
)

// Logger interface for logging (to avoid circular dependency)
type Logger interface {
	Printf(format string, v ...interface{})
	Println(v ...interface{})
}

// Server represents the HTTP server with dependencies
type Server struct {
	httpServer *http.Server
	logger     Logger
}

// NewServer creates a new server instance
func NewServer(cfg *config.Config, handler http.Handler, logger Logger) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:           cfg.Port,
			Handler:        handler,
			ReadTimeout:    time.Duration(types.ReadTimeout) * time.Second,
			WriteTimeout:   time.Duration(types.WriteTimeout) * time.Second,
			MaxHeaderBytes: types.MaxBodySize,
		},
		logger: logger,
	}
}

// Start starts the server (non-blocking)
func (s *Server) Start() error {
	s.logger.Printf("Starting server on %s", s.httpServer.Addr)

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Printf("Server failed to start: %v", err)
			// Don't exit in tests
			if os.Getenv("GO_TEST") != "1" {
				os.Exit(1)
			}
		}
	}()

	return nil
}

// Shutdown performs graceful shutdown
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Println("Shutting down server...")

	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("server forced to shutdown: %w", err)
	}

	s.logger.Println("Server exited")
	return nil
}

// WaitForShutdown waits for interrupt signal and performs graceful shutdown
func (s *Server) WaitForShutdown() error {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(types.ShutdownTimeout)*time.Second)
	defer cancel()

	return s.Shutdown(ctx)
}

// HealthCheck performs a health check (for Docker HEALTHCHECK)
func HealthCheck(url string) error {
	// This is only used for Docker HEALTHCHECK with a known, local URL.
	resp, err := http.Get(url) //gosec:disable G107 -- URL is internally controlled and validated.
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	return nil
}
