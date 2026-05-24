package handlers

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/zhorvath83/flux-provider-pushover/internal/config"
	"github.com/zhorvath83/flux-provider-pushover/internal/types"
)

const (
	MaxStringFieldLen    = 512
	MaxMessageFieldLen   = 4096
	MaxMetadataKeyLen    = 128
	MaxMetadataValueLen  = 1024
	MaxMetadataEntries   = 32
	PushoverMessageLimit = 1024
)

// MessageBuilder is a functional type for building messages
type MessageBuilder func(*types.FluxAlert) string

// BuildPushoverMessage creates a formatted message from FluxAlert (pure function)
func BuildPushoverMessage(alert *types.FluxAlert) string {
	severity := normalizeString(alert.Severity, types.DefaultSeverity, strings.ToUpper)
	reason := defaultIfEmpty(alert.Reason, types.DefaultValue)
	controller := defaultIfEmpty(alert.ReportingController, types.DefaultValue)
	revision := defaultIfEmpty(alert.Metadata["revision"], types.DefaultValue)
	kind := normalizeString(alert.InvolvedObject.Kind, types.DefaultValue, strings.ToLower)
	objectName := defaultIfEmpty(alert.InvolvedObject.Name, types.DefaultValue)
	message := defaultIfEmpty(alert.Message, types.NoMessage)

	revisionLine := revision
	if appVersion, ok := alert.Metadata["app-version"]; ok && appVersion != "" {
		revisionLine = fmt.Sprintf("%s (app: %s)", revision, appVersion)
	}

	return fmt.Sprintf("%s [%s]\n%s\n\nController: %s\nObject: %s/%s\nRevision: %s\n",
		reason, severity, message, controller, kind, objectName, revisionLine)
}

// defaultIfEmpty returns default value if string is empty (pure function)
func defaultIfEmpty(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

// normalizeString applies transformation to string with default (pure function)
func normalizeString(value, defaultValue string, transform func(string) string) string {
	if value == "" {
		return transform(defaultValue)
	}
	return transform(value)
}

// CreatePushoverMessage creates a PushoverMessage struct, truncating message to Pushover limit
func CreatePushoverMessage(cfg *config.Config, message string) *types.PushoverMessage {
	if utf8.RuneCountInString(message) > PushoverMessageLimit {
		runes := []rune(message)
		message = string(runes[:PushoverMessageLimit-3]) + "..."
	}
	return &types.PushoverMessage{
		Token:   cfg.PushoverAPIToken,
		User:    cfg.PushoverUserKey,
		Title:   types.AppTitle,
		Message: message,
	}
}

// ValidateAlert validates a FluxAlert with per-field length limits
func ValidateAlert(alert *types.FluxAlert) error {
	if alert == nil {
		return fmt.Errorf("alert is nil")
	}

	if err := validateFieldLen("message", alert.Message, MaxMessageFieldLen); err != nil {
		return err
	}
	if err := validateFieldLen("reason", alert.Reason, MaxStringFieldLen); err != nil {
		return err
	}
	if err := validateFieldLen("severity", alert.Severity, MaxStringFieldLen); err != nil {
		return err
	}
	if err := validateFieldLen("reportingController", alert.ReportingController, MaxStringFieldLen); err != nil {
		return err
	}
	if err := validateFieldLen("kind", alert.InvolvedObject.Kind, MaxStringFieldLen); err != nil {
		return err
	}
	if err := validateFieldLen("name", alert.InvolvedObject.Name, MaxStringFieldLen); err != nil {
		return err
	}
	if err := validateFieldLen("namespace", alert.InvolvedObject.Namespace, MaxStringFieldLen); err != nil {
		return err
	}

	if len(alert.Metadata) > MaxMetadataEntries {
		return fmt.Errorf("metadata has too many entries: %d (max %d)", len(alert.Metadata), MaxMetadataEntries)
	}
	for k, v := range alert.Metadata {
		if err := validateFieldLen("metadata key", k, MaxMetadataKeyLen); err != nil {
			return err
		}
		if err := validateFieldLen("metadata value", v, MaxMetadataValueLen); err != nil {
			return err
		}
	}

	return nil
}

func validateFieldLen(name, value string, max int) error {
	if utf8.RuneCountInString(value) > max {
		return fmt.Errorf("%s exceeds maximum length: %d (max %d)", name, utf8.RuneCountInString(value), max)
	}
	return nil
}

// ExtractAlertInfo extracts key information from alert (pure function)
func ExtractAlertInfo(alert *types.FluxAlert) map[string]string {
	return map[string]string{
		"severity":   defaultIfEmpty(alert.Severity, types.DefaultSeverity),
		"reason":     defaultIfEmpty(alert.Reason, types.DefaultValue),
		"controller": defaultIfEmpty(alert.ReportingController, types.DefaultValue),
		"revision":   defaultIfEmpty(alert.Metadata["revision"], types.DefaultValue),
		"kind":       defaultIfEmpty(alert.InvolvedObject.Kind, types.DefaultValue),
		"name":       defaultIfEmpty(alert.InvolvedObject.Name, types.DefaultValue),
		"namespace":  defaultIfEmpty(alert.InvolvedObject.Namespace, "default"),
		"message":    defaultIfEmpty(alert.Message, types.NoMessage),
	}
}
