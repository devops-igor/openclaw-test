package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/igorkon/youtube-downloader-bot/internal/bot"
	"github.com/igorkon/youtube-downloader-bot/internal/config"
	"github.com/igorkon/youtube-downloader-bot/internal/downloader"
	"github.com/igorkon/youtube-downloader-bot/pkg/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	logg := logger.New(&logger.Config{
		Level: "info",
		JSON:  false,
	})

	logg.Info("Starting YouTube Downloader Bot",
		"allowed_users", len(cfg.Telegram.AllowedUsers),
		"rate_limit", cfg.Downloader.RateLimitPerUser,
		"share_port", cfg.Share.Port,
		"share_timeout_min", cfg.Share.TimeoutMinutes,
	)

	executor := downloader.NewExecutor(cfg.Downloader.YtDlpPath, logg, cfg.Downloader.CookiesFromBrowser, cfg.Downloader.CookiesFile)
	parser := downloader.NewParser(logg)
	manager := downloader.New(executor, logg, cfg.Downloader.DownloadDir, cfg.Downloader.MaxFileSizeMB, cfg.Storage.FileRetentionMinutes)

	b, err := bot.New(cfg, logg, executor, parser, manager)
	if err != nil {
		logg.Error("Failed to create bot", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Graceful shutdown on SIGINT/SIGTERM
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		logg.Info("Received signal, shutting down", "signal", sig)
		cancel()
	}()

	if err := b.Run(ctx); err != nil && err != context.Canceled {
		logg.Error("Bot stopped with error", "error", err)
		os.Exit(1)
	}

	logg.Info("Bot stopped gracefully")
}
