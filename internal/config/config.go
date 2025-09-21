package config

import (
	"fmt"
	"os"
)

// Config holds application configuration
type Config struct {
	PushoverUserKey  string
	PushoverAPIToken string
	BearerToken      string // Pre-computed Bearer token
	Port             string
	PushoverURL      string // Make it configurable for testing
}

// ConfigValidator is a functional type for config validation
type ConfigValidator func(*Config) error

// ConfigLoader is a functional type for loading config
type ConfigLoader func() (*Config, error)

// NewConfig creates a new configuration with default values
func NewConfig() *Config {
	return &Config{
		Port:        ":8080",
		PushoverURL: "https://api.pushover.net/1/messages.json",
	}
}

// LoadFromEnv loads configuration from environment variables (pure function)
func LoadFromEnv(getEnv func(string) string) ConfigLoader {
	return func() (*Config, error) {
		cfg := NewConfig()

		cfg.PushoverUserKey = getEnv("PUSHOVER_USER_KEY")
		cfg.PushoverAPIToken = getEnv("PUSHOVER_API_TOKEN")

		if port := getEnv("PORT"); port != "" {
			cfg.Port = ":" + port
		}

		if pushoverURL := getEnv("PUSHOVER_URL"); pushoverURL != "" {
			cfg.PushoverURL = pushoverURL
		}

		// Pre-compute Bearer token
		if cfg.PushoverAPIToken != "" {
			cfg.BearerToken = "Bearer " + cfg.PushoverAPIToken
		}

		return cfg, nil
	}
}

// DefaultConfigLoader loads config from os.Getenv
var DefaultConfigLoader = LoadFromEnv(os.Getenv)

// ValidateConfig validates the configuration (pure function)
func ValidateConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}

	if cfg.PushoverUserKey == "" {
		return fmt.Errorf("PUSHOVER_USER_KEY is required")
	}

	if cfg.PushoverAPIToken == "" {
		return fmt.Errorf("PUSHOVER_API_TOKEN is required")
	}

	return nil
}

// WithValidation wraps a ConfigLoader with validation
func WithValidation(loader ConfigLoader, validators ...ConfigValidator) ConfigLoader {
	return func() (*Config, error) {
		cfg, err := loader()
		if err != nil {
			return nil, err
		}

		for _, validator := range validators {
			if err := validator(cfg); err != nil {
				return nil, err
			}
		}

		return cfg, nil
	}
}
