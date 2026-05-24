package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zhorvath83/flux-provider-pushover/internal/config"
	"github.com/zhorvath83/flux-provider-pushover/internal/types"
)

// Stopper is an interface for components that need cleanup on shutdown.
type Stopper interface {
	Stop()
}

// Server represents the HTTP server with dependencies
type Server struct {
	httpServer   *http.Server
	logger       Logger
	startErr     chan error
	serverCtx    context.Context
	serverCancel context.CancelFunc
	stoppers     []Stopper
}

// NewServer creates a new server instance
func NewServer(cfg *config.Config, handler http.Handler, logger Logger, stoppers ...Stopper) *Server {
	serverCtx, serverCancel := context.WithCancel(context.Background())

	return &Server{
		httpServer: &http.Server{
			Addr:           cfg.Port,
			Handler:        handler,
			ReadTimeout:    time.Duration(types.ReadTimeout) * time.Second,
			WriteTimeout:   time.Duration(types.WriteTimeout) * time.Second,
			MaxHeaderBytes: types.MaxHeaderSize,
			BaseContext: func(_ net.Listener) context.Context {
				return serverCtx
			},
		},
		logger:       logger,
		startErr:     make(chan error, 1),
		serverCtx:    serverCtx,
		serverCancel: serverCancel,
		stoppers:     stoppers,
	}
}

// Start starts the server. Returns an error if the server fails to bind.
//
// The 100ms timeout is a pragmatic compromise: it catches immediate bind
// failures (e.g. port already in use) while returning promptly for the
// common case where ListenAndServe succeeds. Longer delays on startup
// are not a concern for this service.
func (s *Server) Start() error {
	s.logger.Info("starting server", "addr", s.httpServer.Addr)

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.startErr <- err
			return
		}
		s.startErr <- nil
	}()

	select {
	case err := <-s.startErr:
		if err != nil {
			s.logger.Error("server failed to start", "error", err)
			return err
		}
	case <-time.After(100 * time.Millisecond):
	}

	return nil
}

// Shutdown performs graceful shutdown and cancels the server context
// so in-flight requests are interrupted.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("shutting down server")
	s.serverCancel()

	for _, stopper := range s.stoppers {
		stopper.Stop()
	}

	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("server forced to shutdown: %w", err)
	}

	s.logger.Info("server exited")
	return nil
}

// SignalChan returns the channel WaitForShutdown waits on.
// Override in tests to inject a controllable channel.
var SignalChan = defaultSignalChan

func defaultSignalChan() chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	return ch
}

// WaitForShutdown waits for interrupt signal and performs graceful shutdown
func (s *Server) WaitForShutdown() error {
	stop := SignalChan()
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(types.ShutdownTimeout)*time.Second)
	defer cancel()

	return s.Shutdown(ctx)
}

// HealthCheck performs a health check (for Docker HEALTHCHECK)
func HealthCheck(url string) error {
	resp, err := healthCheckClient.Get(url) //gosec:disable G107 -- URL is internally controlled and validated.
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	return nil
}

// healthCheckClient has a 2-second timeout to avoid hanging health checks.
var healthCheckClient = &http.Client{
	Timeout: 2 * time.Second,
}

// writeJSONResponse writes a JSON response with proper headers.
func writeJSONResponse(w http.ResponseWriter, statusCode int, body []byte) {
	w.Header().Set("Content-Type", types.ContentTypeJSON)
	w.WriteHeader(statusCode)
	w.Write(body)
}
