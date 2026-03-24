# SPEC.md - Project Specification

## Project Overview

**Name:** youtube-downloader-bot  
**Tagline:** A Telegram bot that downloads YouTube videos using yt-dlp  
**Type:** CLI-based bot application (long-running process)  
**Target Platform:** Linux/Windows/macOS (any system with yt-dlp and Go)

## Problem Statement

Users want a simple way to download YouTube videos from their mobile devices or any chat interface. They need quality selection, format options, and reliable downloads without manually handling yt-dlp commands.

## Solution Summary

A Telegram bot that:
1. Receives YouTube URLs from users via Telegram
2. Validates URLs are from YouTube
3. Uses yt-dlp CLI on the host machine to fetch available formats/qualities
4. Lets user select quality/format
5. Downloads video to configured directory
6. Sends file back to user (or provides download link if too large)
7. Optionally keeps local archive or auto-cleans

## Core Features

### MVP (Must-Have)
- [ ] Telegram bot setup with token authentication
- [ ] `/start` command with usage instructions
- [ ] URL validation (YouTube only)
- [ ] Fetch available formats using `yt-dlp -F`
- [ ] Interactive quality selection (inline buttons or numbered options)
- [ ] Download video using `yt-dlp -f <format>`
- [ ] Save to local directory (configurable)
- [ ] Send downloaded file back to user (Telegram sendDocument)
- [ ] Basic error handling (invalid URL, download failures, etc.)
- [ ] Logging of all actions

### Stretch Goals (Should-Have)
- [ ] Progress updates during download (send "Downloading..." messages)
- [ ] Format/quality presets (best, worst, audio-only, video-only)
- [ ] Download history/log for user
- [ ] Rate limiting per user (prevent abuse)
- [ ] Admin commands (status, logs, maintenance)
- [ ] Configuration via environment variables or config file
- [ ] Download directory auto-organization (by date, user, etc.)

### Future Enhancements (Nice-to-Have)
- [ ] Web interface for monitoring
- [ ] Multi-host distribution (different yt-dlp instances)
- [ ] Queue system for concurrent downloads
- [ ] Notification system (email, push)
- [ ] Download scheduling
- [ ] Analytics (download counts, popular videos, errors)
- [ ] Plugins for other video platforms (beyond YouTube)

## Technical Specifications

### Dependencies
- **Go:** 1.24+
- **yt-dlp:** Must be installed on host system and in PATH
- **Telegram Bot Token:** From BotFather
- **Disk Space:** Adequate for video storage

### Project Structure
```
youtube-downloader-bot/
├── cmd/bot/
│   └── main.go              # Application entry point
├── internal/
│   ├── bot/                 # Telegram bot logic
│   │   ├── handler.go       # Command/message handlers
│   │   ├── keyboard.go      # Inline keyboard layouts
│   │   └── middleware.go    # Auth, logging, rate limiting
│   ├── downloader/          # yt-dlp integration
│   │   ├── executor.go      # Execute yt-dlp commands
│   │   ├── parser.go        # Parse yt-dlp output (formats, progress)
│   │   └── manager.go       # Download queue and lifecycle
│   ├── config/              # Configuration management
│   │   └── config.go        # Load env/config file
│   └── storage/             # File management
│       ├── saver.go         # Save downloads to disk
│       └── cleaner.go       # Cleanup old files
├── pkg/
│   └── types/               # Shared types (format, user, download)
│       └── models.go
├── configs/
│   └── default.yaml        # Default configuration template
├── deployments/
│   └── docker-compose.yml  # Optional containerization
├── Makefile                # Build, test, run targets
├── go.mod                  # Go modules
├── go.sum
├── README.md               # Documentation
└── WORKLOG.md              # Append-only action log
```

### Configuration

**Environment Variables:**
- `TELEGRAM_BOT_TOKEN` (required)
- `ALLOWED_USERS` (comma-separated Telegram user IDs, e.g., `"12345,67890"`) — **required for production, empty means no access**
- `YT_DLP_PATH` (default: `yt-dlp` if in PATH)
- `DOWNLOAD_DIR` (default: `./downloads`)
- `MAX_FILE_SIZE_MB` (default: 50, Telegram limit is 50MB for bots)
- `RATE_LIMIT_PER_USER` (default: 10 downloads/hour)
- `FILE_RETENTION_MINUTES` (default: 60) — auto-delete downloads after 1 hour
- `LOG_LEVEL` (default: info)

**config.yaml (optional alternative to env vars):**
```yaml
telegram:
  token: "YOUR_BOT_TOKEN"
  allowed_users: []  # list of Telegram user IDs; empty = no access (must populate for production)

downloader:
  yt_dlp_path: "yt-dlp"
  download_dir: "./downloads"
  max_file_size_mb: 50
  rate_limit_per_user: 10

storage:
  file_retention_minutes: 60  # auto-cleanup after 1 hour
  max_total_size_gb: 100
```

### yt-dlp Integration

**Commands used:**
1. List formats: `yt-dlp -F <URL>`
2. Download: `yt-dlp -f <format_id> -o "<output_path>" <URL>`
3. Get video info: `yt-dlp --skip-download --print-json <URL>`

