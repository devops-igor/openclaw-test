package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_FromYAMLOnly(t *testing.T) {
	// Create a temporary YAML config file
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "test.yaml")
	yamlContent := `
telegram:
  token: "token-from-yaml"
  allowed_users: [12345, 67890]
downloader:
  yt_dlp_path: "/usr/bin/yt-dlp"
  download_dir: "/tmp/downloads"
  max_file_size_mb: 100
  rate_limit_per_user: 5
storage:
  file_retention_minutes: 120
`
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write temp config: %v", err)
	}

	// Set CONFIG_PATH to our temp file
	os.Setenv("CONFIG_PATH", cfgPath)
	// Ensure no interfering env vars
	os.Unsetenv("TELEGRAM_BOT_TOKEN")
	os.Unsetenv("ALLOWED_USERS")
	os.Unsetenv("YT_DLP_PATH")
	os.Unsetenv("DOWNLOAD_DIR")
	os.Unsetenv("MAX_FILE_SIZE_MB")
	os.Unsetenv("RATE_LIMIT_PER_USER")
	os.Unsetenv("FILE_RETENTION_MINUTES")
	os.Unsetenv("COOKIES_FROM_BROWSER")
	os.Unsetenv("COOKIES_FILE")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.Telegram.Token != "token-from-yaml" {
		t.Errorf("Telegram.Token = %q, want %q", cfg.Telegram.Token, "token-from-yaml")
	}
	if len(cfg.Telegram.AllowedUsers) != 2 {
		t.Errorf("Telegram.AllowedUsers length = %d, want %d", len(cfg.Telegram.AllowedUsers), 2)
	}
	if cfg.Telegram.AllowedUsers[0] != 12345 || cfg.Telegram.AllowedUsers[1] != 67890 {
		t.Errorf("Telegram.AllowedUsers = %v, want [12345, 67890]", cfg.Telegram.AllowedUsers)
	}
	if cfg.Downloader.YtDlpPath != "/usr/bin/yt-dlp" {
		t.Errorf("Downloader.YtDlpPath = %q, want %q", cfg.Downloader.YtDlpPath, "/usr/bin/yt-dlp")
	}
	if cfg.Downloader.DownloadDir != "/tmp/downloads" {
		t.Errorf("Downloader.DownloadDir = %q, want %q", cfg.Downloader.DownloadDir, "/tmp/downloads")
	}
	if cfg.Downloader.MaxFileSizeMB != 100 {
		t.Errorf("Downloader.MaxFileSizeMB = %d, want %d", cfg.Downloader.MaxFileSizeMB, 100)
	}
	if cfg.Downloader.RateLimitPerUser != 5 {
		t.Errorf("Downloader.RateLimitPerUser = %d, want %d", cfg.Downloader.RateLimitPerUser, 5)
	}
	if cfg.Storage.FileRetentionMinutes != 120 {
		t.Errorf("Storage.FileRetentionMinutes = %d, want %d", cfg.Storage.FileRetentionMinutes, 120)
	}
}

func TestLoad_FromEnvOnly(t *testing.T) {
	// No YAML file; use env vars only
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "nonexistent.yaml")
	os.Setenv("CONFIG_PATH", cfgPath)

	os.Setenv("TELEGRAM_BOT_TOKEN", "token-from-env")
	os.Setenv("ALLOWED_USERS", "111, 222 ,333")
	os.Setenv("YT_DLP_PATH", "/custom/yt-dlp")
	os.Setenv("DOWNLOAD_DIR", "/custom/downloads")
	os.Setenv("MAX_FILE_SIZE_MB", "200")
	os.Setenv("RATE_LIMIT_PER_USER", "20")
	os.Setenv("FILE_RETENTION_MINUTES", "180")
	os.Setenv("COOKIES_FROM_BROWSER", "chrome")
	os.Setenv("COOKIES_FILE", "/path/to/cookies.txt")
	// Ensure CONFIG_PATH points to non-existent file

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.Telegram.Token != "token-from-env" {
		t.Errorf("Telegram.Token = %q, want %q", cfg.Telegram.Token, "token-from-env")
	}
	if len(cfg.Telegram.AllowedUsers) != 3 {
		t.Errorf("Telegram.AllowedUsers length = %d, want %d", len(cfg.Telegram.AllowedUsers), 3)
	}
	expectedUsers := []int64{111, 222, 333}
	for i, want := range expectedUsers {
		if cfg.Telegram.AllowedUsers[i] != want {
			t.Errorf("Telegram.AllowedUsers[%d] = %d, want %d", i, cfg.Telegram.AllowedUsers[i], want)
		}
	}
	if cfg.Downloader.YtDlpPath != "/custom/yt-dlp" {
		t.Errorf("Downloader.YtDlpPath = %q, want %q", cfg.Downloader.YtDlpPath, "/custom/yt-dlp")
	}
	if cfg.Downloader.DownloadDir != "/custom/downloads" {
		t.Errorf("Downloader.DownloadDir = %q, want %q", cfg.Downloader.DownloadDir, "/custom/downloads")
	}
	if cfg.Downloader.MaxFileSizeMB != 200 {
		t.Errorf("Downloader.MaxFileSizeMB = %d, want %d", cfg.Downloader.MaxFileSizeMB, 200)
	}
	if cfg.Downloader.RateLimitPerUser != 20 {
		t.Errorf("Downloader.RateLimitPerUser = %d, want %d", cfg.Downloader.RateLimitPerUser, 20)
	}
	if cfg.Storage.FileRetentionMinutes != 180 {
		t.Errorf("Storage.FileRetentionMinutes = %d, want %d", cfg.Storage.FileRetentionMinutes, 180)
	}
	if cfg.Downloader.CookiesFromBrowser != "chrome" {
		t.Errorf("Downloader.CookiesFromBrowser = %q, want %q", cfg.Downloader.CookiesFromBrowser, "chrome")
	}
	if cfg.Downloader.CookiesFile != "/path/to/cookies.txt" {
		t.Errorf("Downloader.CookiesFile = %q, want %q", cfg.Downloader.CookiesFile, "/path/to/cookies.txt")
	}
}

