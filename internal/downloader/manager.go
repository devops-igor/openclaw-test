package downloader

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/igorkon/youtube-downloader-bot/pkg/logger"
	"github.com/igorkon/youtube-downloader-bot/pkg/types"
)

// withRetry executes fn up to maxAttempts times with exponential backoff.
// It retries only on transient errors (network, timeout) and skips permanent errors.
// The baseDelay doubles on each retry (baseDelay, 2*baseDelay, 4*baseDelay).
func withRetry(ctx context.Context, log *logger.Logger, maxAttempts int, baseDelay time.Duration, fn func() error) error {
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		// Do not retry on permanent errors
		if errors.Is(lastErr, ErrInvalidURL) || errors.Is(lastErr, ErrYtDlpNotFound) {
			return lastErr
		}

		// Check if retryable (network or timeout)
		isRetryable := errors.Is(lastErr, ErrNetworkError) || errors.Is(lastErr, ErrTimeout)
		if !isRetryable {
			return lastErr
		}

		if attempt < maxAttempts {
			delay := baseDelay * time.Duration(1<<(attempt-1))
			log.Warn("retrying operation", "attempt", attempt, "maxAttempts", maxAttempts, "delay", delay, "error", lastErr)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
	}
	return lastErr
}

// ExecutorInterface defines the interface for executing yt-dlp commands
type ExecutorInterface interface {
	Run(ctx context.Context, args ...string) (*ExecResult, error)
}

// Manager handles the full download lifecycle
type Manager struct {
	executor         ExecutorInterface
	log              *logger.Logger
	downloadDir      string
	maxFileSizeMB    int
	retentionMinutes int
}

// New creates a new Manager instance
func New(executor ExecutorInterface, log *logger.Logger, downloadDir string, maxFileSizeMB int, retentionMinutes int) *Manager {
	return &Manager{
		executor:         executor,
		log:              log,
		downloadDir:      downloadDir,
		maxFileSizeMB:    maxFileSizeMB,
		retentionMinutes: retentionMinutes,
	}
}

// Download downloads a video/audio file using the specified format
func (m *Manager) Download(ctx context.Context, url string, formatID string) (*types.DownloadResult, error) {
	m.log.Info("starting download", "url", url, "formatID", formatID)

	// Ensure download directory exists
	if err := os.MkdirAll(m.downloadDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create download directory: %w", err)
	}

	// Check free disk space (must have at least 500MB free)
	freeSpace, err := getFreeDiskSpace(m.downloadDir)
	if err != nil {
		m.log.Warn("could not determine free disk space", "error", err)
	} else if freeSpace < 500*1024*1024 {
		return nil, fmt.Errorf("insufficient disk space: %d bytes free, 500MB minimum required", freeSpace)
	}

	// Ensure audio is included for video-only formats.
	// On YouTube, high-res formats (720p+) are typically video-only streams.
	// Appending +bestaudio makes yt-dlp merge video+audio into the final file.
	// Only modify simple numeric format IDs — leave compound strings (e.g. "137+140")
	// and special keywords (e.g. "best") untouched.
	dlFormat := formatID
	if isNumericFormatID(formatID) {
		dlFormat = formatID + "+bestaudio"
		m.log.Info("appending bestaudio to video-only format", "original", formatID, "effective", dlFormat)
	}

	// Build yt-dlp arguments with unique output template for concurrent safety
	args := []string{
		"-f", dlFormat,
		"-o", filepath.Join(m.downloadDir, "%(title)s-%(id)s.%(ext)s"),
		"--no-playlist",
		"--print", "after_move:filepath",
		url,
	}

	// Execute download with retry (exponential backoff on transient errors)
	var result *ExecResult
	err = withRetry(ctx, m.log, 3, 1*time.Second, func() error {
		var runErr error
		result, runErr = m.executor.Run(ctx, args...)
		return runErr
	})
	if err != nil {
		m.log.Error("download failed after retries", "error", err)
		if result != nil {
			m.log.Error("download failed", "error", err, "stderr", result.Stderr)
		}
		return nil, fmt.Errorf("download failed: %w", err)
	}

	// Parse output to find downloaded file path
	filePath, err := m.parseDownloadOutput(result.Stdout)
	if err != nil {
		// Fallback: try to find the file in download directory
		filePath, _, err = m.findDownloadedFile()
		if err != nil {
			return nil, fmt.Errorf("failed to determine downloaded file: %w", err)
		}
	}

	// Get file info
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat downloaded file: %w", err)
	}

	// NOTE: File size validation is intentionally NOT done here.
	// The bot's downloadAndSend() handles size routing:
	//   - ≤ 50MB → send via Telegram
	//   - > 50MB → serve via local HTTP server for direct download
	// This allows large files to be downloaded and shared directly.

	downloadResult := &types.DownloadResult{
		FilePath: filePath,
		Filename: info.Name(),
		Filesize: info.Size(),
		FormatID: formatID,
	}

	m.log.Info("download completed", "file", downloadResult.Filename, "size", downloadResult.Filesize)
	return downloadResult, nil
}

