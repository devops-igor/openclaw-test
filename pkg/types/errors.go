// Package types provides shared types used across the application.
package types

import "fmt"

// DownloadError represents an error that occurred during a download operation.
type DownloadError struct {
	URL     string // The URL that failed to download
	Message string // Human-readable error description
	Err     error  // Underlying error, if any
}

func (e *DownloadError) Error() string {
	if e.URL != "" {
		return fmt.Sprintf("download error for %s: %s", e.URL, e.Message)
	}
	return fmt.Sprintf("download error: %s", e.Message)
}

func (e *DownloadError) Unwrap() error {
	return e.Err
}

// ValidationError represents an input validation failure.
type ValidationError struct {
	Field   string // The field that failed validation
	Message string // Human-readable error description
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("validation error on field %q: %s", e.Field, e.Message)
	}
	return fmt.Sprintf("validation error: %s", e.Message)
}

func (e *ValidationError) Unwrap() error {
	return nil
}

// ConfigError represents a configuration-related error.
type ConfigError struct {
	Key     string // Configuration key that caused the error
	Message string // Human-readable error description
	Err     error  // Underlying error, if any
}

func (e *ConfigError) Error() string {
	if e.Key != "" {
		return fmt.Sprintf("config error for %q: %s", e.Key, e.Message)
	}
	return fmt.Sprintf("config error: %s", e.Message)
}

func (e *ConfigError) Unwrap() error {
	return e.Err
}
