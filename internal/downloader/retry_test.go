package downloader

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/igorkon/youtube-downloader-bot/pkg/logger"
)

func TestWithRetry_SucceedsFirstAttempt(t *testing.T) {
	log := logger.New(&logger.Config{Level: "debug", Output: os.Stderr})
	calls := 0

	err := withRetry(context.Background(), log, 3, 10*time.Millisecond, func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestWithRetry_SucceedsOnSecondAttempt(t *testing.T) {
	log := logger.New(&logger.Config{Level: "debug", Output: os.Stderr})
	calls := 0

	err := withRetry(context.Background(), log, 3, 10*time.Millisecond, func() error {
		calls++
		if calls < 2 {
			return ErrNetworkError
		}
		return nil
	})
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls, got %d", calls)
	}
}

func TestWithRetry_ExhaustsAttempts(t *testing.T) {
	log := logger.New(&logger.Config{Level: "debug", Output: os.Stderr})
	calls := 0

	err := withRetry(context.Background(), log, 3, 10*time.Millisecond, func() error {
		calls++
		return ErrNetworkError
	})
	if err == nil {
		t.Error("expected error after exhausting attempts")
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
	if !errors.Is(err, ErrNetworkError) {
		t.Errorf("expected ErrNetworkError, got: %v", err)
	}
}

func TestWithRetry_DoesNotRetryInvalidURL(t *testing.T) {
	log := logger.New(&logger.Config{Level: "debug", Output: os.Stderr})
	calls := 0

	err := withRetry(context.Background(), log, 3, 10*time.Millisecond, func() error {
		calls++
		return ErrInvalidURL
	})
	if err == nil {
		t.Error("expected error")
	}
	if calls != 1 {
		t.Errorf("expected 1 call (no retry for invalid URL), got %d", calls)
	}
}

func TestWithRetry_DoesNotRetryYtDlpNotFound(t *testing.T) {
	log := logger.New(&logger.Config{Level: "debug", Output: os.Stderr})
	calls := 0

	err := withRetry(context.Background(), log, 3, 10*time.Millisecond, func() error {
		calls++
		return ErrYtDlpNotFound
	})
	if err == nil {
		t.Error("expected error")
	}
	if calls != 1 {
		t.Errorf("expected 1 call (no retry for not found), got %d", calls)
	}
}

func TestWithRetry_RetryOnTimeout(t *testing.T) {
	log := logger.New(&logger.Config{Level: "debug", Output: os.Stderr})
	calls := 0

	err := withRetry(context.Background(), log, 3, 10*time.Millisecond, func() error {
		calls++
		if calls < 3 {
			return ErrTimeout
		}
		return nil
	})
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestWithRetry_DoesNotRetryGenericError(t *testing.T) {
	log := logger.New(&logger.Config{Level: "debug", Output: os.Stderr})
	calls := 0
	genericErr := errors.New("some permanent error")

	err := withRetry(context.Background(), log, 3, 10*time.Millisecond, func() error {
		calls++
		return genericErr
	})
	if err == nil {
		t.Error("expected error")
	}
	if calls != 1 {
		t.Errorf("expected 1 call (no retry for generic error), got %d", calls)
	}
}

func TestWithRetry_ContextCancellation(t *testing.T) {
	log := logger.New(&logger.Config{Level: "debug", Output: os.Stderr})
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0

	err := withRetry(ctx, log, 3, 100*time.Millisecond, func() error {
		calls++
		if calls == 1 {
			// Cancel context during first retry delay
			go func() {
				time.Sleep(5 * time.Millisecond)
				cancel()
			}()
		}
		return ErrNetworkError
	})
	if err == nil {
		t.Error("expected error")
	}
	// Should have been cancelled before completing all 3 attempts
	if calls >= 3 {
		t.Errorf("expected fewer than 3 calls due to cancellation, got %d", calls)
	}
}
