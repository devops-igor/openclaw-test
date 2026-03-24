package types

import (
	"errors"
	"fmt"
	"testing"
)

func TestDownloadError(t *testing.T) {
	t.Run("with URL", func(t *testing.T) {
		e := &DownloadError{
			URL:     "https://youtube.com/watch?v=test",
			Message: "file too large",
		}
		want := `download error for https://youtube.com/watch?v=test: file too large`
		if got := e.Error(); got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})

	t.Run("without URL", func(t *testing.T) {
		e := &DownloadError{
			Message: "no URL provided",
		}
		want := "download error: no URL provided"
		if got := e.Error(); got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})

	t.Run("unwraps underlying error", func(t *testing.T) {
		inner := errors.New("connection refused")
		e := &DownloadError{
			URL:     "https://example.com",
			Message: "network failure",
			Err:     inner,
		}
		if !errors.Is(e, inner) {
			t.Error("errors.Is should match the inner error")
		}
	})

	t.Run("unwrap returns nil when no inner error", func(t *testing.T) {
		e := &DownloadError{Message: "test"}
		if e.Unwrap() != nil {
			t.Error("Unwrap() should return nil when no inner error")
		}
	})

	t.Run("supports errors.As", func(t *testing.T) {
		inner := &DownloadError{URL: "test", Message: "fail"}
		wrapped := fmt.Errorf("outer: %w", inner)

		var de *DownloadError
		if !errors.As(wrapped, &de) {
			t.Fatal("errors.As should find DownloadError")
		}
		if de.URL != "test" {
			t.Errorf("URL = %q, want %q", de.URL, "test")
		}
	})
}

func TestValidationError(t *testing.T) {
	t.Run("with field", func(t *testing.T) {
		e := &ValidationError{
			Field:   "telegram_token",
			Message: "must not be empty",
		}
		want := `validation error on field "telegram_token": must not be empty`
		if got := e.Error(); got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})

	t.Run("without field", func(t *testing.T) {
		e := &ValidationError{
			Message: "invalid input",
		}
		want := "validation error: invalid input"
		if got := e.Error(); got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})

	t.Run("unwrap returns nil", func(t *testing.T) {
		e := &ValidationError{Message: "test"}
		if e.Unwrap() != nil {
			t.Error("Unwrap() should return nil")
		}
	})

	t.Run("supports errors.As", func(t *testing.T) {
		inner := &ValidationError{Field: "url", Message: "invalid"}
		wrapped := fmt.Errorf("outer: %w", inner)

		var ve *ValidationError
		if !errors.As(wrapped, &ve) {
			t.Fatal("errors.As should find ValidationError")
		}
		if ve.Field != "url" {
			t.Errorf("Field = %q, want %q", ve.Field, "url")
		}
	})
}

func TestConfigError(t *testing.T) {
	t.Run("with key", func(t *testing.T) {
		e := &ConfigError{
			Key:     "DOWNLOAD_DIR",
			Message: "directory does not exist",
		}
		want := `config error for "DOWNLOAD_DIR": directory does not exist`
		if got := e.Error(); got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})

	t.Run("without key", func(t *testing.T) {
		e := &ConfigError{
			Message: "missing required config",
		}
		want := "config error: missing required config"
		if got := e.Error(); got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})

	t.Run("unwraps underlying error", func(t *testing.T) {
		inner := errors.New("permission denied")
		e := &ConfigError{
			Key:     "LOG_PATH",
			Message: "cannot write log file",
			Err:     inner,
		}
		if !errors.Is(e, inner) {
			t.Error("errors.Is should match the inner error")
		}
	})

	t.Run("unwrap returns nil when no inner error", func(t *testing.T) {
		e := &ConfigError{Message: "test"}
		if e.Unwrap() != nil {
			t.Error("Unwrap() should return nil when no inner error")
		}
	})

	t.Run("supports errors.As", func(t *testing.T) {
		inner := &ConfigError{Key: "TOKEN", Message: "missing"}
		wrapped := fmt.Errorf("config load failed: %w", inner)

		var ce *ConfigError
		if !errors.As(wrapped, &ce) {
			t.Fatal("errors.As should find ConfigError")
		}
		if ce.Key != "TOKEN" {
			t.Errorf("Key = %q, want %q", ce.Key, "TOKEN")
		}
	})
}

// TestErrorWrappingChain tests nested error wrapping across all custom error types.
func TestErrorWrappingChain(t *testing.T) {
	base := errors.New("connection refused")
	dlErr := &DownloadError{URL: "https://test.com", Message: "download failed", Err: base}
	configErr := &ConfigError{Key: "YT_DLP_PATH", Message: "downloader failed", Err: dlErr}

	// Should unwrap through the chain
	if !errors.Is(configErr, base) {
		t.Error("errors.Is should find the base error through the chain")
	}

	// Should find intermediate types
	var de *DownloadError
	if !errors.As(configErr, &de) {
		t.Error("errors.As should find DownloadError in the chain")
	}

	var ce *ConfigError
	if !errors.As(configErr, &ce) {
		t.Error("errors.As should find ConfigError")
	}
}
