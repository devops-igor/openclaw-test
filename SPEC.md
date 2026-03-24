# SPEC.md — YouTube Downloader Bot

## Overview

**Project:** youtube-downloader-bot
**Language:** Go 1.24.0
**Repository:** `openclaw-ai-dev-team/projects/youtube-downloader-bot`
**PM:** Paula (`pm_bot` on openclaw-ai-dev-team)
**Status:** ACTIVE

A Telegram bot that downloads YouTube videos and audio, delivering files directly to the user via Telegram or via a temporary HTTP share server for files exceeding Telegram's size limits. Supports authenticated YouTube access via cookies.

## Requirements

### Functional Requirements

1. **URL Detection:** Recognize valid YouTube URLs (`youtube.com/watch?v=`, `youtu.be/`, `youtube.com/shorts/`) and reject invalid ones.
2. **Format Listing:** Fetch and present available formats via `yt-dlp -F`, filtering to 720p or higher quality.
3. **Quality Selection:** Let the user pick a format by number; the bot downloads and delivers the file.
4. **Audio Merge:** For video-only formats (common at 720p+), automatically append `+bestaudio` to the format ID so yt-dlp merges video and audio streams.
5. **Small Files (≤ 50 MB):** Send directly via Telegram `sendDocument`.
6. **Large Files (> 50 MB):** Serve via a temporary HTTP share server. The user receives a download link in Telegram. The server auto-shuts down after download completes or after a configurable timeout.
7. **Cookie-based Auth:** Support `--cookies-from-browser` and `--cookies` flags for yt-dlp to bypass YouTube bot-detection challenges.
8. **Rate Limiting:** Per-user request throttling (default 10 requests/hour).
9. **User Allowlist:** Only approved Telegram user IDs may interact with the bot.

### Non-Functional Requirements

- Single static binary, no external services required.
- File cleanup after delivery (no permanent storage).
- Graceful error handling with user-friendly messages.
- Configurable via YAML and/or environment variables.

## Architecture

```
┌──────────────────┐
│   Telegram User  │
└──────┬───────────┘
       │
┌──────▼───────────┐     ┌──────────────┐
│   bot.Bot        │────▶│ yt-dlp       │
│  (handler loop)  │     └──────────────┘
│                  │
│  downloadAndSend │
│  ┌─────────────┐ │
│  │ ≤ 50 MB     │─┼──▶ Telegram API (sendDocument)
│  └─────────────┘ │
│  ┌─────────────┐ │
│  │ > 50 MB     │─┼──▶ HTTP Share Server (temporary link)
│  └─────────────┘ │
└──────────────────┘
```

### Packages

| Package | Purpose |
|---|---|
| `cmd/bot` | Entry point — loads config, wires dependencies, starts polling loop |
| `internal/bot` | Telegram handler, URL parsing, format selection, delivery logic |
| `internal/downloader` | `Executor` (runs `yt-dlp`), `Manager` (high-level download orchestration) |
| `internal/config` | YAML + env-var configuration loader |
| `internal/serve` | Temporary HTTP server for sharing large files |

## Configuration

### Config File

Optional YAML at `./configs/default.yaml` (overridden by `CONFIG_PATH` env var). Path is sanitized against directory traversal (`..` components rejected).

```yaml
telegram:
  token: "BOT_TOKEN_HERE"
  allowed_users: [123456789]

downloader:
  yt_dlp_path: "yt-dlp"
  download_dir: "./downloads"
  max_file_size_mb: 50
  rate_limit_per_user: 10
  cookies_from_browser: ""   # e.g. "chrome", "firefox", "edge"
  cookies_file: ""           # path to Netscape-format cookies.txt

storage:
  file_retention_minutes: 60

share:
  host: ""               # public base URL, e.g. "http://203.0.113.5" (blank = auto-detect)
  port: 8080             # listener port (0 = OS-assigned)
  timeout_minutes: 30    # auto-shutdown after this many minutes
```

### Environment Variables (override YAML)

| Variable | Description | Default |
|---|---|---|
| `TELEGRAM_BOT_TOKEN` | **Required.** Telegram Bot API token | — |
| `ALLOWED_USERS` | Comma-separated user IDs | none (open) |
| `YT_DLP_PATH` | Path to yt-dlp binary | `yt-dlp` |
| `DOWNLOAD_DIR` | Temp download directory | `./downloads` |
| `MAX_FILE_SIZE_MB` | Max file size for Telegram send | 50 |
| `RATE_LIMIT_PER_USER` | Requests per hour per user | 10 |
| `COOKIES_FROM_BROWSER` | Browser name for cookie extraction (e.g. `chrome`) | — |
| `COOKIES_FILE` | Path to Netscape-format cookies file | — |
| `FILE_RETENTION_MINUTES` | How long to keep downloaded files | 60 |
| `CONFIG_PATH` | Custom YAML config file path | `./configs/default.yaml` |
| `SHARE_HOST` | Public base URL for share server | auto-detect local IP |
| `SHARE_PORT` | Share server listener port | 8080 |
| `SHARE_TIMEOUT_MINUTES` | Share server auto-shutdown timeout | 30 |

## Security Hardening

The following security measures have been implemented:

1. **Slowloris Protection:** HTTP share server sets `ReadHeaderTimeout` (30s) to prevent slow-header DoS attacks.
2. **yt-dlp Injection Prevention:** URLs starting with `--` are rejected before being passed to yt-dlp, preventing option injection.
3. **Content-Disposition Header Injection:** Filenames are escaped (quotes replaced with `\"`) in `Content-Disposition` headers to prevent HTTP header injection.
4. **Directory Permissions:** Download directories created with `0700` permissions (owner-only access) instead of the default `0755`.
5. **CONFIG_PATH Sanitization:** Config file path is validated with `filepath.Clean` and `..` component rejection to prevent directory traversal.
6. **Path Traversal Protection:** Share server uses `filepath.Base()` on filenames and URL-encodes route patterns.
7. **Error Handling:** All file operations (`file.Close()`, `os.Remove()`) have their errors logged.

## API / Command Reference

| Command | Description |
|---|---|
| `/start` | Welcome message |
| `/help` | Usage instructions (including cookie auth info) |
| `<YouTube URL>` | List available formats (720p+) |
| `<format number>` | Download & deliver selected format |
| Any other text | "Send a YouTube URL to get started" |

## Known Limitations

- Cookie auth requires yt-dlp ≥ 2023.x and a supported browser installed on the host.
- Share server binds to 0.0.0.0 — must be behind a firewall or use `SHARE_HOST` to restrict access.
- No playlist support (single video only).
- No subtitle download.
