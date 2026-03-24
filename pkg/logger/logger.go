// Package logger provides structured logging utilities built on Go's log/slog package.
// It offers a simple interface for creating and configuring loggers with consistent
// formatting and level control across the application.
package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Level represents the severity of a log message.
type Level = slog.Level

const (
	LevelDebug = slog.LevelDebug
	LevelInfo  = slog.LevelInfo
	LevelWarn  = slog.LevelWarn
	LevelError = slog.LevelError
)

// Logger wraps slog.Logger to provide a convenient application-level logging interface.
type Logger struct {
	*slog.Logger
}

// Config holds logger configuration.
type Config struct {
	// Level sets the minimum log level. Defaults to "info".
	Level string
	// Output specifies where logs are written. Defaults to os.Stdout.
	Output io.Writer
	// JSON enables JSON-formatted output. Defaults to false (human-readable text).
	JSON bool
}

// New creates a new Logger with the given configuration.
// If cfg is nil, a default logger writing to stdout at info level is returned.
func New(cfg *Config) *Logger {
	if cfg == nil {
		cfg = &Config{}
	}

	output := cfg.Output
	if output == nil {
		output = os.Stdout
	}

	level := parseLevel(cfg.Level)

	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: level,
	}

	if cfg.JSON {
		handler = slog.NewJSONHandler(output, opts)
	} else {
		handler = slog.NewTextHandler(output, opts)
	}

	return &Logger{slog.New(handler)}
}

// With creates a child logger with additional context attributes.
func (l *Logger) With(attrs ...any) *Logger {
	return &Logger{l.Logger.With(attrs...)}
}

// WithComponent creates a logger tagged with a component name for easy filtering.
func (l *Logger) WithComponent(name string) *Logger {
	return l.With("component", name)
}

// Debug logs at debug level.
func (l *Logger) Debug(msg string, args ...any) {
	l.Logger.Debug(msg, args...)
}

// Info logs at info level.
func (l *Logger) Info(msg string, args ...any) {
	l.Logger.Info(msg, args...)
}

// Warn logs at warning level.
func (l *Logger) Warn(msg string, args ...any) {
	l.Logger.Warn(msg, args...)
}

// Error logs at error level.
func (l *Logger) Error(msg string, args ...any) {
	l.Logger.Error(msg, args...)
}

// DebugContext logs at debug level with context.
func (l *Logger) DebugContext(ctx context.Context, msg string, args ...any) {
	l.Logger.DebugContext(ctx, msg, args...)
}

// InfoContext logs at info level with context.
func (l *Logger) InfoContext(ctx context.Context, msg string, args ...any) {
	l.Logger.InfoContext(ctx, msg, args...)
}

// WarnContext logs at warning level with context.
func (l *Logger) WarnContext(ctx context.Context, msg string, args ...any) {
	l.Logger.WarnContext(ctx, msg, args...)
}

// ErrorContext logs at error level with context.
func (l *Logger) ErrorContext(ctx context.Context, msg string, args ...any) {
	l.Logger.ErrorContext(ctx, msg, args...)
}

// parseLevel converts a string log level name to a slog.Level.
// Accepts: "debug", "info", "warn", "error" (case-insensitive).
// Defaults to LevelInfo for unrecognized values.
func parseLevel(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

// Default returns a logger configured with default settings (info level, stdout, text format).
var Default = New(nil)
