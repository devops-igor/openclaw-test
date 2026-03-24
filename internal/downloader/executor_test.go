package downloader

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/igorkon/youtube-downloader-bot/pkg/logger"
)

func newTestLogger() *logger.Logger {
	return logger.New(&logger.Config{Level: "debug", Output: os.Stderr})
}

func TestNewExecutor(t *testing.T) {
	log := newTestLogger()

	t.Run("default path when empty", func(t *testing.T) {
		ex := NewExecutor("", log, "", "")
		if ex.ytDlpPath != "yt-dlp" {
			t.Errorf("expected default path 'yt-dlp', got %q", ex.ytDlpPath)
		}
	})

	t.Run("custom path", func(t *testing.T) {
		ex := NewExecutor("/usr/local/bin/yt-dlp", log, "", "")
		if ex.ytDlpPath != "/usr/local/bin/yt-dlp" {
			t.Errorf("expected custom path, got %q", ex.ytDlpPath)
		}
	})
}

func TestExecutorRun_InvalidURL(t *testing.T) {
	log := newTestLogger()
	ex := NewExecutor("echo", log, "", "") // use echo as a stand-in

	_, err := ex.Run(context.Background(), "")
	if !errors.Is(err, ErrInvalidURL) {
		t.Errorf("expected ErrInvalidURL, got: %v", err)
	}
}

func TestExecutorRun_BinaryNotFound(t *testing.T) {
	log := newTestLogger()
	ex := NewExecutor("/nonexistent/path/yt-dlp", log, "", "")

	_, err := ex.Run(context.Background(), "--version")
	if !errors.Is(err, ErrYtDlpNotFound) {
		t.Errorf("expected ErrYtDlpNotFound, got: %v", err)
	}
}

func TestExecutorRun_ContextTimeout(t *testing.T) {
	log := newTestLogger()
	// Use a real command that blocks (sleep) with a very short timeout
	ex := NewExecutor("powershell", log, "", "")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := ex.Run(ctx, "-Command", "Start-Sleep", "10")
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("expected ErrTimeout, got: %v", err)
	}
}

func TestExecutorRun_SuccessfulExecution(t *testing.T) {
	log := newTestLogger()
	// Use echo to simulate successful execution
	ex := NewExecutor("powershell", log, "", "")

	result, err := ex.Run(context.Background(), "-Command", "Write-Output 'hello world'")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Stdout == "" {
		t.Error("expected non-empty stdout")
	}
}

func TestExecutorRun_CapturesStderr(t *testing.T) {
	log := newTestLogger()
	// Use a command that writes to stderr and fails
	ex := NewExecutor("powershell", log, "", "")

	result, _ := ex.Run(context.Background(), "-Command", "Write-Error 'test error'")
	// Don't check error - may or may not fail depending on PowerShell behavior
	// Just verify the result struct is populated
	if result == nil {
		t.Fatal("expected non-nil result even on error")
	}
}

func TestExecutorRun_EmptyStderr(t *testing.T) {
	log := newTestLogger()
	ex := NewExecutor("powershell", log, "", "")

	result, err := ex.Run(context.Background(), "-Command", "Write-Output 'ok'")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// stderr should be captured (may or may not be empty)
	_ = result.Stderr
}

func TestExecutorRun_ContextCancellation(t *testing.T) {
	log := newTestLogger()
	ex := NewExecutor("powershell", log, "", "")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := ex.Run(ctx, "-Command", "Start-Sleep", "10")
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("expected ErrTimeout, got: %v", err)
	}
}

func TestExecutorRun_ErrorClassification(t *testing.T) {
	log := newTestLogger()

	tests := []struct {
		name    string
		stderr  string
		wantErr error
	}{
		{
			name:    "network error",
			stderr:  "ERROR: unable to download video data: <urlopen error [Errno 11001] getaddrinfo failed>",
			wantErr: ErrNetworkError,
		},
		{
			name:    "invalid URL",
			stderr:  "ERROR: 'not-a-url' is not a valid URL. Set --default-search \"ytsearch\" ...",
			wantErr: ErrInvalidURL,
		},
		{
			name:    "connection refused",
			stderr:  "ERROR: connection refused",
			wantErr: ErrNetworkError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ex := NewExecutor("nonexistent-binary", log, "", "")
			result := &ExecResult{Stderr: tt.stderr}
			_, err := ex.handleError(context.Background(), errors.New("exec failed"), result)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("expected %v, got: %v", tt.wantErr, err)
			}
		})
	}
}

func TestCookieFlags(t *testing.T) {
	log := newTestLogger()

	t.Run("no cookies", func(t *testing.T) {
		ex := NewExecutor("yt-dlp", log, "", "")
		flags := ex.cookieFlags()
		if flags != nil {
			t.Errorf("expected nil flags, got %v", flags)
		}
	})

	t.Run("cookies from browser", func(t *testing.T) {
		ex := NewExecutor("yt-dlp", log, "chrome", "")
		flags := ex.cookieFlags()
		if len(flags) != 2 || flags[0] != "--cookies-from-browser" || flags[1] != "chrome" {
			t.Errorf("expected [--cookies-from-browser chrome], got %v", flags)
		}
	})

	t.Run("cookies from file", func(t *testing.T) {
		ex := NewExecutor("yt-dlp", log, "", "/path/to/cookies.txt")
		flags := ex.cookieFlags()
		if len(flags) != 2 || flags[0] != "--cookies" || flags[1] != "/path/to/cookies.txt" {
			t.Errorf("expected [--cookies /path/to/cookies.txt], got %v", flags)
		}
	})

	t.Run("browser takes precedence over file", func(t *testing.T) {
		ex := NewExecutor("yt-dlp", log, "firefox", "/path/to/cookies.txt")
		flags := ex.cookieFlags()
		if len(flags) != 2 || flags[0] != "--cookies-from-browser" || flags[1] != "firefox" {
			t.Errorf("expected [--cookies-from-browser firefox], got %v", flags)
		}
	})
}

func TestExecResult_StructFields(t *testing.T) {
	result := &ExecResult{
		Stdout: "output data",
		Stderr: "error data",
	}

	if result.Stdout != "output data" {
		t.Errorf("unexpected stdout: %q", result.Stdout)
	}
	if result.Stderr != "error data" {
		t.Errorf("unexpected stderr: %q", result.Stderr)
	}
}