// parseDownloadOutput extracts the file path from yt-dlp output with path traversal protection
func (m *Manager) parseDownloadOutput(output string) (string, error) {
	// Resolve the download directory to its absolute, cleaned form for prefix comparison
	absDir, err := filepath.Abs(m.downloadDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve download directory: %w", err)
	}

	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// Validate path: must resolve to absolute path and be under download directory
		absPath, err := filepath.Abs(line)
		if err != nil {
			continue
		}

		// Ensure the path is within the download directory (prevent path traversal)
		relPath, err := filepath.Rel(absDir, absPath)
		if err != nil || strings.HasPrefix(relPath, "..") {
			continue
		}

		// Verify the file actually exists
		if _, err := os.Stat(absPath); err == nil {
			return absPath, nil
		}
	}
	return "", fmt.Errorf("could not parse download output")
}

// findDownloadedFile tries to find the most recently modified file in the download directory
// Returns the file path and os.FileInfo directly to avoid TOCTOU race conditions
func (m *Manager) findDownloadedFile() (string, os.FileInfo, error) {
	entries, err := os.ReadDir(m.downloadDir)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read download directory: %w", err)
	}

	var newestPath string
	var newestInfo os.FileInfo
	var newestTime time.Time

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Get FileInfo directly from DirEntry to avoid TOCTOU race
		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().After(newestTime) {
			newestTime = info.ModTime()
			newestPath = filepath.Join(m.downloadDir, info.Name())
			newestInfo = info
		}
	}

	if newestPath == "" {
		return "", nil, fmt.Errorf("no files found in download directory")
	}

	return newestPath, newestInfo, nil
}

// isNumericFormatID returns true if the format ID is a simple numeric string (e.g. "137").
// This distinguishes single video-only format IDs from compound expressions like "137+140"
// or keywords like "best" / "bestvideo+bestaudio".
func isNumericFormatID(id string) bool {
	if id == "" {
		return false
	}
	for _, c := range id {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// Cleanup removes files older than retentionMinutes from the download directory
func (m *Manager) Cleanup(ctx context.Context) error {
	m.log.Info("starting cleanup", "retentionMinutes", m.retentionMinutes)

	entries, err := os.ReadDir(m.downloadDir)
	if err != nil {
		if os.IsNotExist(err) {
			m.log.Info("download directory does not exist, nothing to clean")
			return nil
		}
		return fmt.Errorf("failed to read download directory: %w", err)
	}

	cutoff := time.Now().Add(-time.Duration(m.retentionMinutes) * time.Minute)
	var deleteErrors []error
	deletedCount := 0

	for _, entry := range entries {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			m.log.Warn("failed to get file info", "file", entry.Name(), "error", err)
			continue
		}

		if info.ModTime().Before(cutoff) {
			filePath := filepath.Join(m.downloadDir, info.Name())
			if err := os.Remove(filePath); err != nil {
				m.log.Error("failed to delete file", "file", filePath, "error", err)
				deleteErrors = append(deleteErrors, fmt.Errorf("failed to delete %s: %w", filePath, err))
			} else {
				m.log.Info("deleted old file", "file", info.Name())
				deletedCount++
			}
		}
	}

	m.log.Info("cleanup completed", "deleted", deletedCount)

	if len(deleteErrors) > 0 {
		return fmt.Errorf("cleanup encountered %d errors: %v", len(deleteErrors), deleteErrors)
	}

	return nil
}
