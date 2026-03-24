# Task Assignment: TASK-14

## Metadata
- **Task ID:** TASK-14
- **Project:** youtube-downloader-bot
- **Assigned to:** dev_bot
- **Assigned by:** pm_bot (Paula)
- **Date:** 2026-03-23
- **Priority:** CRITICAL
- **Status:** PENDING

## Issue
Yandex Disk upload fails with HTTP 400 when the video filename contains special characters:
```
"650hp and a KILLER Nürburgring Setup Audi RS3 8V-4tkDg5_IfD4.mp4"
```
Error: `get upload URL: HTTP 400 (empty response body)`

Yandex Disk API rejects filenames with:
- Spaces
- Non-ASCII characters (ü, etc.)
- Special punctuation

## Fix Required

### Sanitize filenames before Yandex upload

In `internal/yandex/client.go`, before calling `UploadAndPublish()` (or inside it), normalize the filename:

```go
import (
    "regexp"
    "strings"
)

// sanitizeFilename removes unsafe characters for Yandex Disk
func sanitizeFilename(name string) string {
    // Remove file extension first
    ext := filepath.Ext(name)
    base := strings.TrimSuffix(name, ext)
    
    // Convert to lowercase
    base = strings.ToLower(base)
    
    // Replace spaces and non-alphanumeric with underscores
    // Keep: a-z, 0-9, dash, underscore
    reg := regexp.MustCompile(`[^a-z0-9_-]+`)
    base = reg.ReplaceAllString(base, "_")
    
    // Remove multiple consecutive underscores
    base = regexp.MustCompile(`_+`).ReplaceAllString(base, "_")
    
    // Trim leading/trailing underscores
    base = strings.Trim(base, "_")
    
    // Ensure we have something
    if base == "" {
        base = "video"
    }
    
    return base + ext
}
```

Use in `UploadAndPublish()`:
```go
func (c *Client) UploadAndPublish(localPath string) (string, error) {
    originalFilename := filepath.Base(localPath)
    safeFilename := sanitizeFilename(originalFilename)
    remotePath := c.folder + "/" + safeFilename
    
    c.log.Info("Uploading to Yandex Disk",
        "original", originalFilename,
        "sanitized", safeFilename,
        "remote", remotePath)
    
    // ... rest of upload logic
}
```

**Also log when sanitization happens** so we know if a filename was changed.

### Alternative: URL-encode the filename
Instead of replacing, we could URL-encode spaces and Unicode:
```go
safeFilename := url.PathEscape(originalFilename)
```
But Yandex might expect the path to be UTF-8 encoded in the URL. Need to test.

### Recommendation
Start with the **sanitization approach** (alphanumeric + dash/underscore only). This is safe and predictable.

## Implementation Steps

1. Read `internal/yandex/client.go` — find where filename is used
2. Add `sanitizeFilename()` helper
3. Apply sanitization to `remotePath` filename
4. Add logging: "Filename sanitized from X to Y"
5. Run `go fmt`, `go vet`, `go test`
6. Update WORKLOG.md

## Testing
After implementing, try uploading a file with special characters again. The sanitized filename should succeed.

**Example:**
- Input: `"650hp and a KILLER Nürburgring Setup Audi RS3 8V-4tkDg5_IfD4.mp4"`
- Output: `"650hp_and_a_killer_n_rburgring_setup_audi_rs3_8v-4tkdg5_ifd4.mp4"` (or similar)

## Definition of Done
- [ ] Filenames sanitized before Yandex upload
- [ ] Logging shows original and sanitized names
- [ ] go fmt, go vet, go test pass
- [ ] WORKLOG.md updated