func TestLoad_EnvOverridesYAML(t *testing.T) {
	// Both YAML and env present; env should override
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "test.yaml")
	yamlContent := `
telegram:
  token: "token-yaml"
  allowed_users: [100, 200]
downloader:
  yt_dlp_path: "yt-dlp-yaml"
  download_dir: "dir-yaml"
  max_file_size_mb: 50
  rate_limit_per_user: 10
storage:
  file_retention_minutes: 60
`
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write temp config: %v", err)
	}
	os.Setenv("CONFIG_PATH", cfgPath)
	os.Setenv("TELEGRAM_BOT_TOKEN", "token-env")
	os.Setenv("ALLOWED_USERS", "999")
	os.Setenv("YT_DLP_PATH", "yt-dlp-env")
	os.Setenv("DOWNLOAD_DIR", "dir-env")
	os.Setenv("MAX_FILE_SIZE_MB", "150")
	os.Setenv("RATE_LIMIT_PER_USER", "15")
	os.Setenv("FILE_RETENTION_MINUTES", "90")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	// Telegram: token and allowed_users from env
	if cfg.Telegram.Token != "token-env" {
		t.Errorf("Telegram.Token = %q, want %q", cfg.Telegram.Token, "token-env")
	}
	if len(cfg.Telegram.AllowedUsers) != 1 || cfg.Telegram.AllowedUsers[0] != 999 {
		t.Errorf("Telegram.AllowedUsers = %v, want [999]", cfg.Telegram.AllowedUsers)
	}
	// Downloader: all from env
	if cfg.Downloader.YtDlpPath != "yt-dlp-env" {
		t.Errorf("Downloader.YtDlpPath = %q, want %q", cfg.Downloader.YtDlpPath, "yt-dlp-env")
	}
	if cfg.Downloader.DownloadDir != "dir-env" {
		t.Errorf("Downloader.DownloadDir = %q, want %q", cfg.Downloader.DownloadDir, "dir-env")
	}
	if cfg.Downloader.MaxFileSizeMB != 150 {
		t.Errorf("Downloader.MaxFileSizeMB = %d, want %d", cfg.Downloader.MaxFileSizeMB, 150)
	}
	if cfg.Downloader.RateLimitPerUser != 15 {
		t.Errorf("Downloader.RateLimitPerUser = %d, want %d", cfg.Downloader.RateLimitPerUser, 15)
	}
	// Storage: from env
	if cfg.Storage.FileRetentionMinutes != 90 {
		t.Errorf("Storage.FileRetentionMinutes = %d, want %d", cfg.Storage.FileRetentionMinutes, 90)
	}
}

func TestLoad_MissingRequiredToken(t *testing.T) {
	// No env var and no token in YAML
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "test.yaml")
	yamlContent := `
telegram:
  allowed_users: [123]
downloader:
  download_dir: "./downloads"
storage:
  file_retention_minutes: 60
`
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write temp config: %v", err)
	}
	os.Setenv("CONFIG_PATH", cfgPath)
	os.Unsetenv("TELEGRAM_BOT_TOKEN")

	cfg, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for missing token, got nil")
	}
	if cfg != nil {
		t.Errorf("Load() returned non-nil cfg with error: %v", err)
	}
}

