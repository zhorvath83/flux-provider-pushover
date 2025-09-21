package handlers

import (
	"fmt"
	"strings"

	"github.com/zhorvath83/flux-provider-pushover/internal/config"
	"github.com/zhorvath83/flux-provider-pushover/internal/types"
)

// MessageBuilder is a functional type for building messages
type MessageBuilder func(*types.FluxAlert) string

// BuildPushoverMessage creates a formatted message from FluxAlert (pure function)
func BuildPushoverMessage(alert *types.FluxAlert) string {
	severity := normalizeString(alert.Severity, types.DefaultSeverity, strings.ToUpper)
	reason := defaultIfEmpty(alert.Reason, types.DefaultValue)
	controller := defaultIfEmpty(alert.ReportingController, types.DefaultValue)
	revision := defaultIfEmpty(alert.Metadata.Revision, types.DefaultValue)
	kind := normalizeString(alert.InvolvedObject.Kind, types.DefaultValue, strings.ToLower)
	objectName := defaultIfEmpty(alert.InvolvedObject.Name, types.DefaultValue)
	message := defaultIfEmpty(alert.Message, types.NoMessage)

	return fmt.Sprintf("%s [%s]\n%s\n\nController: %s\nObject: %s/%s\nRevision: %s\n",
		reason, severity, message, controller, kind, objectName, revision)
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

// CreatePushoverMessage creates a PushoverMessage struct (pure function)
func CreatePushoverMessage(cfg *config.Config, message string) *types.PushoverMessage {
	return &types.PushoverMessage{
		Token:   cfg.PushoverAPIToken,
		User:    cfg.PushoverUserKey,
		Title:   types.AppTitle,
		Message: message,
	}
}

// ValidateAlert validates a FluxAlert (pure function)
func ValidateAlert(alert *types.FluxAlert) error {
	if alert == nil {
		return fmt.Errorf("alert is nil")
	}
	// Add more validation if needed
	return nil
}

// ExtractAlertInfo extracts key information from alert (pure function)
func ExtractAlertInfo(alert *types.FluxAlert) map[string]string {
	return map[string]string{
		"severity":   defaultIfEmpty(alert.Severity, types.DefaultSeverity),
		"reason":     defaultIfEmpty(alert.Reason, types.DefaultValue),
		"controller": defaultIfEmpty(alert.ReportingController, types.DefaultValue),
		"revision":   defaultIfEmpty(alert.Metadata.Revision, types.DefaultValue),
		"kind":       defaultIfEmpty(alert.InvolvedObject.Kind, types.DefaultValue),
		"name":       defaultIfEmpty(alert.InvolvedObject.Name, types.DefaultValue),
		"namespace":  defaultIfEmpty(alert.InvolvedObject.Namespace, "default"),
		"message":    defaultIfEmpty(alert.Message, types.NoMessage),
	}
}
