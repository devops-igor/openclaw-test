// Package downloader provides yt-dlp command execution and output parsing.
package downloader

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/igorkon/youtube-downloader-bot/pkg/logger"
)

var (
	// ErrYtDlpNotFound is returned when the yt-dlp binary cannot be found.
	ErrYtDlpNotFound = errors.New("yt-dlp binary not found in PATH")
	// ErrInvalidURL is returned when the URL is empty or clearly invalid.
	ErrInvalidURL = errors.New("invalid or empty URL")
	// ErrNetworkError is returned when yt-dlp reports a network-related failure.
	ErrNetworkError = errors.New("network error during yt-dlp execution")
	// ErrTimeout is returned when the command exceeds its context deadline.
	ErrTimeout = errors.New("yt-dlp command timed out")
)

// Executor wraps yt-dlp command execution with logging and error handling.
type Executor struct {
	// ytDlpPath is the path to the yt-dlp binary.
	ytDlpPath string
	// cookiesFromBrowser is the browser name for --cookies-from-browser (e.g. "chrome", "firefox").
	cookiesFromBrowser string
	// cookiesFile is the path to a Netscape-format cookies file for --cookies.
	cookiesFile string
	// log is the logger for execution events.
	log *logger.Logger
}

// NewExecutor creates a new Executor with the given yt-dlp binary path and logger.
// If ytDlpPath is empty, defaults to "yt-dlp" (assumed in PATH).
// cookiesFromBrowser and cookiesFile control cookie authentication for yt-dlp.
// If both are set, --cookies-from-browser takes precedence.
func NewExecutor(ytDlpPath string, log *logger.Logger, cookiesFromBrowser string, cookiesFile string) *Executor {
	if ytDlpPath == "" {
		ytDlpPath = "yt-dlp"
	}
	return &Executor{
		ytDlpPath:          ytDlpPath,
		cookiesFromBrowser: cookiesFromBrowser,
		cookiesFile:        cookiesFile,
		log:                log.WithComponent("downloader"),
	}
}

// cookieFlags returns the yt-dlp flags for cookie authentication.
// If cookiesFromBrowser is set, it takes precedence over cookiesFile.
func (e *Executor) cookieFlags() []string {
	if e.cookiesFromBrowser != "" {
		return []string{"--cookies-from-browser", e.cookiesFromBrowser}
	}
	if e.cookiesFile != "" {
		return []string{"--cookies", e.cookiesFile}
	}
	return nil
}

// ExecResult holds the captured output from a yt-dlp command execution.
type ExecResult struct {
	Stdout string // Standard output
	Stderr string // Standard error
}

// Run executes a yt-dlp command with the given arguments.
// It captures stdout and stderr separately. The context can be used for timeout/cancellation.
// Returns ErrYtDlpNotFound if the binary is missing, ErrInvalidURL if args contain an empty URL,
// ErrNetworkError if stderr indicates a network failure, or ErrTimeout on context cancellation.
func (e *Executor) Run(ctx context.Context, args ...string) (*ExecResult, error) {
	// Basic URL validation first (before binary check)
	if len(args) > 0 {
		last := args[len(args)-1]
		if last == "" {
			return nil, ErrInvalidURL
		}
	}

	// Validate the binary exists
	if err := e.validateBinary(); err != nil {
		return nil, err
	}

	// Prepend cookie flags if configured
	allArgs := append(e.cookieFlags(), args...)

	e.log.Debug("Executing yt-dlp", "path", e.ytDlpPath, "args", allArgs)

	cmd := exec.CommandContext(ctx, e.ytDlpPath, allArgs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	result := &ExecResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if err != nil {
		return e.handleError(ctx, err, result)
	}

	e.log.Debug("yt-dlp completed successfully", "args", allArgs, "stdout_len", len(result.Stdout), "stderr_len", len(result.Stderr))
	return result, nil
}

// validateBinary checks that yt-dlp is available at the configured path.
func (e *Executor) validateBinary() error {
	path, err := exec.LookPath(e.ytDlpPath)
	if err != nil {
		e.log.Error("yt-dlp binary not found", "path", e.ytDlpPath, "error", err)
		return fmt.Errorf("%w: %s", ErrYtDlpNotFound, e.ytDlpPath)
	}
	e.log.Debug("Found yt-dlp binary", "resolved_path", path)
	return nil
}

// handleError classifies the error from yt-dlp execution into a typed error.
func (e *Executor) handleError(ctx context.Context, err error, result *ExecResult) (*ExecResult, error) {
	stderrLower := strings.ToLower(result.Stderr)

	// Check for context timeout/cancellation
	if ctx.Err() != nil {
		e.log.Warn("yt-dlp command timed out or cancelled", "error", ctx.Err())
		return result, fmt.Errorf("%w: %v", ErrTimeout, ctx.Err())
	}

	// Check for network-related errors in stderr
	networkKeywords := []string{
		"unable to download",
		"network error",
		"connection refused",
		"connection timed out",
		"could not connect",
		"no route to host",
		"temporary failure in name resolution",
		"http error 5",
		"download error",
	}
	for _, kw := range networkKeywords {
		if strings.Contains(stderrLower, kw) {
			e.log.Error("yt-dlp network error", "keyword", kw, "stderr", result.Stderr)
			return result, fmt.Errorf("%w: %s", ErrNetworkError, strings.TrimSpace(result.Stderr))
		}
	}

	// Check for "not a valid URL" type errors
	if strings.Contains(stderrLower, "not a valid url") || strings.Contains(stderrLower, "is not a valid url") {
		e.log.Error("yt-dlp received invalid URL", "stderr", result.Stderr)
		return result, fmt.Errorf("%w: %s", ErrInvalidURL, strings.TrimSpace(result.Stderr))
	}

	// Generic execution error
	e.log.Error("yt-dlp execution failed", "error", err, "stderr", result.Stderr)
	return result, fmt.Errorf("yt-dlp execution failed: %w\nstderr: %s", err, strings.TrimSpace(result.Stderr))
}
