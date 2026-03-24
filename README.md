# YouTube Downloader Bot

A Telegram bot for downloading YouTube videos and audio with quality selection, built in Go.

# Required configuration
# See Configuration Reference section below

## Features

- 🎬 Download YouTube videos in any available quality (up to 4K)
- 🎵 Audio-only downloads (Opus, MP4A)
- 🔄 Automatic retry with exponential backoff on network errors
- 💾 Disk space pre-check before downloading (500MB minimum)
- 🔒 User whitelist — only approved Telegram users can access
- ⏱️ Rate limiting per user
- 🧹 Automatic cleanup of old downloaded files
- 📊 Structured logging for debugging and monitoring

## Prerequisites

- **Go 1.24+** — [install Go](https://go.dev/dl/)
- **yt-dlp** — must be in PATH ([install yt-dlp](https://github.com/yt-dlp/yt-dlp#installation))
- **Telegram Bot Token** — create via [@BotFather](https://t.me/BotFather)

## Installation

### Build from Source

```bash
git clone https://github.com/igorkon/youtube-downloader-bot.git
cd youtube-downloader-bot
go build -o youtube-downloader-bot ./cmd/bot/
```

### Configuration

The bot is configured via environment variables. Create a `.env` file or export them directly:

```bash
# Required
export TELEGRAM_BOT_TOKEN="your-bot-token-here"

# Optional (shown with defaults)
export ALLOWED_USERS="123456789,987654321"  # Comma-separated Telegram user IDs
export YT_DLP_PATH="yt-dlp"                # Path to yt-dlp binary
export DOWNLOAD_DIR="./downloads"           # Download directory
export MAX_FILE_SIZE_MB="50"                # Max file size in MB
export RATE_LIMIT_PER_USER="10"             # Requests per user per hour
export FILE_RETENTION_MINUTES="60"          # Auto-cleanup after N minutes
```

### Configuration Reference

| Variable | Default | Description |
|---|---|---|
| `TELEGRAM_BOT_TOKEN` | *(required)* | Telegram bot API token from BotFather |
| `ALLOWED_USERS` | *(empty = all allowed)* | Comma-separated Telegram user IDs (whitelist) |
| `YT_DLP_PATH` | `yt-dlp` | Path to the yt-dlp binary |
| `DOWNLOAD_DIR` | `./downloads` | Directory for downloaded files |
| `MAX_FILE_SIZE_MB` | `50` | Maximum file size allowed for download |
| `RATE_LIMIT_PER_USER` | `10` | Max requests per user per hour |
| `FILE_RETENTION_MINUTES` | `60` | How long to keep downloaded files before cleanup |

## Usage

1. **Start the bot:**
   ```bash
   ./youtube-downloader-bot
   ```

2. **Open Telegram and message your bot:**

3. **Send a YouTube URL** — the bot will fetch available formats:
   ```
   https://www.youtube.com/watch?v=dQw4w9WgXcQ
   ```

4. **Select a quality** — click one of the quality buttons:
   - `1080p · MP4 · 25.3MiB`
   - `720p · MP4 · 12.1MiB`
   - `audio · Opus · 3.2MiB`

5. **Receive the file** — the bot will download and send the file to you.

### What happens under the hood:
```
User sends URL → Bot fetches formats → User selects quality → Download → Send file → Auto-cleanup
```

## Rate Limits

Each user is limited to a configurable number of requests per hour (default: 10). This prevents abuse and ensures fair usage. The rate limit is tracked in-memory and resets each hour.

## File Retention

Downloaded files are automatically cleaned up after `FILE_RETENTION_MINUTES` (default: 60 minutes). This keeps your disk usage under control. Files are also cleaned up by a background goroutine that runs periodically.

## Security

- **User Whitelist:** Only Telegram users whose IDs are in `ALLOWED_USERS` can use the bot. If `ALLOWED_USERS` is empty, all users are allowed.
- **URL Validation:** Only YouTube URLs are accepted; the bot validates URLs before processing.
- **File Size Limits:** Downloads are capped at `MAX_FILE_SIZE_MB` to prevent disk exhaustion.
- **Disk Space Check:** The bot verifies at least 500MB of free space before starting a download.
- **Auto-Cleanup:** Downloaded files are automatically deleted after the retention period.
- **No Public Exposure:** The bot only communicates through Telegram's API; no web server is exposed.

## Architecture

See [ARCHITECTURE.md](./ARCHITECTURE.md) for detailed component and data flow documentation.

## Project Structure

```
youtube-downloader-bot/
├── cmd/bot/              # Application entry point
├── internal/
│   ├── bot/              # Telegram bot handlers
│   ├── config/           # Configuration loading
│   └── downloader/       # yt-dlp execution, format parsing, retry logic
├── pkg/
│   ├── logger/           # Structured logging wrapper
│   └── types/            # Shared types
├── configs/
│   └── default.yaml      # Default configuration
└── README.md
```

## Troubleshooting

### Bot doesn't start
- Check that `TELEGRAM_BOT_TOKEN` is set correctly
- Verify the token works: `curl https://api.telegram.org/bot<token>/getMe`

### "yt-dlp binary not found"
- Install yt-dlp: `pip install yt-dlp`
- Verify installation: `yt-dlp --version`
- If installed in a non-standard path, set `YT_DLP_PATH`

### "Network error during yt-dlp execution"
- Check your internet connection
- The bot will retry automatically (up to 3 times with exponential backoff)
- YouTube may be temporarily blocking requests; try again later

### "Insufficient disk space"
- Free up space on the disk containing `DOWNLOAD_DIR`
- The bot requires at least 500MB of free space to start a download

### "Rate limit exceeded"
- Wait for the rate limit window to reset (1 hour)
- Adjust `RATE_LIMIT_PER_USER` if needed for your use case

### Video is too large
- Select a lower quality option
- Increase `MAX_FILE_SIZE_MB` in configuration

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

### Development

```bash
# Run tests
go test ./...

# Run with verbose logging
export LOG_LEVEL=debug
go run ./cmd/bot/

# Build
go build -o youtube-downloader-bot ./cmd/bot/
```

## License

MIT License. See [LICENSE](./LICENSE) for details.
