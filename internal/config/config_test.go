package config

import (
	"fmt"
	"testing"
)

func TestNewConfig(t *testing.T) {
	config := NewConfig()

	if config.Port != ":8080" {
		t.Errorf("Expected port :8080, got %s", config.Port)
	}

	if config.PushoverURL != "https://api.pushover.net/1/messages.json" {
		t.Errorf("Expected default Pushover URL, got %s", config.PushoverURL)
	}
}

func TestLoadFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		expected *Config
	}{
		{
			name: "default values",
			env:  map[string]string{},
			expected: &Config{
				Port:        ":8080",
				PushoverURL: "https://api.pushover.net/1/messages.json",
			},
		},
		{
			name: "with API token",
			env: map[string]string{
				"PUSHOVER_USER_KEY":  "user123",
				"PUSHOVER_API_TOKEN": "token456",
			},
			expected: &Config{
				PushoverUserKey:  "user123",
				PushoverAPIToken: "token456",
				BearerToken:      "Bearer token456",
				Port:             ":8080",
				PushoverURL:      "https://api.pushover.net/1/messages.json",
			},
		},
		{
			name: "custom port",
			env: map[string]string{
				"PORT": "9090",
			},
			expected: &Config{
				Port:        ":9090",
				PushoverURL: "https://api.pushover.net/1/messages.json",
			},
		},
		{
			name: "custom pushover URL",
			env: map[string]string{
				"PUSHOVER_URL": "http://mock.pushover.com",
			},
			expected: &Config{
				Port:        ":8080",
				PushoverURL: "http://mock.pushover.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockGetEnv := func(key string) string {
				return tt.env[key]
			}

			loader := LoadFromEnv(mockGetEnv)
			config, err := loader()

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if config.PushoverUserKey != tt.expected.PushoverUserKey {
				t.Errorf("PushoverUserKey: expected %s, got %s",
					tt.expected.PushoverUserKey, config.PushoverUserKey)
			}

			if config.PushoverAPIToken != tt.expected.PushoverAPIToken {
				t.Errorf("PushoverAPIToken: expected %s, got %s",
					tt.expected.PushoverAPIToken, config.PushoverAPIToken)
			}

			if config.BearerToken != tt.expected.BearerToken {
				t.Errorf("BearerToken: expected %s, got %s",
					tt.expected.BearerToken, config.BearerToken)
			}

			if config.Port != tt.expected.Port {
				t.Errorf("Port: expected %s, got %s",
					tt.expected.Port, config.Port)
			}

			if config.PushoverURL != tt.expected.PushoverURL {
				t.Errorf("PushoverURL: expected %s, got %s",
					tt.expected.PushoverURL, config.PushoverURL)
			}
		})
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		wantError bool
		errorMsg  string
	}{
		{
			name:      "nil config",
			config:    nil,
			wantError: true,
			errorMsg:  "config is nil",
		},
		{
			name:      "missing user key",
			config:    &Config{PushoverAPIToken: "token"},
			wantError: true,
			errorMsg:  "PUSHOVER_USER_KEY is required",
		},
		{
			name:      "missing API token",
			config:    &Config{PushoverUserKey: "user"},
			wantError: true,
			errorMsg:  "PUSHOVER_API_TOKEN is required",
		},
		{
			name: "valid config",
			config: &Config{
				PushoverUserKey:  "user",
				PushoverAPIToken: "token",
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.config)

			if tt.wantError {
				if err == nil {
					t.Error("Expected error but got nil")
				} else if err.Error() != tt.errorMsg {
					t.Errorf("Expected error '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestWithValidation(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		wantError bool
	}{
		{
			name: "valid config passes validation",
			config: &Config{
				PushoverUserKey:  "user",
				PushoverAPIToken: "token",
			},
			wantError: false,
		},
		{
			name: "invalid config fails validation",
			config: &Config{
				PushoverUserKey: "user",
				// Missing API token
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLoader := func() (*Config, error) {
				return tt.config, nil
			}

			validatedLoader := WithValidation(mockLoader, ValidateConfig)
			config, err := validatedLoader()

			if tt.wantError {
				if err == nil {
					t.Error("Expected validation error")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if config != tt.config {
					t.Error("Config was modified during validation")
				}
			}
		})
	}
}

func TestWithValidation_LoaderError(t *testing.T) {
	expectedErr := fmt.Errorf("loader error")

	mockLoader := func() (*Config, error) {
		return nil, expectedErr
	}

	validatedLoader := WithValidation(mockLoader, ValidateConfig)
	_, err := validatedLoader()

	if err != expectedErr {
		t.Errorf("Expected loader error to be returned, got %v", err)
	}
}

func TestWithValidation_MultipleValidators(t *testing.T) {
	config := &Config{
		PushoverUserKey:  "user",
		PushoverAPIToken: "token",
	}

	validator1Called := false
	validator2Called := false

	validator1 := func(c *Config) error {
		validator1Called = true
		return nil
	}

	validator2 := func(c *Config) error {
		validator2Called = true
		return nil
	}

	mockLoader := func() (*Config, error) {
		return config, nil
	}

	validatedLoader := WithValidation(mockLoader, validator1, validator2)
	_, err := validatedLoader()

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !validator1Called {
		t.Error("Validator 1 was not called")
	}

	if !validator2Called {
		t.Error("Validator 2 was not called")
	}
}

func TestDefaultConfigLoader(t *testing.T) {
	// This test ensures DefaultConfigLoader is properly defined
	// It will use actual environment variables
	_, err := DefaultConfigLoader()
	if err != nil {
		// This is expected if env vars are not set
		t.Logf("DefaultConfigLoader returned error as expected: %v", err)
	}
}
