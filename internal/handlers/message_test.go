package handlers

import (
	"strings"
	"testing"

	"github.com/zhorvath83/flux-provider-pushover/internal/config"
	"github.com/zhorvath83/flux-provider-pushover/internal/types"
)

func TestBuildPushoverMessage(t *testing.T) {
	tests := []struct {
		name     string
		alert    *types.FluxAlert
		expected string
	}{
		{
			name: "complete alert",
			alert: &types.FluxAlert{
				Severity:            "error",
				Message:             "Test message",
				Reason:              "TestReason",
				ReportingController: "test-controller",
				InvolvedObject: struct {
					Kind            string `json:"kind"`
					Namespace       string `json:"namespace"`
					Name            string `json:"name"`
					UID             string `json:"uid"`
					APIVersion      string `json:"apiVersion"`
					ResourceVersion string `json:"resourceVersion"`
				}{
					Kind: "Deployment",
					Name: "test-deployment",
				},
				Metadata: struct {
					CommitStatus string `json:"commit_status"`
					Revision     string `json:"revision"`
					Summary      string `json:"summary"`
				}{
					Revision: "abc123",
				},
			},
			expected: "TestReason [ERROR]\nTest message\n\nController: test-controller\nObject: deployment/test-deployment\nRevision: abc123\n",
		},
		{
			name:     "empty alert",
			alert:    &types.FluxAlert{},
			expected: "Unknown [INFO]\nNo Message\n\nController: Unknown\nObject: unknown/Unknown\nRevision: Unknown\n",
		},
		{
			name: "partial alert",
			alert: &types.FluxAlert{
				Severity: "warning",
				Message:  "Partial message",
			},
			expected: "Unknown [WARNING]\nPartial message\n\nController: Unknown\nObject: unknown/Unknown\nRevision: Unknown\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildPushoverMessage(tt.alert)
			if result != tt.expected {
				t.Errorf("BuildPushoverMessage():\nExpected:\n%s\nGot:\n%s", tt.expected, result)
			}
		})
	}
}

func TestDefaultIfEmpty(t *testing.T) {
	tests := []struct {
		value        string
		defaultValue string
		expected     string
	}{
		{"", "default", "default"},
		{"value", "default", "value"},
		{" ", "default", " "}, // Space is not empty
	}

	for _, tt := range tests {
		result := defaultIfEmpty(tt.value, tt.defaultValue)
		if result != tt.expected {
			t.Errorf("defaultIfEmpty(%q, %q) = %q, want %q",
				tt.value, tt.defaultValue, result, tt.expected)
		}
	}
}

func TestNormalizeString(t *testing.T) {
	tests := []struct {
		name         string
		value        string
		defaultValue string
		transform    func(string) string
		expected     string
	}{
		{
			name:         "empty value returns default",
			value:        "",
			defaultValue: "DEFAULT",
			transform:    strings.ToUpper,
			expected:     "DEFAULT",
		},
		{
			name:         "non-empty value gets transformed",
			value:        "hello",
			defaultValue: "DEFAULT",
			transform:    strings.ToUpper,
			expected:     "HELLO",
		},
		{
			name:         "transform to lower",
			value:        "HELLO",
			defaultValue: "default",
			transform:    strings.ToLower,
			expected:     "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeString(tt.value, tt.defaultValue, tt.transform)
			if result != tt.expected {
				t.Errorf("normalizeString() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestCreatePushoverMessage(t *testing.T) {
	cfg := &config.Config{
		PushoverAPIToken: "test_token",
		PushoverUserKey:  "test_user",
	}
	message := "Test message content"

	result := CreatePushoverMessage(cfg, message)

	if result.Token != "test_token" {
		t.Errorf("Expected token 'test_token', got '%s'", result.Token)
	}

	if result.User != "test_user" {
		t.Errorf("Expected user 'test_user', got '%s'", result.User)
	}

	if result.Title != types.AppTitle {
		t.Errorf("Expected title '%s', got '%s'", types.AppTitle, result.Title)
	}

	if result.Message != message {
		t.Errorf("Expected message '%s', got '%s'", message, result.Message)
	}
}

func TestValidateAlert(t *testing.T) {
	tests := []struct {
		name      string
		alert     *types.FluxAlert
		wantError bool
	}{
		{
			name:      "nil alert",
			alert:     nil,
			wantError: true,
		},
		{
			name:      "valid alert",
			alert:     &types.FluxAlert{},
			wantError: false,
		},
		{
			name: "alert with data",
			alert: &types.FluxAlert{
				Severity: "error",
				Message:  "Test",
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAlert(tt.alert)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateAlert() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestExtractAlertInfo(t *testing.T) {
	alert := &types.FluxAlert{
		Severity:            "error",
		Message:             "Test message",
		Reason:              "TestReason",
		ReportingController: "test-controller",
		InvolvedObject: struct {
			Kind            string `json:"kind"`
			Namespace       string `json:"namespace"`
			Name            string `json:"name"`
			UID             string `json:"uid"`
			APIVersion      string `json:"apiVersion"`
			ResourceVersion string `json:"resourceVersion"`
		}{
			Kind:      "Deployment",
			Name:      "test-deployment",
			Namespace: "test-namespace",
		},
		Metadata: struct {
			CommitStatus string `json:"commit_status"`
			Revision     string `json:"revision"`
			Summary      string `json:"summary"`
		}{
			Revision: "abc123",
		},
	}

	info := ExtractAlertInfo(alert)

	tests := []struct {
		key      string
		expected string
	}{
		{"severity", "error"},
		{"reason", "TestReason"},
		{"controller", "test-controller"},
		{"revision", "abc123"},
		{"kind", "Deployment"},
		{"name", "test-deployment"},
		{"namespace", "test-namespace"},
		{"message", "Test message"},
	}

	for _, tt := range tests {
		if info[tt.key] != tt.expected {
			t.Errorf("ExtractAlertInfo()[%s] = %s, want %s",
				tt.key, info[tt.key], tt.expected)
		}
	}
}

func TestExtractAlertInfo_EmptyAlert(t *testing.T) {
	alert := &types.FluxAlert{}
	info := ExtractAlertInfo(alert)

	tests := []struct {
		key      string
		expected string
	}{
		{"severity", types.DefaultSeverity},
		{"reason", types.DefaultValue},
		{"controller", types.DefaultValue},
		{"revision", types.DefaultValue},
		{"kind", types.DefaultValue},
		{"name", types.DefaultValue},
		{"namespace", "default"},
		{"message", types.NoMessage},
	}

	for _, tt := range tests {
		if info[tt.key] != tt.expected {
			t.Errorf("ExtractAlertInfo()[%s] = %s, want %s",
				tt.key, info[tt.key], tt.expected)
		}
	}
}

// Benchmark tests
func BenchmarkBuildPushoverMessage(b *testing.B) {
	alert := &types.FluxAlert{
		Severity:            "error",
		Message:             "Benchmark test message",
		Reason:              "BenchmarkReason",
		ReportingController: "benchmark-controller",
		InvolvedObject: struct {
			Kind            string `json:"kind"`
			Namespace       string `json:"namespace"`
			Name            string `json:"name"`
			UID             string `json:"uid"`
			APIVersion      string `json:"apiVersion"`
			ResourceVersion string `json:"resourceVersion"`
		}{
			Kind: "Deployment",
			Name: "benchmark-deployment",
		},
		Metadata: struct {
			CommitStatus string `json:"commit_status"`
			Revision     string `json:"revision"`
			Summary      string `json:"summary"`
		}{
			Revision: "abc123def456",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = BuildPushoverMessage(alert)
	}
}
