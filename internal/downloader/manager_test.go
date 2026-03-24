package downloader

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// MockExecutor is a mock implementation of the Executor for testing
type MockExecutor struct {
	RunFunc func(ctx context.Context, args ...string) (*ExecResult, error)
}

func (m *MockExecutor) Run(ctx context.Context, args ...string) (*ExecResult, error) {
	if m.RunFunc != nil {
		return m.RunFunc(ctx, args...)
	}
	return &ExecResult{}, nil
}

// TestDownloadSuccess tests successful download
func TestDownloadSuccess(t *testing.T) {
	// Create temporary directory for test
	tempDir := t.TempDir()

	// Create a test file that will be "downloaded"
	testFile := filepath.Join(tempDir, "test_video.mp4")
	testContent := []byte("test video content")
	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create mock executor that returns the file path
	mockExecutor := &MockExecutor{
		RunFunc: func(ctx context.Context, args ...string) (*ExecResult, error) {
			// Simulate yt-dlp output with file path
			return &ExecResult{Stdout: testFile + "\n", Stderr: ""}, nil
		},
	}

	log := newTestLogger()
	manager := New(mockExecutor, log, tempDir, 100, 60)

	// Execute download
	result, err := manager.Download(context.Background(), "https://example.com/video", "123")
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	// Verify result
	if result.FilePath != testFile {
		t.Errorf("expected FilePath %s, got %s", testFile, result.FilePath)
	}
	if result.Filename != "test_video.mp4" {
		t.Errorf("expected Filename test_video.mp4, got %s", result.Filename)
	}
	if result.Filesize != int64(len(testContent)) {
		t.Errorf("expected Filesize %d, got %d", len(testContent), result.Filesize)
	}
	if result.FormatID != "123" {
		t.Errorf("expected FormatID 123, got %s", result.FormatID)
	}
}

// TestDownloadFailure tests download failure handling
func TestDownloadFailure(t *testing.T) {
	tempDir := t.TempDir()

	// Create mock executor that returns an error
	mockExecutor := &MockExecutor{
		RunFunc: func(ctx context.Context, args ...string) (*ExecResult, error) {
			return &ExecResult{Stdout: "", Stderr: "error: video not found"}, errors.New("yt-dlp failed")
		},
	}

	log := newTestLogger()
	manager := New(mockExecutor, log, tempDir, 100, 60)

	// Execute download
	_, err := manager.Download(context.Background(), "https://example.com/invalid", "123")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Verify error message contains expected text
	if !strings.Contains(err.Error(), "download failed") {
		t.Errorf("expected error to contain 'download failed', got: %v", err)
	}
}

// TestDownloadNoSizeValidation verifies Manager does NOT reject oversized files.
// Size routing (Telegram vs HTTP share) is handled by the bot's downloadAndSend().
func TestDownloadNoSizeValidation(t *testing.T) {
	tempDir := t.TempDir()

	// Create a large test file (1MB)
	testFile := filepath.Join(tempDir, "large_video.mp4")
	largeContent := make([]byte, 1024*1024)
	if err := os.WriteFile(testFile, largeContent, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create mock executor
	mockExecutor := &MockExecutor{
		RunFunc: func(ctx context.Context, args ...string) (*ExecResult, error) {
			return &ExecResult{Stdout: testFile + "\n", Stderr: ""}, nil
		},
	}

	log := newTestLogger()
	// maxFileSizeMB=0 should NOT cause rejection — Manager no longer validates size
	manager := New(mockExecutor, log, tempDir, 0, 60)

	// Execute download — should succeed even with "oversized" file
	result, err := manager.Download(context.Background(), "https://example.com/video", "123")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	// Verify the file was returned (not deleted)
	if result.Filesize != 1024*1024 {
		t.Errorf("expected file size %d, got %d", 1024*1024, result.Filesize)
	}

	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Error("file should NOT be deleted — size validation is now the bot's responsibility")
	}
}

// TestCleanup tests file cleanup functionality
func TestCleanup(t *testing.T) {
	tempDir := t.TempDir()

	// Create test files with different modification times
	oldFile := filepath.Join(tempDir, "old_video.mp4")
	newFile := filepath.Join(tempDir, "new_video.mp4")

	// Create files
	if err := os.WriteFile(oldFile, []byte("old content"), 0644); err != nil {
		t.Fatalf("failed to create old file: %v", err)
	}
	if err := os.WriteFile(newFile, []byte("new content"), 0644); err != nil {
		t.Fatalf("failed to create new file: %v", err)
	}

	// Set old file's modification time to 2 hours ago
	oldTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatalf("failed to set old file time: %v", err)
	}

	// Create manager with 60 minute retention
	mockExecutor := &MockExecutor{}
	log := newTestLogger()
	manager := New(mockExecutor, log, tempDir, 100, 60)

	// Execute cleanup
	err := manager.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// Verify old file was deleted
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("expected old file to be deleted")
	}

	// Verify new file still exists
	if _, err := os.Stat(newFile); os.IsNotExist(err) {
		t.Error("expected new file to still exist")
	}
}

// TestCleanupWithContextCancellation tests cleanup with context cancellation
func TestCleanupWithContextCancellation(t *testing.T) {
	tempDir := t.TempDir()

	// Create multiple test files
	for i := 0; i < 5; i++ {
		file := filepath.Join(tempDir, "video"+string(rune('0'+i))+".mp4")
		if err := os.WriteFile(file, []byte("content"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
		// Set all files to be old
		oldTime := time.Now().Add(-2 * time.Hour)
		if err := os.Chtimes(file, oldTime, oldTime); err != nil {
			t.Fatalf("failed to set file time: %v", err)
		}
	}

	// Create context that cancels immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mockExecutor := &MockExecutor{}
	log := newTestLogger()
	manager := New(mockExecutor, log, tempDir, 100, 60)

	// Execute cleanup - should return context error
	err := manager.Cleanup(ctx)
	if err == nil {
		t.Fatal("expected error due to context cancellation")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got: %v", err)
	}
}

// TestCleanupNonExistentDirectory tests cleanup on non-existent directory
func TestCleanupNonExistentDirectory(t *testing.T) {
	tempDir := filepath.Join(os.TempDir(), "non-existent-dir")

	mockExecutor := &MockExecutor{}
	log := newTestLogger()
	manager := New(mockExecutor, log, tempDir, 100, 60)

	// Execute cleanup - should succeed (nothing to clean)
	err := manager.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("Cleanup should succeed on non-existent directory: %v", err)
	}
}