**Output parsing:**
- Parse `-F` output to extract format IDs, resolution, filesize, codec
- Present options to user via inline keyboard or numbered list
- Use `-o` with template like `%(title)s.%(ext)s` for filename

**Progress monitoring:**
- yt-dlp writes progress to stderr with percentages
- Parse stderr for real-time progress updates to user

### Telegram Bot API

**Commands:**
- `/start` — Welcome message with usage instructions
- `/help` — Detailed help
- `/settings` — User-specific settings (optional)
- `/status` — Current download status for user
- `/cancel` — Cancel ongoing download

**Message handlers:**
- URL message (any text containing YouTube URL) → Trigger format fetch
- Inline callback queries (format selection) → Start download
- Document uploads (not needed, bot only sends)

**Conversation flow:**
1. User sends YouTube URL
2. Bot acknowledges and calls `yt-dlp -F <URL>`
3. Bot presents format options (e.g., "1) 720p MP4, 2) 1080p MP4, 3) Audio only")
4. User selects option
5. Bot starts download with `yt-dlp -f <id>`
6. Bot periodically sends progress updates (e.g., "Downloading... 45%")
7. On completion, bot sends file to user
8. Optionally, auto-cleanup local file after transmission

### Error Handling

- Invalid/unsupported URL → "That doesn't look like a YouTube URL"
- yt-dlp not found → "Downloader not configured, contact admin"
- Download fails → Retry up to 3 times, then report error
- File too large → "Video exceeds Telegram's 50MB limit, try lower quality"
- Rate limit exceeded → "Slow down! You can download {N} videos per hour"
- yt-dlp returns error → Forward error message (sanitized) to user

### Security Considerations

- Validate all URLs are from youtube.com/youtu.be
- Sanitize filenames to prevent path traversal
- Run yt-dlp with restricted permissions (if possible)
- Rate limit per user to prevent DoS
- Optional whitelist of allowed users (configurable)
- Cleanup temporary files on error
- Log all user actions for audit

### Testing Strategy

**Unit Tests:**
- URL validation function
- yt-dlp output parser (format extraction)
- Progress parser (percentage extraction)
- Configuration loading
- Filename sanitization

**Integration Tests:**
- Mock yt-dlp responses (no actual downloads)
- Bot command handlers with fake Telegram updates
- End-to-end workflow (URL → format → download → send)

**Manual Testing:**
- Real Telegram bot with test token
- Real yt-dlp with small test videos
- Verify file transmission, progress updates

## Acceptance Criteria

The project is considered complete when:

1. ✅ Bot responds to `/start` and `/help`
2. ✅ Accepts YouTube URL and presents format options
3. ✅ Downloads video using selected format
4. ✅ Sends downloaded file back to Telegram user
5. ✅ Handles errors gracefully (invalid URL, download failure, large file)
6. ✅ Enforces rate limits per user
7. ✅ Enforces user whitelist (only allowed users can use the bot)
8. ✅ Auto-deletes downloaded files after 1 hour (configurable)
9. ✅ No playlist support (single video URLs only)
10. ✅ All unit and integration tests pass
11. ✅ Code follows GOLANG_STANDARDS.md
12. ✅ Proper logging throughout
13. ✅ README with setup and usage instructions
14. ✅ WORKLOG.md contains complete development history
15. ✅ QA review passed with no critical security issues

## Out of Scope

- Supporting video platforms other than YouTube (future enhancement)
- Web dashboard or admin panel
- Database persistence (config file and file system only)
- OAuth or user accounts (Telegram user ID is identifier)
- Video transcoding (rely on yt-dlp's format selection)
- Distributed downloads (single host only)
- **Playlist support** — only single video URLs

## Success Metrics

- **Functionality:** All acceptance criteria met
- **Reliability:** < 5% download failure rate (excluding YouTube-side issues)
- **Performance:** Download-to-receive time within 30s of yt-dlp alone
- **Usability:** User can complete download in < 5 interactions
- **Code Quality:** Test coverage ≥80%, no critical QA issues

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| yt-dlp not installed on host | High | Check at startup, clear error message |
| Telegram file size limits (50MB) | Medium | Filter formats by size, warn user |
| YouTube rate limits | Medium | Implement retry with backoff, respect yt-dlp errors |
| Disk space exhaustion | High | Quotas, auto-cleanup, monitor free space |
| Bot token leakage | Critical | Never commit token, use env vars, .gitignore |
| Abusive users (spam) | Medium | Rate limiting, optional whitelist |

## Open Questions

1. Should we support a webhook mode or long-polling? (Recommend: start with long-polling, add webhook later if needed)
2. Should we have an admin interface for monitoring? (Maybe simple `/admin` commands — currently not planned, but could add if requested)
3. Deployment environment specifics: Any OS-specific considerations for yt-dlp? (Windows/Linux/macOS all supported)

---

## Decisions Made (Human Feedback)

- ✅ **User whitelist:** Only whitelisted users can use the bot (config `allowed_users` required for production)
- ✅ **Auto-delete:** Downloads auto-deleted after 1 hour (configurable via `FILE_RETENTION_MINUTES`)
- ✅ **Playlist support:** Not included (out of scope — single videos only)

---

**Spec Owner:** pm_bot  
**Date:** 2026-03-21  
**Status:** ✅ Approved (human) — 2026-03-21 03:03 GMT+3
