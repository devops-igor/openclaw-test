# TASK-23 Review: Replace Yandex Disk with Local HTTP Server Sharing

**Reviewer:** qa_bot  
**Date:** 2026-03-24  
**Model:** openrouter/xiaomi/mimo-v2-pro

---

## Files Reviewed

1. `internal/serve/server.go` — New HTTP server package
2. `internal/config/config.go` — Share config struct + env overrides
3. `internal/bot/bot.go` — Updated Bot struct and downloadAndSend()
4. `cmd/bot/main.go` — Removed Yandex initialization
5. `go.mod` — Yandex dependency removed

---

## Verification Results

| Check | Result |
|-------|--------|
| `go fmt ./...` | ✅ Clean (no output) |
| `go vet ./...` | ✅ No errors |
| `go build ./cmd/...` | ✅ Compiles successfully |
| `go test ./...` | ✅ All tests pass |
| Yandex references in .go files | ✅ None found |
| YandexDisk config in config.go | ✅ Fully removed |
| yandexClient in bot.go | ✅ Fully removed |
| yandex-disk-go in go.mod | ✅ Removed |
| `internal/yandex/` directory | ✅ Deleted |
| `lib/yandex-disk-go/` directory | ✅ Deleted |

---

## Findings

### BUG — Medium: URL/Port Mismatch When Configured Port Is Unavailable

**Location:** `internal/bot/bot.go:667`

```go
publicURL := srv.BuildURL(b.cfg.Share.Port, srv.Filename())
```

The URL is constructed using `b.cfg.Share.Port` (the configured port), **before** `srv.Serve()` is called on line 674. However, `Serve()` has a port fallback: if the configured port is in use, it falls back to OS-assigned port 0. In that scenario:

1. User receives a download link pointing to the **configured** port (e.g. `:8080`)
2. Server actually listens on a **different** port (e.g. `:49321`)
3. Download link is broken — connection refused

**Impact:** Users would get non-functional download links when the configured share port is occupied by another process.

**Suggested fix:** Add an exported method to `Server` that returns the actual bound port (e.g. `func (s *Server) ActualPort() int`), or restructure `Serve()` to start the listener first and return the URL. Then build/send the message URL after the actual port is known.

### MINOR — Missing Tests for `internal/serve` Package

`internal/serve` has no test files. This is the most critical new package (handles file serving, security, cleanup). Recommended test coverage:

- `BuildURL()` with various host/port configs
- Path traversal attempts against the HTTP handler
- Timeout behavior
- Cleanup after download
- Port fallback behavior

### MINOR — Empty `lib/` Directory Remains

The `lib/` directory exists but is empty (yandex-disk-go was deleted). Cosmetic — can be removed with `rmdir lib`.

---

## Code Quality Assessment

### Security ✅
- Path traversal protection via `filepath.Base()` — safe
- No directory listing
- Explicit GET-only method check
- `http.ServeFile` used correctly with absolute path

### Message Format ✅
Matches spec: `📤 Video ready for download (SIZE MB)\n\n🔗 [Download](URL)\n\n⏰ Link expires in X minutes`

### Timeout & Cleanup ✅
- 3-way select: download complete / timeout / context cancellation
- Graceful 5-second shutdown
- File cleanup via `os.Remove()` after shutdown
- No resource leaks

### Yandex Removal ✅
Complete and clean. No remaining references in code, config, imports, or go.mod.

---

## Verdict: CHANGES NEEDED

**1 issue must be fixed before merge:**

1. **BUG (Medium):** Fix the URL/port mismatch in `downloadAndSend()` — the public URL must reflect the actual bound port, not the configured port. The `serve.Server` needs to expose the actual port before the URL is sent to the user.

**Recommended improvements (non-blocking):**

2. Add unit tests for `internal/serve` package
3. Remove empty `lib/` directory
