package main

import (
	"log"
	"os"

	"github.com/zhorvath83/flux-provider-pushover/internal/config"
	"github.com/zhorvath83/flux-provider-pushover/internal/handlers"
	"github.com/zhorvath83/flux-provider-pushover/internal/server"
)

// DefaultLogger is the default logger implementation
type DefaultLogger struct{}

func (d DefaultLogger) Printf(format string, v ...interface{}) {
	log.Printf(format, v...)
}

func (d DefaultLogger) Println(v ...interface{}) {
	log.Println(v...)
}

// RunApp runs the application with dependency injection (testable)
func RunApp(configLoader config.ConfigLoader, logger server.Logger) error {
	// Load and validate configuration
	cfg, err := config.WithValidation(configLoader, config.ValidateConfig)()
	if err != nil {
		return err
	}

	// Create dependencies
	deps, err := handlers.CreateServerDependencies(cfg, logger)
	if err != nil {
		return err
	}

	// Create router
	router := handlers.CreateRouter(deps)

	// Create and start server
	srv := server.NewServer(cfg, router, logger)
	if err := srv.Start(); err != nil {
		return err
	}

	// Wait for shutdown signal
	return srv.WaitForShutdown()
}

func main() {
	// Handle health check mode for Docker HEALTHCHECK
	if len(os.Args) > 1 && os.Args[1] == "-health" {
		if err := server.HealthCheck("http://localhost:8080/health"); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Run the application
	logger := DefaultLogger{}
	if err := RunApp(config.DefaultConfigLoader, logger); err != nil {
		log.Fatalf("Application failed: %v", err)
	}
}