func TestLoad_DefaultsApplied(t *testing.T) {
	// Only token set; all other values should default
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "test.yaml")
	yamlContent := `
telegram:
  token: "mytoken"
`
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write temp config: %v", err)
	}
	os.Setenv("CONFIG_PATH", cfgPath)
	// Ensure no env overrides
	os.Unsetenv("ALLOWED_USERS")
	os.Unsetenv("YT_DLP_PATH")
	os.Unsetenv("DOWNLOAD_DIR")
	os.Unsetenv("MAX_FILE_SIZE_MB")
	os.Unsetenv("RATE_LIMIT_PER_USER")
	os.Unsetenv("FILE_RETENTION_MINUTES")
	os.Unsetenv("COOKIES_FROM_BROWSER")
	os.Unsetenv("COOKIES_FILE")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.Telegram.Token != "mytoken" {
		t.Errorf("Telegram.Token = %q, want %q", cfg.Telegram.Token, "mytoken")
	}
	// AllowedUsers should be nil slice (no defaults, empty allowed)
	if len(cfg.Telegram.AllowedUsers) != 0 {
		t.Errorf("Telegram.AllowedUsers length = %d, want %d", len(cfg.Telegram.AllowedUsers), 0)
	}
	// Downloader defaults
	if cfg.Downloader.YtDlpPath != "yt-dlp" {
		t.Errorf("Downloader.YtDlpPath = %q, want %q", cfg.Downloader.YtDlpPath, "yt-dlp")
	}
	if cfg.Downloader.DownloadDir != "./downloads" {
		t.Errorf("Downloader.DownloadDir = %q, want %q", cfg.Downloader.DownloadDir, "./downloads")
	}
	if cfg.Downloader.MaxFileSizeMB != 50 {
		t.Errorf("Downloader.MaxFileSizeMB = %d, want %d", cfg.Downloader.MaxFileSizeMB, 50)
	}
	if cfg.Downloader.RateLimitPerUser != 10 {
		t.Errorf("Downloader.RateLimitPerUser = %d, want %d", cfg.Downloader.RateLimitPerUser, 10)
	}
	if cfg.Downloader.CookiesFromBrowser != "" {
		t.Errorf("Downloader.CookiesFromBrowser = %q, want %q", cfg.Downloader.CookiesFromBrowser, "")
	}
	if cfg.Downloader.CookiesFile != "" {
		t.Errorf("Downloader.CookiesFile = %q, want %q", cfg.Downloader.CookiesFile, "")
	}
	// Storage defaults
	if cfg.Storage.FileRetentionMinutes != 60 {
		t.Errorf("Storage.FileRetentionMinutes = %d, want %d", cfg.Storage.FileRetentionMinutes, 60)
	}
	// Share defaults
	if cfg.Share.Host != "" {
		t.Errorf("Share.Host = %q, want %q", cfg.Share.Host, "")
	}
	if cfg.Share.Port != 8080 {
		t.Errorf("Share.Port = %d, want %d", cfg.Share.Port, 8080)
	}
	if cfg.Share.TimeoutMinutes != 30 {
		t.Errorf("Share.TimeoutMinutes = %d, want %d", cfg.Share.TimeoutMinutes, 30)
	}
}

func TestLoad_ShareEnvOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "test.yaml")
	yamlContent := `
telegram:
  token: "mytoken"
`
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write temp config: %v", err)
	}
	os.Setenv("CONFIG_PATH", cfgPath)
	os.Setenv("SHARE_HOST", "http://myserver.example.com")
	os.Setenv("SHARE_PORT", "9090")
	os.Setenv("SHARE_TIMEOUT_MINUTES", "60")
	defer os.Unsetenv("SHARE_HOST")
	defer os.Unsetenv("SHARE_PORT")
	defer os.Unsetenv("SHARE_TIMEOUT_MINUTES")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.Share.Host != "http://myserver.example.com" {
		t.Errorf("Share.Host = %q, want %q", cfg.Share.Host, "http://myserver.example.com")
	}
	if cfg.Share.Port != 9090 {
		t.Errorf("Share.Port = %d, want %d", cfg.Share.Port, 9090)
	}
	if cfg.Share.TimeoutMinutes != 60 {
		t.Errorf("Share.TimeoutMinutes = %d, want %d", cfg.Share.TimeoutMinutes, 60)
	}
}

func TestParseAllowedUsers(t *testing.T) {
	tests := []struct {
		input string
		want  []int64
	}{
		{"", []int64{}},
		{"   ", []int64{}},
		{"123", []int64{123}},
		{"123,456,789", []int64{123, 456, 789}},
		{" 123 , 456 , 789 ", []int64{123, 456, 789}},
		{"123,,456", []int64{123, 456}},      // skip empty
		{"123, abc, 456", []int64{123, 456}}, // skip invalid
		{"-5", []int64{-5}},                  // negative allowed
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseAllowedUsers(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("parseAllowedUsers(%q) = %v, length %d; want length %d", tt.input, got, len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseAllowedUsers(%q)[%d] = %d, want %d", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}
