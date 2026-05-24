package handlers

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

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
				InvolvedObject: types.ObjectReference{
					Kind: "Deployment",
					Name: "test-deployment",
				},
				Metadata: map[string]string{
					"revision": "abc123",
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
		{
			name: "alert with app-version in metadata",
			alert: &types.FluxAlert{
				Severity:            "error",
				Message:             "Helm install failed",
				Reason:              "InstallFailed",
				ReportingController: "helm-controller",
				InvolvedObject: types.ObjectReference{
					Kind:      "HelmRelease",
					Name:      "tuppr",
					Namespace: "system-upgrade",
				},
				Metadata: map[string]string{
					"revision":    "main@sha1:abc123",
					"app-version": "0.1.35",
				},
			},
			expected: "InstallFailed [ERROR]\nHelm install failed\n\nController: helm-controller\nObject: helmrelease/tuppr\nRevision: main@sha1:abc123 (app: 0.1.35)\n",
		},
		{
			name: "alert with app-version empty string",
			alert: &types.FluxAlert{
				Severity: "info",
				Message:  "Deployed",
				Reason:   "TestReason",
				InvolvedObject: types.ObjectReference{
					Kind: "HelmRelease",
					Name: "my-release",
				},
				Metadata: map[string]string{
					"revision":    "abc123",
					"app-version": "",
				},
			},
			expected: "TestReason [INFO]\nDeployed\n\nController: Unknown\nObject: helmrelease/my-release\nRevision: abc123\n",
		},
		{
			name: "alert with multiple metadata keys",
			alert: &types.FluxAlert{
				Severity:            "error",
				Message:             "Test message",
				Reason:              "TestReason",
				ReportingController: "test-controller",
				InvolvedObject: types.ObjectReference{
					Kind: "Deployment",
					Name: "test-deployment",
				},
				Metadata: map[string]string{
					"revision":      "abc123",
					"commit_status": "success",
					"summary":       "test summary",
					"app-version":   "1.0.0",
				},
			},
			expected: "TestReason [ERROR]\nTest message\n\nController: test-controller\nObject: deployment/test-deployment\nRevision: abc123 (app: 1.0.0)\n",
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
		InvolvedObject: types.ObjectReference{
			Kind:      "Deployment",
			Name:      "test-deployment",
			Namespace: "test-namespace",
		},
		Metadata: map[string]string{
			"revision": "abc123",
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

func TestExtractAlertInfo_MetadataMap(t *testing.T) {
	alert := &types.FluxAlert{
		Severity:            "error",
		Message:             "Test message",
		Reason:              "TestReason",
		ReportingController: "helm-controller",
		InvolvedObject: types.ObjectReference{
			Kind:      "HelmRelease",
			Name:      "tuppr",
			Namespace: "system-upgrade",
		},
		Metadata: map[string]string{
			"revision":      "main@sha1:abc123",
			"app-version":   "0.1.35",
			"commit_status": "success",
			"summary":       "test summary",
		},
	}

	info := ExtractAlertInfo(alert)

	tests := []struct {
		key      string
		expected string
	}{
		{"revision", "main@sha1:abc123"},
		{"severity", "error"},
		{"controller", "helm-controller"},
		{"kind", "HelmRelease"},
		{"name", "tuppr"},
		{"namespace", "system-upgrade"},
	}

	for _, tt := range tests {
		if info[tt.key] != tt.expected {
			t.Errorf("ExtractAlertInfo()[%s] = %s, want %s",
				tt.key, info[tt.key], tt.expected)
		}
	}
}

func TestBuildPushoverMessage_NilMetadata(t *testing.T) {
	alert := &types.FluxAlert{
		Severity:            "info",
		Message:             "Reconciliation succeeded",
		Reason:              "ReconciliationSucceeded",
		ReportingController: "kustomize-controller",
		InvolvedObject: types.ObjectReference{
			Kind: "Kustomization",
			Name: "flux-system",
		},
		Metadata: nil,
	}

	result := BuildPushoverMessage(alert)
	expected := "ReconciliationSucceeded [INFO]\nReconciliation succeeded\n\nController: kustomize-controller\nObject: kustomization/flux-system\nRevision: Unknown\n"

	if result != expected {
		t.Errorf("BuildPushoverMessage with nil Metadata:\nExpected:\n%s\nGot:\n%s", expected, result)
	}
}

func TestExtractAlertInfo_NilMetadata(t *testing.T) {
	alert := &types.FluxAlert{
		Severity:            "info",
		Reason:              "ReconciliationSucceeded",
		ReportingController: "kustomize-controller",
		InvolvedObject: types.ObjectReference{
			Kind: "Kustomization",
			Name: "flux-system",
		},
		Metadata: nil,
	}

	info := ExtractAlertInfo(alert)

	if info["revision"] != types.DefaultValue {
		t.Errorf("Expected revision=%s for nil Metadata, got %s", types.DefaultValue, info["revision"])
	}
}

// Benchmark tests
func BenchmarkBuildPushoverMessage(b *testing.B) {
	alert := &types.FluxAlert{
		Severity:            "error",
		Message:             "Benchmark test message",
		Reason:              "BenchmarkReason",
		ReportingController: "benchmark-controller",
		InvolvedObject: types.ObjectReference{
			Kind: "Deployment",
			Name: "benchmark-deployment",
		},
		Metadata: map[string]string{
			"revision": "abc123def456",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = BuildPushoverMessage(alert)
	}
}

func TestCreatePushoverMessage_Truncation(t *testing.T) {
	cfg := &config.Config{
		PushoverAPIToken: "test_token",
		PushoverUserKey:  "test_user",
	}

	longMsg := strings.Repeat("x", 2000)
	result := CreatePushoverMessage(cfg, longMsg)

	if utf8.RuneCountInString(result.Message) > PushoverMessageLimit {
		t.Errorf("Message not truncated: got %d runes, max %d", utf8.RuneCountInString(result.Message), PushoverMessageLimit)
	}
	if !strings.HasSuffix(result.Message, "...") {
		t.Error("Truncated message should end with ...")
	}
}

func TestValidateAlert_FieldLengths(t *testing.T) {
	tests := []struct {
		name      string
		alert     *types.FluxAlert
		wantError bool
	}{
		{
			name: "message too long",
			alert: &types.FluxAlert{
				Message: strings.Repeat("x", MaxMessageFieldLen+1),
			},
			wantError: true,
		},
		{
			name: "name too long",
			alert: &types.FluxAlert{
				InvolvedObject: types.ObjectReference{
					Name: strings.Repeat("x", MaxStringFieldLen+1),
				},
			},
			wantError: true,
		},
		{
			name: "too many metadata entries",
			alert: &types.FluxAlert{
				Metadata: func() map[string]string {
					m := make(map[string]string)
					for i := 0; i < MaxMetadataEntries+1; i++ {
						m[fmt.Sprintf("key%d", i)] = "val"
					}
					return m
				}(),
			},
			wantError: true,
		},
		{
			name: "metadata key too long",
			alert: &types.FluxAlert{
				Metadata: map[string]string{
					strings.Repeat("k", MaxMetadataKeyLen+1): "value",
				},
			},
			wantError: true,
		},
		{
			name: "valid within limits",
			alert: &types.FluxAlert{
				Message: strings.Repeat("x", MaxMessageFieldLen),
				InvolvedObject: types.ObjectReference{
					Name: strings.Repeat("n", MaxStringFieldLen),
				},
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

func TestValidateAlert_RuneBasedLength(t *testing.T) {
	// Multi-byte UTF-8: 2-byte runes (Hungarian accents)
	longName := strings.Repeat("á", MaxStringFieldLen+1) // 513 runes, 1026 bytes
	alert := &types.FluxAlert{
		InvolvedObject: types.ObjectReference{
			Name: longName,
		},
	}
	err := ValidateAlert(alert)
	if err == nil {
		t.Error("Expected error for name exceeding rune limit")
	}

	// Exactly 512 runes should pass even though bytes > 512
	validName := strings.Repeat("á", MaxStringFieldLen) // 512 runes, 1024 bytes
	alert = &types.FluxAlert{
		InvolvedObject: types.ObjectReference{
			Name: validName,
		},
	}
	err = ValidateAlert(alert)
	if err != nil {
		t.Errorf("Expected valid for 512 runes, got: %v", err)
	}
}
