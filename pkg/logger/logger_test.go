package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestNew_DefaultConfig(t *testing.T) {
	l := New(nil)
	if l == nil {
		t.Fatal("New(nil) returned nil")
	}
	// Should not panic when logging
	l.Info("test message")
}

func TestNew_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	l := New(&Config{
		Level:  "debug",
		Output: &buf,
		JSON:   false,
	})

	l.Info("hello world", "key", "value")
	output := buf.String()

	if !strings.Contains(output, "hello world") {
		t.Errorf("expected log message in output, got: %s", output)
	}
	if !strings.Contains(output, "key=value") {
		t.Errorf("expected key=value in output, got: %s", output)
	}
}

func TestNew_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	l := New(&Config{
		Level:  "info",
		Output: &buf,
		JSON:   true,
	})

	l.Info("json test", "count", 42)
	output := buf.String()

	var parsed map[string]any
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("expected valid JSON output, got error: %v, output: %s", err, output)
	}
	if parsed["msg"] != "json test" {
		t.Errorf("msg = %v, want 'json test'", parsed["msg"])
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  Level
	}{
		{"debug", LevelDebug},
		{"DEBUG", LevelDebug},
		{"info", LevelInfo},
		{"INFO", LevelInfo},
		{"warn", LevelWarn},
		{"warning", LevelWarn},
		{"error", LevelError},
		{"ERROR", LevelError},
		{"", LevelInfo},        // empty defaults to info
		{"unknown", LevelInfo}, // unknown defaults to info
		{" info ", LevelInfo},  // whitespace trimmed
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseLevel(tt.input)
			if got != tt.want {
				t.Errorf("parseLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	l := New(&Config{
		Level:  "warn",
		Output: &buf,
	})

	// Debug and Info should be filtered out
	l.Debug("should not appear")
	l.Info("should not appear")
	l.Warn("should appear")
	l.Error("should also appear")

	output := buf.String()
	if strings.Contains(output, "should not appear") {
		t.Errorf("debug/info messages should be filtered at warn level, got: %s", output)
	}
	if !strings.Contains(output, "should appear") {
		t.Errorf("warn message should appear, got: %s", output)
	}
	if !strings.Contains(output, "should also appear") {
		t.Errorf("error message should appear, got: %s", output)
	}
}

func TestLogger_With(t *testing.T) {
	var buf bytes.Buffer
	l := New(&Config{
		Level:  "info",
		Output: &buf,
	})

	child := l.With("request_id", "abc123")
	child.Info("child log")

	output := buf.String()
	if !strings.Contains(output, "request_id=abc123") {
		t.Errorf("expected request_id in output, got: %s", output)
	}
}

func TestLogger_WithComponent(t *testing.T) {
	var buf bytes.Buffer
	l := New(&Config{
		Level:  "info",
		Output: &buf,
	})

	child := l.WithComponent("downloader")
	child.Info("component log")

	output := buf.String()
	if !strings.Contains(output, "component=downloader") {
		t.Errorf("expected component=downloader in output, got: %s", output)
	}
}

func TestDefault(t *testing.T) {
	if Default == nil {
		t.Fatal("Default logger should not be nil")
	}
	// Should not panic
	Default.Info("default logger works")
}

func TestNew_AllLevelsOutput(t *testing.T) {
	var buf bytes.Buffer
	l := New(&Config{
		Level:  "debug",
		Output: &buf,
	})

	l.Debug("debug msg")
	l.Info("info msg")
	l.Warn("warn msg")
	l.Error("error msg")

	output := buf.String()
	for _, msg := range []string{"debug msg", "info msg", "warn msg", "error msg"} {
		if !strings.Contains(output, msg) {
			t.Errorf("expected %q in output, got: %s", msg, output)
		}
	}
}

func TestLogger_ContextMethods(t *testing.T) {
	var buf bytes.Buffer
	l := New(&Config{
		Level:  "debug",
		Output: &buf,
	})

	ctx := t.Context()
	l.DebugContext(ctx, "debug ctx")
	l.InfoContext(ctx, "info ctx")
	l.WarnContext(ctx, "warn ctx")
	l.ErrorContext(ctx, "error ctx")

	output := buf.String()
	for _, msg := range []string{"debug ctx", "info ctx", "warn ctx", "error ctx"} {
		if !strings.Contains(output, msg) {
			t.Errorf("expected %q in output, got: %s", msg, output)
		}
	}
}

// TestLogger_LevelConstants verifies the level constants match slog values.
func TestLogger_LevelConstants(t *testing.T) {
	if LevelDebug != slog.LevelDebug {
		t.Error("LevelDebug should match slog.LevelDebug")
	}
	if LevelInfo != slog.LevelInfo {
		t.Error("LevelInfo should match slog.LevelInfo")
	}
	if LevelWarn != slog.LevelWarn {
		t.Error("LevelWarn should match slog.LevelWarn")
	}
	if LevelError != slog.LevelError {
		t.Error("LevelError should match slog.LevelError")
	}
}
