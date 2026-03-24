# Task Assignment: TASK-12

## Metadata
- **Task ID:** TASK-12
- **Project:** youtube-downloader-bot
- **Assigned to:** dev_bot
- **Assigned by:** pm_bot (Paula)
- **Date:** 2026-03-23
- **Priority:** CRITICAL
- **Status:** PENDING

## Bug Description
When a YouTube video is larger than 50MB, yt-dlp fails BEFORE the bot can upload to Yandex Disk. The error is:
```
"downloaded file size 426020299 bytes exceeds maximum allowed size 52428800 bytes"
```

This happens because the executor has a `maxSizeBytes` check that fails during download — so the Yandex upload path in `downloadAndSend()` is never reached.

## Root Cause
In `internal/downloader/executor.go`, the `Run()` method has a `maxSizeBytes` parameter that yt-dlp uses to reject files over that limit. The bot passes 50MB here, so yt-dlp refuses to download the file at all for large videos.

## Fix

### Fix 1: Remove the executor size check
In `internal/downloader/executor.go`, the `maxSizeBytes` is passed to yt-dlp as `--max-filesize`. We should NOT limit yt-dlp's download — we only limit what Telegram can send. Remove the `--max-filesize` flag from the yt-dlp command, or set it to 0 (no limit).

The size check should ONLY happen in the bot's `downloadAndSend()` after the file is downloaded — that's where we decide to send via Telegram or upload to Yandex.

### Fix 2: Ensure Yandex path is reached
In `internal/bot/bot.go`, `downloadAndSend()` should:
1. Download without size limit
2. Check file size AFTER download completes
3. If > 50MB and Yandex configured → upload to Yandex
4. If > 50MB and Yandex NOT configured → send "file too large" error
5. If ≤ 50MB → send via Telegram (existing path)

## Implementation

### executor.go
Change the yt-dlp args — remove or increase `--max-filesize`:

```go
// OLD (problematic):
args := []string{
    "--no-playlist",
    "--quiet",
    "--no-warnings",
    "--max-filesize", strconv.FormatInt(maxSizeBytes, 10),
    "-f", formatID,
    "-o", tempDir + "/%(title)s.%(ext)s",
    url,
}

// NEW (no limit at download time):
args := []string{
    "--no-playlist",
    "--quiet",
    "--no-warnings",
    // REMOVED: "--max-filesize" — we check size AFTER download
    "-f", formatID,
    "-o", tempDir + "/%(title)s.%(ext)s",
    url,
}
```

Or if you want to keep maxSizeBytes for safety, set it to 0 or a very large number when calling from bot.go.

### bot.go — downloadAndSend()
Make sure the size check is AFTER download and properly routes to Yandex:

```go
func (b *Bot) downloadAndSend(chatID int64, videoURL string, formatID string) {
    // Download without size limit
    result, err := b.ytDlp.Download(videoURL, formatID, 0) // pass 0 = no limit
    if err != nil {
        // Check if it's a size error
        if strings.Contains(err.Error(), "exceeds maximum allowed size") {
            // This shouldn't happen now, but handle it
            _ = b.SendMessage(chatID, "❌ File too large for Telegram. Consider using Yandex Disk.")
            return
        }
        _ = b.SendMessage(chatID, "❌ Download failed: "+err.Error())
        return
    }
    defer os.Remove(result.Filepath)
    
    // Check size AFTER download
    fileInfo, err := os.Stat(result.Filepath)
    if err != nil {
        _ = b.SendMessage(chatID, "❌ Could not check file size")
        return
    }
    
    const maxDirectSendSize = 50 * 1024 * 1024 // 50MB
    
    if fileInfo.Size() > maxDirectSendSize && b.yandexClient != nil {
        // Upload to Yandex Disk
        b.log.Info("Uploading large file to Yandex Disk",
            "size_mb", float64(fileInfo.Size())/1e6)
        
        shareURL, err := b.ytDlp.UploadToYandexDisk(result.Filepath, b.yandexClient)
        if err != nil {
            _ = b.SendMessage(chatID, "❌ Upload to cloud failed: "+err.Error())
            return
        }
        
        _ = b.SendMessage(chatID, fmt.Sprintf(
            "📤 Video uploaded to cloud (%.1f MB)\n\n🔗 %s\n\n📥 Download from the link above",
            float64(fileInfo.Size())/1e6, shareURL))
        return
    }
    
    if fileInfo.Size() > maxDirectSendSize && b.yandexClient == nil {
        _ = b.SendMessage(chatID, "❌ File too large to send directly (%.1f MB). Set YANDEX_DISK_TOKEN to enable cloud uploads.",
            float64(fileInfo.Size())/1e6)
        return
    }
    
    // Send via Telegram (≤50MB)
    // ... existing code ...
}
```

### Fix 3: Verify manager.go Download signature
If `Download()` signature takes `maxSizeBytes int64`, make sure bot.go passes `0` (no limit) and the executor handles 0 as "unlimited".

## Requirements

### Must Have
1. yt-dlp downloads videos regardless of size (no --max-filesize limit at download time)
2. Large files (>50MB) are uploaded to Yandex Disk when configured
3. When Yandex is not configured, user gets clear error message with instructions
4. Files ≤ 50MB sent via Telegram (unchanged)

## Implementation Steps
1. Read `internal/downloader/executor.go` — find `--max-filesize` usage
2. Read `internal/bot/bot.go` — find `downloadAndSend()` and the size check
3. Remove `--max-filesize` from executor args (or set to 0 in bot)
4. Verify `downloadAndSend()` properly routes large files to Yandex
5. Run `go build`, `go fmt`, `go vet`, `go test`
6. Update WORKLOG.md

## Definition of Done
- [ ] yt-dlp downloads videos of any size
- [ ] Files > 50MB with Yandex configured → share link sent
- [ ] Files > 50MB without Yandex → clear error message
- [ ] Files ≤ 50MB → sent via Telegram unchanged
- [ ] go fmt, go vet, go test pass
