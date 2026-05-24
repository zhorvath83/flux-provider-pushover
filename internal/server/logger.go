package server

import (
	"log/slog"
	"os"
)

// Logger provides structured logging with severity levels and key-value pairs.
type Logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	With(args ...any) Logger
}

// SlogLogger wraps slog.Logger to implement the Logger interface.
type SlogLogger struct {
	*slog.Logger
}

// NewSlogLogger creates a new SlogLogger with JSON output to stdout.
func NewSlogLogger() *SlogLogger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return &SlogLogger{Logger: slog.New(handler)}
}

// With returns a new Logger with additional key-value pairs.
func (s *SlogLogger) With(args ...any) Logger {
	return &SlogLogger{Logger: s.Logger.With(args...)}
}
