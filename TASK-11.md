# Task Assignment: TASK-11

## Metadata
- **Task ID:** TASK-11
- **Project:** youtube-downloader-bot
- **Assigned to:** dev_bot
- **Assigned by:** pm_bot (Paula)
- **Date:** 2026-03-23
- **Priority:** HIGH
- **Status:** PENDING

## Objective
Add Yandex Disk upload support so videos > 50MB can be sent as share links instead of being rejected.

## Architecture

**Flow:**
1. User sends YouTube URL
2. Bot downloads video via yt-dlp
3. If file > 50MB → upload to Yandex Disk → publish → send share link to user
4. If file ≤ 50MB → send directly via Telegram (existing behavior)

**New package:** `internal/yandex/`
**Library:** `github.com/tigusigalpa/yandex-disk-go`

## Changes to Make

### 1. Add Yandex Disk config
In `internal/config/config.go`, add:

```go
type YandexDiskConfig struct {
    Token string `env:"YANDEX_DISK_TOKEN,required"`
    // Folder path on Yandex Disk where uploads go
    Folder string `env:"YANDEX_DISK_FOLDER" envDefault:"/disk/TelegramBot"`
}
```

And add to the main config:
```go
YandexDisk YandexDiskConfig
```

### 2. Create `internal/yandex/client.go`
Implement a Yandex Disk client wrapper:

```go
package yandex

import (
    "fmt"
    "io"
    "os"
    "path/filepath"
    
    yandexdisk "github.com/tigusigalpa/yandex-disk-go"
)

type Client struct {
    client *yandexdisk.Client
    folder string
}

func NewClient(token, folder string) (*Client, error) {
    client := yandexdisk.NewClient(token)
    
    // Create folder if not exists
    err := client.CreateFolder(folder)
    if err != nil {
        // Folder might already exist, that's fine
    }
    
    return &Client{
        client: client,
        folder: folder,
    }, nil
}

// UploadAndPublish uploads a local file to Yandex Disk and publishes it.
// Returns the public share URL.
func (c *Client) UploadAndPublish(localPath string) (string, error) {
    filename := filepath.Base(localPath)
    remotePath := c.folder + "/" + filename
    
    // Upload file (overwrite if exists)
    _, err := c.client.UploadFile(localPath, remotePath, true)
    if err != nil {
        return "", fmt.Errorf("upload failed: %w", err)
    }
    
    // Publish and get public URL
    resource, err := c.client.Publish(remotePath)
    if err != nil {
        return "", fmt.Errorf("publish failed: %w", err)
    }
    
    return resource.PublicURL, nil
}

// UploadAndPublishReader uploads from a reader (for streaming)
func (c *Client) UploadAndPublishReader(reader io.Reader, filename string) (string, error) {
    // Create temp file
    tempFile, err := os.CreateTemp("", "yandex-upload-*")
    if err != nil {
        return "", fmt.Errorf("temp file failed: %w", err)
    }
    defer os.Remove(tempFile.Name())
    defer tempFile.Close()
    
    // Copy reader to temp file
    _, err = io.Copy(tempFile, reader)
    if err != nil {
        return "", fmt.Errorf("copy to temp failed: %w", err)
    }
    tempFile.Close()
    
    return c.UploadAndPublish(tempFile.Name())
}

// GetDiskInfo returns disk usage info
func (c *Client) GetDiskInfo() (*yandexdisk.DiskInfo, error) {
    return c.client.GetCapacity()
}
```

### 3. Update `internal/downloader/manager.go`
Add `UploadToYandexDisk` method:

```go
func (m *Manager) UploadToYandexDisk(filePath string, yandexClient *yandex.Client) (string, error) {
    m.log.Info("Uploading to Yandex Disk", "file", filePath)
    
    publicURL, err := yandexClient.UploadAndPublish(filePath)
    if err != nil {
        return "", err
    }
    
    m.log.Info("Uploaded to Yandex Disk", "url", publicURL)
    return publicURL, nil
}
```

### 4. Update `internal/bot/bot.go`
In `handleDownload()`, after getting the file from yt-dlp:

```go
// Check file size
fileInfo, err := os.Stat(result.Filepath)
if err != nil {
    // handle error
}

const maxDirectSendSize = 50 * 1024 * 1024 // 50MB

if fileInfo.Size() > maxDirectSendSize && b.yandexClient != nil {
    // Upload to Yandex Disk
    b.log.Info("File too large for Telegram, uploading to Yandex Disk",
        "size", fileInfo.Size(), "limit", maxDirectSendSize)
    
    shareURL, err := b.downloader.UploadToYandexDisk(result.Filepath, b.yandexClient)
    if err != nil {
        _ = b.SendMessage(chatID, "❌ Upload to cloud failed: "+err.Error())
        return
    }
    
    // Send share link with video info
    msg := fmt.Sprintf("📤 Video uploaded to cloud (%.1f MB)\n\n🔗 %s\n\n📥 Download from the link above", 
        float64(fileInfo.Size())/1e6, shareURL)
    _ = b.SendMessage(chatID, msg)
    return
}

// Existing Telegram send logic for small files...
```

### 5. Update bot initialization in `cmd/bot/main.go`
Initialize Yandex client:

```go
var yandexClient *yandex.Client
if cfg.YandexDisk.Token != "" {
    yandexClient, err = yandex.NewClient(cfg.YandexDisk.Token, cfg.YandexDisk.Folder)
    if err != nil {
        log.Printf("Yandex Disk init failed: %v", err)
    } else {
        log.Printf("Yandex Disk connected: %s", cfg.YandexDisk.Folder)
    }
}
```

Pass it to the bot:
```go
bot, err := bot.New(cfg, ytDlp, yandexClient)
```

### 6. Update `internal/bot/bot.go` struct and New()

Add to `Bot` struct:
```go
yandexClient *yandex.Client
```

Update `New()` to accept it.

## Requirements

### Must Have
1. Files > 50MB uploaded to Yandex Disk, published, link sent to user
2. Yandex Disk token from env var (never hardcoded)
3. Graceful fallback: if Yandex fails, report error to user
4. Existing Telegram send for files ≤ 50MB unchanged

### Should Have
- Log Yandex Disk upload progress
- Handle duplicate filenames gracefully (overwrite)
- Create upload folder if not exists

## Implementation Steps
1. Add Yandex Disk config
2. Create `internal/yandex/client.go`
3. Add `UploadToYandexDisk` to manager
4. Update bot initialization
5. Update `handleDownload` flow
6. Run `go get github.com/tigusigalpa/yandex-disk-go`
7. Run `go fmt`, `go vet`, `go test`
8. Update WORKLOG.md

## Definition of Done
- [ ] Files > 50MB uploaded to Yandex Disk and share link sent
- [ ] Files ≤ 50MB sent directly via Telegram (unchanged)
- [ ] Yandex token from env var only
- [ ] go fmt, go vet, go test pass
- [ ] WORKLOG.md updated
