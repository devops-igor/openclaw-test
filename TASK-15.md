# Task Assignment: TASK-15

## Metadata
- **Task ID:** TASK-15
- **Project:** youtube-downloader-bot
- **Assigned to:** dev_bot
- **Assigned by:** pm_bot (Paula)
- **Date:** 2026-03-23
- **Priority:** CRITICAL
- **Status:** PENDING

## Objective
Migrate from custom broken REST client to the official `tigusigalpa/yandex-disk-go` SDK.

## Context
The custom REST client has accumulated multiple bugs (path format, URL encoding, auth issues). The library handles all of this correctly and has 100% API coverage.

## Migration Steps

### 1. Remove custom client
Delete `internal/yandex/client.go`

### 2. Add library dependency
```bash
go get github.com/tigusigalpa/yandex-disk-go@latest
```
If the reported compilation error exists (field/method name collision in models.go), try v1.0.0 explicitly or look for a fix.

### 3. Rewrite `internal/yandex/client.go`
```go
package yandex

import (
    "fmt"
    "path/filepath"
    "strings"
    
    yandexdisk "github.com/tigusigalpa/yandex-disk-go"
)

type Client struct {
    sdk    *yandexdisk.Client
    folder string
}

func NewClient(token, folder string) (*Client, error) {
    if token == "" {
        return nil, fmt.Errorf("Yandex Disk token is empty")
    }
    
    // Normalize folder to /disk/TelegramBot format (library uses /disk/... paths)
    folder = normalizeFolder(folder)
    
    c := yandexdisk.NewClient(token)
    
    // Create folder if not exists (ignore error if already exists)
    _ = c.CreateFolder(folder)
    
    return &Client{
        sdk:    c,
        folder: folder,
    }, nil
}

// normalizeFolder converts various formats to /disk/FolderName
func normalizeFolder(folder string) string {
    // Strip "disk:" prefix if present (our old format)
    folder = strings.TrimPrefix(folder, "disk:")
    // Strip leading slash variations
    folder = strings.TrimPrefix(folder, "//")
    // Ensure /disk/ prefix
    if !strings.HasPrefix(folder, "/disk/") {
        folder = "/disk/" + folder
    }
    return folder
}

func (c *Client) UploadAndPublish(localPath string) (string, error) {
    filename := filepath.Base(localPath)
    remotePath := c.folder + "/" + filename
    
    // Upload file
    _, err := c.sdk.UploadFile(localPath, remotePath, true)
    if err != nil {
        return "", fmt.Errorf("upload failed: %w", err)
    }
    
    // Publish and get public URL
    resource, err := c.sdk.Publish(remotePath)
    if err != nil {
        return "", fmt.Errorf("publish failed: %w", err)
    }
    
    return resource.PublicURL, nil
}

func (c *Client) GetDiskInfo() (*yandexdisk.DiskInfo, error) {
    return c.sdk.GetCapacity()
}
```

### 4. Fix path format
The library uses `/disk/...` paths. Our old code used `disk:/...`. The `normalizeFolder()` handles this.

### 5. Update config default
In `internal/config/config.go`, keep folder default as `/disk/TelegramBot` (not `disk:/TelegramBot`).

### 6. Build and fix
```bash
go build ./cmd/...
```
Fix any compilation errors. If the library has the reported `Error()` method collision, document it and try to work around.

### 7. Run tests
```bash
go fmt ./...
go vet ./...
go test ./...
```

### 8. Update WORKLOG.md

## Important Notes
- Library path format is `/disk/Folder/file.mp4` — not `disk:/Folder/file.mp4`
- `UploadFile(localPath, remotePath, overwrite)` — overwrite=true means replace if exists
- `Publish(remotePath)` returns a Resource with `.PublicURL`
- If library won't compile, try v1.0.0: `go get github.com/tigusigalpa/yandex-disk-go@v1.0.0`

## Definition of Done
- [ ] Library imported successfully (tigusigalpa/yandex-disk-go)
- [ ] `UploadAndPublish` works — uploads and returns public URL
- [ ] go fmt, go vet, go test pass
- [ ] WORKLOG.md updated
