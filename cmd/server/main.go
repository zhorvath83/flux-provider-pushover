package main

import (
	"os"

	"github.com/zhorvath83/flux-provider-pushover/internal/config"
	"github.com/zhorvath83/flux-provider-pushover/internal/handlers"
	"github.com/zhorvath83/flux-provider-pushover/internal/server"
)

// RunApp runs the application with dependency injection (testable)
func RunApp(configLoader config.ConfigLoader, logger server.Logger) error {
	cfg, err := config.WithValidation(configLoader, config.ValidateConfig)()
	if err != nil {
		return err
	}

	deps, err := handlers.CreateServerDependencies(cfg, logger)
	if err != nil {
		return err
	}

	router := handlers.CreateRouter(deps)
	rateLimiter := server.NewIPRateLimiter(server.DefaultRateLimiterConfig)
	handler := server.RequestIDMiddleware(
		server.RateLimitMiddleware(rateLimiter, logger)(router),
	)

	srv := server.NewServer(cfg, handler, logger, rateLimiter)
	if err := srv.Start(); err != nil {
		return err
	}

	return srv.WaitForShutdown()
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "-health" {
		if err := server.HealthCheck("http://localhost:8080/health"); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}

	logger := server.NewSlogLogger()
	if err := RunApp(config.DefaultConfigLoader, logger); err != nil {
		logger.Error("application failed", "error", err)
		os.Exit(1)
	}
}
