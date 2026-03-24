# Task Assignment: TASK-13

## Metadata
- **Task ID:** TASK-13
- **Project:** youtube-downloader-bot
- **Assigned to:** dev_bot
- **Assigned by:** pm_bot (Paula)
- **Date:** 2026-03-23
- **Priority:** HIGH
- **Status:** PENDING

## Issue
Yandex Disk upload failing with `HTTP 400` on "get upload URL" step. Error details are vague. Need better diagnostics and token refresh handling.

## What to Investigate

### 1. Check token validity
- Has the token expired? OAuth tokens can expire.
- Does the token have `disk.write` permission?

### 2. Check folder creation
- The code tries to create `/disk/TelegramBot` on init. Is the folder actually created? Maybe the API call failed but we ignored the error.

### 3. Improve error logging
The Yandex client should log:
- The exact remote path being uploaded
- The HTTP response body from Yandex (400 responses often have JSON with error details)
- The operation ID if it's an async operation

### 4. Handle 401/403 vs 400
- 401/403 → token issue, tell user to re-auth
- 400 → bad request, maybe invalid path

## Implementation

### Update `internal/yandex/client.go`

Add detailed error logging and response capture:

```go
func (c *Client) UploadAndPublish(localPath string) (string, error) {
    filename := filepath.Base(localPath)
    remotePath := c.folder + "/" + filename
    
    c.log.Info("Uploading to Yandex Disk", 
        "local", localPath, 
        "remote", remotePath,
        "size_bytes", getFileSize(localPath))
    
    // Upload
    uploadResult, err := c.client.UploadFile(localPath, remotePath, true)
    if err != nil {
        // Try to extract Yandex API error
        if apiErr, ok := err.(*yandexdisk.APIError); ok {
            c.log.Error("Yandex API error",
                "status", apiErr.StatusCode,
                "code", apiErr.Code,  // e.g., "NotFound", "InvalidPath"
                "description", apiErr.Description,
                "details", apiErr.Error())
            return "", fmt.Errorf("Yandex upload failed (HTTP %d): %s", 
                apiErr.StatusCode, apiErr.Description)
        }
        return "", fmt.Errorf("upload failed: %w", err)
    }
    
    c.log.Info("Upload completed, starting publish", "href", uploadResult.Href)
    
    // If async operation, poll for completion
    if uploadResult.Href != "" {
        operationID := filepath.Base(uploadResult.Href)
        status, err := c.client.GetOperationStatus(operationID)
        if err != nil {
            c.log.Warn("Failed to get operation status", "err", err)
        } else {
            c.log.Info("Upload operation status", "status", status.Status)
        }
    }
    
    // Publish
    c.log.Info("Publishing file", "remote", remotePath)
    resource, err := c.client.Publish(remotePath)
    if err != nil {
        if apiErr, ok := err.(*yandexdisk.APIError); ok {
            c.log.Error("Publish failed",
                "status", apiErr.StatusCode,
                "code", apiErr.Code,
                "description", apiErr.Description)
            return "", fmt.Errorf("publish failed (HTTP %d): %s", 
                apiErr.StatusCode, apiErr.Description)
        }
        return "", fmt.Errorf("publish failed: %w", err)
    }
    
    c.log.Info("File published successfully", "public_url", resource.PublicURL)
    return resource.PublicURL, nil
}
```

### Add logging to NewClient
```go
func NewClient(token, folder string) (*Client, error) {
    c.log.Info("Initializing Yandex Disk client", "folder", folder)
    // Don't log the token!
    
    client := yandexdisk.NewClient(token)
    
    // Create folder if it doesn't exist — with better error handling
    c.log.Info("Ensuring Yandex Disk folder exists", "path", folder)
    err := client.CreateFolder(folder)
    if err != nil {
        if apiErr, ok := err.(*yandexdisk.APIError); ok {
            c.log.Error("Failed to create folder",
                "status", apiErr.StatusCode,
                "code", apiErr.Code,
                "path", folder)
            return nil, fmt.Errorf("failed to create folder %q: %w (HTTP %d)", 
                folder, apiErr.Code, apiErr.StatusCode)
        }
        return nil, fmt.Errorf("create folder failed: %w", err)
    }
    
    c.log.Info("Yandex Disk initialized", "folder", folder)
    return &Client{client: client, folder: folder}, nil
}
```

### Add a test with invalid credentials
Create a test that tries to upload with an invalid token and verifies we get a clear error (not a generic "upload failed").

### Manual verification steps
After updating, please provide:

1. **Folder existence check:**
   ```powershell
   curl -H "Authorization: OAuth y0__xDSkcHzARj8pz8gt53K6xYwgdCI6AcN5O6Xi7DdYYsWIsS4WZO1JxkEGg" https://cloud-api.yandex.net/v1/disk/resources?path=/disk/TelegramBot
   ```

2. **Token permissions:**
   ```powershell
   curl -H "Authorization: OAuth y0__xDSkcHzARj8pz8gt53K6xYwgdCI6AcN5O6Xi7DdYYsWIsS4WZO1JxkEGg" https://cloud-api.yandex.net/v1/disk
   ```

Run those to see what Yandex returns. Share the output so we know if it's:
- Token invalid/expired
- Folder permission denied
- Something else

## Definition of Done
- [ ] Better error logging with Yandex API details
- [ ] Distinguish between 401/403 (auth) and 400 (bad request)
- [ ] Folder creation error properly reported
- [ ] go fmt, go vet, go test pass
- [ ] WORKLOG.md updated
