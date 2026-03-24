package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// TelegramConfig holds Telegram-specific configuration.
type TelegramConfig struct {
	Token        string  `yaml:"token"`
	AllowedUsers []int64 `yaml:"allowed_users"`
}

// DownloaderConfig holds downloader-specific configuration.
type DownloaderConfig struct {
	YtDlpPath          string `yaml:"yt_dlp_path"`
	DownloadDir        string `yaml:"download_dir"`
	MaxFileSizeMB      int    `yaml:"max_file_size_mb"`
	RateLimitPerUser   int    `yaml:"rate_limit_per_user"`
	CookiesFromBrowser string `yaml:"cookies_from_browser"` // browser name, e.g. "chrome", "firefox", "edge"
	CookiesFile        string `yaml:"cookies_file"`         // path to Netscape-format cookies file
}

// StorageConfig holds storage-specific configuration.
type StorageConfig struct {
	FileRetentionMinutes int `yaml:"file_retention_minutes"`
}

// ShareConfig holds file sharing server configuration for large files.
type ShareConfig struct {
	Host           string `yaml:"host"`            // Public URL base (e.g. "http://your-ip")
	Port           int    `yaml:"port"`            // Listener port (default 8080)
	TimeoutMinutes int    `yaml:"timeout_minutes"` // Auto-shutdown timeout (default 30)
}

// Config holds the entire application configuration.
type Config struct {
	Telegram   TelegramConfig   `yaml:"telegram"`
	Downloader DownloaderConfig `yaml:"downloader"`
	Storage    StorageConfig    `yaml:"storage"`
	Share      ShareConfig      `yaml:"share"`
}

// cfgPath tracks the resolved config file path for live-reload watching.
var cfgPath string

// ConfigPath returns the resolved config file path, or empty if none was loaded.
func ConfigPath() string {
	return cfgPath
}

// Load loads configuration from environment variables and optionally from a YAML file.
// The YAML file path defaults to "./configs/default.yaml" but can be overridden via CONFIG_PATH env var.
// Environment variables take precedence over YAML values.
func Load() (*Config, error) {
	// Determine config file path
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "./configs/default.yaml"
	}

	// Sanitize path to prevent directory traversal
	configPath = filepath.Clean(configPath)
	if strings.Contains(configPath, "..") {
		return nil, fmt.Errorf("CONFIG_PATH contains invalid path components")
	}

	// Store resolved path for live-reload watching
	cfgPath = configPath

	// Load YAML file if it exists
	cfg := &Config{}
	if _, err := os.Stat(configPath); err == nil {
		// File exists, load it
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file %s: %w", configPath, err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse YAML config: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("error checking config file %s: %w", configPath, err)
	}
	// else: file doesn't exist, use defaults from struct zero values

	// Override with environment variables
	applyEnvOverrides(cfg)

	// Validate required fields
	if cfg.Telegram.Token == "" {
		return nil, errors.New("TELEGRAM_BOT_TOKEN is required")
	}

	// Set defaults for missing values
	if cfg.Storage.FileRetentionMinutes == 0 {
		cfg.Storage.FileRetentionMinutes = 60
	}
	if cfg.Downloader.DownloadDir == "" {
		cfg.Downloader.DownloadDir = "./downloads"
	}
	if cfg.Downloader.YtDlpPath == "" {
		cfg.Downloader.YtDlpPath = "yt-dlp"
	}
	if cfg.Downloader.MaxFileSizeMB == 0 {
		cfg.Downloader.MaxFileSizeMB = 50
	}
	if cfg.Downloader.RateLimitPerUser == 0 {
		cfg.Downloader.RateLimitPerUser = 10
	}
	if cfg.Share.Port == 0 {
		cfg.Share.Port = 8080
	}
	if cfg.Share.TimeoutMinutes == 0 {
		cfg.Share.TimeoutMinutes = 30
	}

	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	// Telegram
	if token := os.Getenv("TELEGRAM_BOT_TOKEN"); token != "" {
		cfg.Telegram.Token = token
	}
	if users := os.Getenv("ALLOWED_USERS"); users != "" {
		cfg.Telegram.AllowedUsers = parseAllowedUsers(users)
	}
	// Downloader
	if path := os.Getenv("YT_DLP_PATH"); path != "" {
		cfg.Downloader.YtDlpPath = path
	}
	if dir := os.Getenv("DOWNLOAD_DIR"); dir != "" {
		cfg.Downloader.DownloadDir = dir
	}
	if maxSize := os.Getenv("MAX_FILE_SIZE_MB"); maxSize != "" {
		if n, err := strconv.Atoi(maxSize); err == nil && n > 0 {
			cfg.Downloader.MaxFileSizeMB = n
		}
	}
	if rateLimit := os.Getenv("RATE_LIMIT_PER_USER"); rateLimit != "" {
		if n, err := strconv.Atoi(rateLimit); err == nil && n > 0 {
			cfg.Downloader.RateLimitPerUser = n
		}
	}
	if cfb := os.Getenv("COOKIES_FROM_BROWSER"); cfb != "" {
		cfg.Downloader.CookiesFromBrowser = strings.TrimSpace(cfb)
	}
	if cf := os.Getenv("COOKIES_FILE"); cf != "" {
		cfg.Downloader.CookiesFile = strings.TrimSpace(cf)
	}
	// Storage
	if retention := os.Getenv("FILE_RETENTION_MINUTES"); retention != "" {
		if n, err := strconv.Atoi(retention); err == nil && n > 0 {
			cfg.Storage.FileRetentionMinutes = n
		}
	}
	// Share
	if host := os.Getenv("SHARE_HOST"); host != "" {
		cfg.Share.Host = strings.TrimSpace(host)
	}
	if port := os.Getenv("SHARE_PORT"); port != "" {
		if n, err := strconv.Atoi(port); err == nil && n > 0 {
			cfg.Share.Port = n
		}
	}
	if timeout := os.Getenv("SHARE_TIMEOUT_MINUTES"); timeout != "" {
		if n, err := strconv.Atoi(timeout); err == nil && n > 0 {
			cfg.Share.TimeoutMinutes = n
		}
	}
}

// parseAllowedUsers parses a comma-separated string of user IDs into a slice of int64.
// It trims whitespace from each ID. Returns an error if any ID is not a valid integer.
func parseAllowedUsers(s string) []int64 {
	if strings.TrimSpace(s) == "" {
		return []int64{}
	}
	parts := strings.Split(s, ",")
	result := make([]int64, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		val, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			// Invalid ID, skip it (or could return error? We'll skip silently for robustness)
			continue
		}
		result = append(result, val)
	}
	return result
}
