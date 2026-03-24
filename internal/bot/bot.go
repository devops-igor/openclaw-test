// Package bot provides Telegram bot initialization, middleware, and command handling.
package bot

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/igorkon/youtube-downloader-bot/internal/config"
	"github.com/igorkon/youtube-downloader-bot/internal/downloader"
	"github.com/igorkon/youtube-downloader-bot/internal/serve"
	"github.com/igorkon/youtube-downloader-bot/pkg/logger"
	"github.com/igorkon/youtube-downloader-bot/pkg/types"
)

// HandlerFunc is the signature for update handlers.
type HandlerFunc func(b *Bot, u *tgbotapi.Update)

// Middleware wraps a HandlerFunc to add cross-cutting behavior.
type Middleware func(next HandlerFunc) HandlerFunc

// MessageSender abstracts sending messages, enabling test doubles.
type MessageSender interface {
	Send(msg tgbotapi.Chattable) (tgbotapi.Message, error)
}

// Bot is the core Telegram bot instance.
type Bot struct {
	api      MessageSender
	cfg      *config.Config
	log      *logger.Logger
	executor *downloader.Executor
	parser   *downloader.Parser
	manager  *downloader.Manager
	state    *state

	// Stats
	startTime     time.Time
	messageCount  atomic.Int64
	downloadCount atomic.Int64

	// Rate limiter state
	mu          sync.Mutex
	rateBuckets map[int64]*rateBucket
}

// rateBucket tracks token-bucket state for a single user.
type rateBucket struct {
	tokens     float64
	lastRefill time.Time
}

// New creates a new Bot instance. It authenticates with the Telegram API
// but does not start polling — call Run() to begin processing updates.
func New(cfg *config.Config, log *logger.Logger, executor *downloader.Executor, parser *downloader.Parser, manager *downloader.Manager) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.Telegram.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot API: %w", err)
	}

	log.Info("Telegram bot authorized", "bot_username", api.Self.UserName)

	return &Bot{
		api:         api,
		cfg:         cfg,
		log:         log.WithComponent("bot"),
		executor:    executor,
		parser:      parser,
		manager:     manager,
		state:       newState(),
		startTime:   time.Now(),
		rateBuckets: make(map[int64]*rateBucket),
	}, nil
}

// NewForTest creates a Bot with a mock MessageSender for unit tests.
func NewForTest(cfg *config.Config, log *logger.Logger, sender MessageSender) *Bot {
	return &Bot{
		api:         sender,
		cfg:         cfg,
		log:         log.WithComponent("bot"),
		state:       newState(),
		startTime:   time.Now(),
		rateBuckets: make(map[int64]*rateBucket),
	}
}

// Run starts the long-polling loop. It blocks until ctx is cancelled.
// Panics if the bot was not created with New() (i.e., api is not *tgbotapi.BotAPI).
func (b *Bot) Run(ctx context.Context) error {
	realAPI, ok := b.api.(*tgbotapi.BotAPI)
	if !ok {
		return fmt.Errorf("Run() requires a real *tgbotapi.BotAPI, use New() to create the bot")
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := realAPI.GetUpdatesChan(u)

	b.log.Info("Bot started polling", "allowed_users", len(b.cfg.Telegram.AllowedUsers))

	handler := b.buildHandler()

	for {
		select {
		case <-ctx.Done():
			b.log.Info("Bot shutting down")
			realAPI.StopReceivingUpdates()
			return ctx.Err()
		case update := <-updates:
			handler(b, &update)
		}
	}
}

// SendMessage sends a text message to the given chat.
func (b *Bot) SendMessage(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	_, err := b.api.Send(msg)
	return err
}

// Config returns the bot's configuration.
func (b *Bot) Config() *config.Config {
	return b.cfg
}

// StartTime returns when the bot was started.
func (b *Bot) StartTime() time.Time {
	return b.startTime
}

// MessageCount returns the total number of messages processed.
func (b *Bot) MessageCount() int64 {
	return b.messageCount.Load()
}

// DownloadCount returns the total number of downloads completed.
func (b *Bot) DownloadCount() int64 {
	return b.downloadCount.Load()
}

// buildHandler constructs the middleware chain ending with the command router.
func (b *Bot) buildHandler() HandlerFunc {
	return WithMiddleware(
		b.loggingMiddleware(),
		b.rateLimitMiddleware(),
		b.whitelistMiddleware(),
	)(func(_ *Bot, u *tgbotapi.Update) {
		b.routeUpdate(u)
	})
}

// WithMiddleware chains middlewares in order: the first middleware wraps the second,
// which wraps the third, and so on, ending with the provided handler.
func WithMiddleware(middlewares ...Middleware) func(HandlerFunc) HandlerFunc {
	return func(handler HandlerFunc) HandlerFunc {
		for i := len(middlewares) - 1; i >= 0; i-- {
			handler = middlewares[i](handler)
		}
		return handler
	}
}

// --- Middleware ---

// loggingMiddleware logs every incoming update.
func (b *Bot) loggingMiddleware() Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(bot *Bot, u *tgbotapi.Update) {
			var userID int64
			var text string
			if u.Message != nil {
				userID = u.Message.From.ID
				text = u.Message.Text
			} else if u.CallbackQuery != nil {
				userID = u.CallbackQuery.From.ID
				text = u.CallbackQuery.Data
			}

			bot.log.Debug("Incoming update",
				"update_id", u.UpdateID,
				"user_id", userID,
				"text", text,
			)

			next(bot, u)
		}
	}
}

// rateLimitMiddleware enforces per-user rate limiting using a token bucket.
func (b *Bot) rateLimitMiddleware() Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(bot *Bot, u *tgbotapi.Update) {
			var userID int64
			var chatID int64
			if u.Message != nil {
				userID = u.Message.From.ID
				chatID = u.Message.Chat.ID
			} else if u.CallbackQuery != nil {
				userID = u.CallbackQuery.From.ID
				chatID = u.CallbackQuery.Message.Chat.ID
			} else {
				next(bot, u)
				return
			}

			limit := float64(bot.cfg.Downloader.RateLimitPerUser)
			if limit <= 0 {
				limit = 10
			}

			if !bot.allowRequest(userID, limit) {
				bot.log.Warn("Rate limit exceeded", "user_id", userID)
				_ = bot.SendMessage(chatID, fmt.Sprintf(
					"⚠️ Rate limit exceeded. You can send up to %d requests per hour. Please try again later.",
					int(limit),
				))
				return
			}

			next(bot, u)
		}
	}
}

// allowRequest checks and deducts a token from the user's bucket.
// Returns true if the request is allowed.
func (b *Bot) allowRequest(userID int64, limit float64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	bucket, exists := b.rateBuckets[userID]
	if !exists {
		bucket = &rateBucket{
			tokens:     limit,
			lastRefill: now,
		}
		b.rateBuckets[userID] = bucket
	}

	// Refill tokens: one hour window
	elapsed := now.Sub(bucket.lastRefill).Hours()
	bucket.tokens += elapsed * limit
	if bucket.tokens > limit {
		bucket.tokens = limit
	}
	bucket.lastRefill = now

	if bucket.tokens < 1 {
		return false
	}

	bucket.tokens--
	return true
}

// whitelistMiddleware rejects users not in the allowed list (if configured).
func (b *Bot) whitelistMiddleware() Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(bot *Bot, u *tgbotapi.Update) {
			allowed := bot.cfg.Telegram.AllowedUsers
			if len(allowed) == 0 {
				// No whitelist configured — allow all
				next(bot, u)
				return
			}

			var userID int64
			var chatID int64
			if u.Message != nil {
				userID = u.Message.From.ID
				chatID = u.Message.Chat.ID
			} else if u.CallbackQuery != nil {
				userID = u.CallbackQuery.From.ID
				chatID = u.CallbackQuery.Message.Chat.ID
			} else {
				next(bot, u)
				return
			}

			if !isAllowed(userID, allowed) {
				bot.log.Error("Unauthorized user — not in allowed list", "user_id", userID, "allowed_count", len(allowed))
				_ = bot.SendMessage(chatID, "🚫 You are not authorized to use this bot.")
				return
			}

			next(bot, u)
		}
	}
}

// isAllowed checks if userID is in the allowed list.
func isAllowed(userID int64, allowed []int64) bool {
	for _, id := range allowed {
		if id == userID {
			return true
		}
	}
	return false
}

// --- Command Handlers ---

// routeUpdate dispatches updates to the appropriate handler.
func (b *Bot) routeUpdate(u *tgbotapi.Update) {
	// Handle callback queries from inline keyboards
	if u.CallbackQuery != nil {
		b.handleCallbackQuery(u)
		return
	}

	if u.Message == nil {
		return
	}

	b.messageCount.Add(1)

	// Non-command text: check for YouTube URLs
	if !u.Message.IsCommand() {
		if u.Message.Text != "" {
			b.log.Info("routeUpdate: non-command text received",
				"user_id", u.Message.From.ID,
				"chat_id", u.Message.Chat.ID,
				"text_len", len(u.Message.Text),
				"text_preview", truncate(u.Message.Text, 120),
			)
			if isYouTubeURL(u.Message.Text) {
				b.log.Info("routeUpdate: YouTube URL detected, calling handleURL", "url", strings.TrimSpace(u.Message.Text))
				b.handleURL(u)
			} else {
				b.log.Info("routeUpdate: text is NOT a YouTube URL, ignoring",
					"user_id", u.Message.From.ID,
					"text_preview", truncate(u.Message.Text, 120),
				)
			}
		} else {
			b.log.Debug("routeUpdate: empty non-command message, ignoring")
		}
		return
	}

	switch u.Message.Command() {
	case "start":
		b.handleStart(u)
	case "help":
		b.handleHelp(u)
	case "status":
		b.handleStatus(u)
	case "cancel":
		b.handleCancel(u)
	default:
		_ = b.SendMessage(u.Message.Chat.ID, "❓ Unknown command. Type /help for available commands.")
	}
}

// isYouTubeURL checks if the text contains a valid YouTube URL.
// Supports: youtube.com/watch?v=..., youtu.be/..., youtube.com/shorts/...
func isYouTubeURL(text string) bool {
	// Quick check before parsing
	text = strings.TrimSpace(text)
	if !strings.Contains(text, "youtube.com") && !strings.Contains(text, "youtu.be") {
		return false
	}

	u, err := url.Parse(text)
	if err != nil {
		return false
	}

	// Must have a scheme
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}

	host := strings.ToLower(u.Host)
	if host != "youtube.com" && host != "www.youtube.com" && host != "youtu.be" && host != "m.youtube.com" {
		return false
	}

	// For youtu.be, path must not be empty
	if host == "youtu.be" {
		return len(u.Path) > 1 // more than just "/"
	}

	// For youtube.com, check path
	path := strings.ToLower(u.Path)
	// Accept /watch, /shorts, /live, /embed
	validPaths := []string{"/watch", "/shorts", "/live", "/embed"}
	for _, vp := range validPaths {
		if path == vp || strings.HasPrefix(path, vp+"/") {
			return true
		}
	}

	return false
}

// cleanYouTubeURL removes tracking/sharing query parameters from YouTube URLs.
// Parameters like ?si=... are share tokens that can confuse yt-dlp.
func cleanYouTubeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	q := u.Query()
	// Remove known tracking parameters
	q.Del("si")

	// For youtu.be short links, strip all query params — none are needed for downloading
	if strings.ToLower(u.Host) == "youtu.be" {
		q = url.Values{}
	}

	u.RawQuery = q.Encode()
	return u.String()
}

// truncate returns a string truncated to maxLen characters with "…" suffix if needed.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}

// handleURL processes a message containing a YouTube URL.
func (b *Bot) handleURL(u *tgbotapi.Update) {
	chatID := u.Message.Chat.ID
	userID := u.Message.From.ID
	rawURL := strings.TrimSpace(u.Message.Text)

	// Panic recovery — catch any nil pointer dereference or unexpected error
	defer func() {
		if r := recover(); r != nil {
			b.log.Error("handleURL: PANIC recovered", "panic", r, "user_id", userID, "url", rawURL)
			_ = b.SendMessage(chatID, "❌ An unexpected error occurred. The bot maintainers have been notified.")
		}
	}()

	// Defensive nil checks — must log at Error level for visibility
	if b.executor == nil {
		b.log.Error("handleURL: executor is nil — not initialized", "user_id", userID)
		_ = b.SendMessage(chatID, "❌ Bot configuration error: download service not initialized.")
		return
	}
	if b.parser == nil {
		b.log.Error("handleURL: parser is nil — not initialized", "user_id", userID)
		_ = b.SendMessage(chatID, "❌ Bot configuration error: format parser not initialized.")
		return
	}

	b.log.Info("YouTube URL received", "user_id", userID, "url", rawURL)

	// Show "typing" action
	_ = b.sendChatAction(chatID, tgbotapi.ChatTyping)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Clean URL: remove tracking parameters (e.g., ?si=...) that can confuse yt-dlp
	cleanURL := cleanYouTubeURL(rawURL)
	b.log.Info("Cleaned URL for yt-dlp", "original", rawURL, "cleaned", cleanURL)

	// List formats
	result, err := b.executor.Run(ctx, "-F", cleanURL)
	if err != nil {
		b.log.Error("Failed to list formats", "error", err, "url", rawURL)
		_ = b.SendMessage(chatID, "❌ Failed to retrieve video formats. Please check the URL and try again.")
		return
	}

	formats, err := b.parser.ParseFormats(result.Stdout)
	if err != nil {
		b.log.Error("Failed to parse formats", "error", err, "url", rawURL)
		_ = b.SendMessage(chatID, "❌ Failed to parse video formats. Please try again later.")
		return
	}

	if len(formats) == 0 {
		b.log.Warn("No formats returned by yt-dlp",
			"url", cleanURL,
			"stdout", result.Stdout,
			"stderr", result.Stderr,
		)
		_ = b.SendMessage(chatID, "❌ Could not fetch video formats. The video may be age-restricted, private, or unavailable in your region.")
		return
	}

	// Filter to only 720p and above, remove audio-only formats
	// Note: size filtering is NOT done here — downloadAndSend() handles routing
	// (≤50MB → Telegram, >50MB → HTTP share) so large formats remain selectable.
	var qualityFormats []types.Format
	for _, f := range formats {
		// Skip audio-only
		if strings.Contains(strings.ToLower(f.Description), "audio only") {
			continue
		}
		// Skip below 720p
		height := extractHeight(f.Resolution)
		if height < 720 {
			continue
		}
		qualityFormats = append(qualityFormats, f)
	}
	if len(qualityFormats) == 0 {
		_ = b.SendMessage(chatID, "❌ No suitable formats found (requires 720p or higher).")
		return
	}

	// Sort qualityFormats by height descending (highest quality first)
	sort.SliceStable(qualityFormats, func(i, j int) bool {
		return extractHeight(qualityFormats[i].Resolution) > extractHeight(qualityFormats[j].Resolution)
	})

	b.log.Info("Formats filtered to 720p+", "total", len(formats), "filtered_720p_plus", len(qualityFormats))

	// Pick top 5
	selected := selectTopFormats(qualityFormats, 5)
	b.log.Info("Formats listed", "user_id", userID, "total", len(formats), "filtered_720p_plus", len(qualityFormats), "selected", len(selected))

	// Store pending state (use cleaned URL for downloads)
	b.state.set(userID, &PendingDownload{
		UserID:    userID,
		URL:       cleanURL,
		Formats:   selected,
		CreatedAt: time.Now(),
	})

	// Build and send inline keyboard
	kb := buildFormatKeyboard(selected)
	msg := tgbotapi.NewMessage(chatID, "🎬 Select quality:")
	msg.ReplyMarkup = kb
	_, err = b.api.Send(msg)
	if err != nil {
		b.log.Error("Failed to send format keyboard", "error", err)
	}
}

// handleCallbackQuery handles inline keyboard button presses.
func (b *Bot) handleCallbackQuery(u *tgbotapi.Update) {
	cb := u.CallbackQuery
	userID := cb.From.ID
	chatID := cb.Message.Chat.ID
	data := cb.Data

	// Always acknowledge the callback immediately (Telegram requires ~30s)
	ack := tgbotapi.NewCallback(cb.ID, "")
	_, _ = b.api.Send(ack)

	// M2: Cancel deletes the pending selection state so the user cannot select
	// a format again. It does NOT stop an in-flight download goroutine — this
	// is intentional: the download continues so the user can still receive the
	// file if they simply wait for it to finish.
	if data == "cancel:pending" {
		b.state.delete(userID)
		_ = b.SendMessage(chatID, "❌ Selection cancelled.")
		return
	}

	// Check for download:{formatID} pattern
	if !strings.HasPrefix(data, "download:") {
		return
	}

	formatID := strings.TrimPrefix(data, "download:")

	// Check pending state
	pd := b.state.get(userID)
	if pd == nil {
		_ = b.SendMessage(chatID, "⏰ Selection expired. Please send the URL again.")
		return
	}

	// Validate that the selected format is in the pending formats
	validFormat := false
	for _, f := range pd.Formats {
		if f.ID == formatID {
			validFormat = true
			break
		}
	}
	if !validFormat {
		b.log.Warn("Invalid format selected", "user_id", userID, "format_id", formatID)
		_ = b.SendMessage(chatID, "❌ Invalid format selected. Please try again.")
		return
	}

	// Delete state early to prevent double-clicks
	b.state.delete(userID)

	b.log.Info("Starting download", "user_id", userID, "format_id", formatID, "url", pd.URL)

	// Send initial "Downloading..." message
	statusMsg := tgbotapi.NewMessage(chatID, "⬇️ Downloading... Please wait.")
	sentMsg, err := b.api.Send(statusMsg)
	if err != nil {
		b.log.Error("Failed to send status message", "error", err)
	}

	// Capture values for goroutine
	url := pd.URL
	msgID := sentMsg.MessageID

	// Spawn goroutine for download (Telegram callbacks must be fast)
	go b.downloadAndSend(chatID, userID, url, formatID, msgID)
}

// downloadAndSend performs the actual download in a background goroutine.
func (b *Bot) downloadAndSend(chatID, userID int64, url, formatID string, statusMsgID int) {
	// Defensive check: manager is required for downloads
	if b.manager == nil {
		b.log.Error("Download manager is nil, cannot download")
		_ = b.SendMessage(chatID, "❌ Download service unavailable. Please try again later.")
		return
	}

	// Create context with timeout for the download
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Perform download
	result, err := b.manager.Download(ctx, url, formatID)

	if err != nil {
		b.log.Error("Download failed", "user_id", userID, "url", url, "format_id", formatID, "error", err)
		errMsg := fmt.Sprintf("❌ Download failed: %s", err.Error())
		// Edit status message to reflect failure
		edit := tgbotapi.NewEditMessageText(chatID, statusMsgID, errMsg)
		_, _ = b.api.Send(edit)
		return
	}

	b.log.Info("Download complete, sending file", "user_id", userID, "file", result.Filename, "size", result.Filesize)

	// M1: Pre-send file size check (Telegram limit is 50MB for bots)
	const maxFileSizeBytes = 50 * 1024 * 1024
	if result.Filesize > maxFileSizeBytes {
		// Serve file via local HTTP server
		b.log.Info("File too large for Telegram, serving via HTTP",
			"user_id", userID, "file", result.Filename, "size", result.Filesize)

		edit := tgbotapi.NewEditMessageText(chatID, statusMsgID, "📤 File too large for Telegram. Preparing download link...")
		_, _ = b.api.Send(edit)

		serveCfg := serve.Config{
			Host:           b.cfg.Share.Host,
			Port:           b.cfg.Share.Port,
			TimeoutMinutes: b.cfg.Share.TimeoutMinutes,
		}
		srv := serve.New(serveCfg, result.FilePath, b.log.Logger)

		// Start the HTTP server in a goroutine so we can get the actual port
		ctx2, cancel2 := context.WithTimeout(context.Background(), time.Duration(b.cfg.Share.TimeoutMinutes)*time.Minute)
		defer cancel2()

		urlCh := make(chan string, 1)
		errCh := make(chan error, 1)
		go func() {
			u, err := srv.Serve(ctx2)
			if err != nil {
				errCh <- err
			} else {
				urlCh <- u
			}
		}()

		// Wait briefly for the server to start and bind its port
		time.Sleep(200 * time.Millisecond)

		actualPort := srv.GetPort()
		sizeMB := float64(result.Filesize) / (1024 * 1024)
		publicURL := srv.BuildURL(actualPort, srv.Filename())
		msg := fmt.Sprintf("📤 Video ready for download (%.1f MB)\n\n🔗 [Download](%s)\n\n⏰ Link expires in %d minutes", sizeMB, publicURL, b.cfg.Share.TimeoutMinutes)
		edit = tgbotapi.NewEditMessageText(chatID, statusMsgID, msg)
		edit.ParseMode = "Markdown"
		_, _ = b.api.Send(edit)

		// Wait for the server to finish (download completes, timeout, or error)
		select {
		case <-urlCh:
			// Download completed
		case serveErr := <-errCh:
			if serveErr != nil {
				b.log.Error("share server error", "error", serveErr, "user_id", userID)
			}
		}
		// File is cleaned up by the server
		b.downloadCount.Add(1)
		return
	}

	// Open file for sending
	file, err := os.Open(result.FilePath)
	if err != nil {
		b.log.Error("Failed to open downloaded file", "error", err, "path", result.FilePath)
		edit := tgbotapi.NewEditMessageText(chatID, statusMsgID, "❌ Failed to send file. Please try again.")
		_, _ = b.api.Send(edit)
		os.Remove(result.FilePath)
		return
	}

	// Send file as document
	doc := tgbotapi.NewDocument(chatID, tgbotapi.FileReader{
		Name:   result.Filename,
		Reader: file,
	})
	_, err = b.api.Send(doc)
	file.Close() // Close immediately after send

	if err != nil {
		b.log.Error("Failed to send document", "error", err, "file", result.Filename)
		edit := tgbotapi.NewEditMessageText(chatID, statusMsgID, "❌ Failed to send file. It may be too large for Telegram.")
		_, _ = b.api.Send(edit)
		os.Remove(result.FilePath)
		return
	}

	// Cleanup: delete the temp file after sending
	if err := os.Remove(result.FilePath); err != nil {
		b.log.Warn("Failed to cleanup temp file", "path", result.FilePath, "error", err)
	}

	// C2: Edit status message to indicate success
	edit := tgbotapi.NewEditMessageText(chatID, statusMsgID, fmt.Sprintf("✅ Sent: %s", result.Filename))
	_, _ = b.api.Send(edit)

	// C3: Increment download counter only after successful send
	b.downloadCount.Add(1)
}

// handleCancel clears pending state for the user.
func (b *Bot) handleCancel(u *tgbotapi.Update) {
	userID := u.Message.From.ID
	chatID := u.Message.Chat.ID
	b.state.delete(userID)
	_ = b.SendMessage(chatID, "✅ Cleared any pending selections.")
}

// sendChatAction sends a chat action (e.g., typing indicator).
func (b *Bot) sendChatAction(chatID int64, action string) error {
	msg := tgbotapi.NewChatAction(chatID, action)
	_, err := b.api.Send(msg)
	return err
}

// filterFormats removes formats that exceed the size limit or have no size info.
func filterFormats(formats []types.Format, maxSizeBytes int64) []types.Format {
	var result []types.Format
	for _, f := range formats {
		if f.Filesize == "" || f.Filesize == "N/A" {
			// Include formats with unknown size — we can't filter them out
			result = append(result, f)
			continue
		}
		// Try to parse size — if we can't, include it anyway
		size, err := parseSizeToBytes(f.Filesize)
		if err != nil {
			result = append(result, f)
			continue
		}
		if size <= maxSizeBytes {
			result = append(result, f)
		}
	}
	return result
}

// parseSizeToBytes converts human-readable sizes like "50.2MiB" to bytes.
func parseSizeToBytes(size string) (int64, error) {
	size = strings.TrimSpace(size)
	if size == "" || size == "N/A" {
		return 0, fmt.Errorf("empty size")
	}

	// Find where the numeric part ends
	numEnd := 0
	for i, c := range size {
		if (c >= '0' && c <= '9') || c == '.' {
			numEnd = i + 1
		} else {
			break
		}
	}
	if numEnd == 0 {
		return 0, fmt.Errorf("cannot parse size: %s", size)
	}

	unit := strings.ToUpper(strings.TrimSpace(size[numEnd:]))

	// Pure number with no unit suffix = bytes
	if unit == "" {
		var bytes int64
		if _, err := fmt.Sscanf(size, "%d", &bytes); err == nil {
			return bytes, nil
		}
		// Try float (e.g., "5.0" as bytes)
		var val float64
		if _, err := fmt.Sscanf(size[:numEnd], "%f", &val); err == nil {
			return int64(val), nil
		}
		return 0, fmt.Errorf("cannot parse size: %s", size)
	}

	// Parse numeric value
	var val float64
	if _, err := fmt.Sscanf(size[:numEnd], "%f", &val); err != nil {
		return 0, fmt.Errorf("cannot parse size: %s", size)
	}

	switch unit {
	case "B", "BYTES":
		return int64(val), nil
	case "KB", "KIB":
		return int64(val * 1024), nil
	case "MB", "MIB":
		return int64(val * 1024 * 1024), nil
	case "GB", "GIB":
		return int64(val * 1024 * 1024 * 1024), nil
	case "TB", "TIB":
		return int64(val * 1024 * 1024 * 1024 * 1024), nil
	default:
		return 0, fmt.Errorf("unknown unit: %s", unit)
	}
}

// extractHeight parses resolution strings like "1920x1080" -> 1080, "720p" -> 720, "audio only" -> 0.
func extractHeight(resolution string) int {
	// Try "WxH" format
	if idx := strings.Index(resolution, "x"); idx != -1 {
		h, _ := strconv.Atoi(resolution[idx+1:])
		return h
	}
	// Try "720p", "1080p60" format
	if strings.HasSuffix(resolution, "p") {
		h, _ := strconv.Atoi(strings.TrimSuffix(resolution, "p"))
		return h
	}
	return 0
}

// selectTopFormats picks the best formats up to maxCount.
// Prefers video formats with audio, then by resolution (highest first).
func selectTopFormats(formats []types.Format, maxCount int) []types.Format {
	if len(formats) <= maxCount {
		return formats
	}

	// Sort: prefer formats with audio, then by resolution
	sort.SliceStable(formats, func(i, j int) bool {
		// Prefer non-audio-only
		iAudio := strings.Contains(strings.ToLower(formats[i].Description), "audio only")
		jAudio := strings.Contains(strings.ToLower(formats[j].Description), "audio only")
		if iAudio != jAudio {
			return !iAudio // non-audio comes first
		}

		// Prefer mp4
		iMp4 := formats[i].Ext == "mp4"
		jMp4 := formats[j].Ext == "mp4"
		if iMp4 != jMp4 {
			return iMp4
		}

		return false
	})

	return formats[:maxCount]
}

// buildFormatKeyboard creates an inline keyboard from a list of formats.
func buildFormatKeyboard(formats []types.Format) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	// Telegram limits inline keyboard button text to 64 UTF-8 characters.
	const maxButtonTextLen = 64

	for _, f := range formats {
		label := f.Description
		if f.Filesize != "" && f.Filesize != "N/A" {
			label = fmt.Sprintf("%s — %s", f.Description, f.Filesize)
		}
		// Truncate button text to Telegram's 64-char limit
		if len(label) > maxButtonTextLen {
			label = label[:maxButtonTextLen-3] + "..."
		}
		btn := tgbotapi.NewInlineKeyboardButtonData(label, "download:"+f.ID)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(btn))
	}

	// Add cancel button
	cancelBtn := tgbotapi.NewInlineKeyboardButtonData("❌ Cancel", "cancel:pending")
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(cancelBtn))

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// handleStart responds to /start with a welcome message.
func (b *Bot) handleStart(u *tgbotapi.Update) {
	text := "🎬 Welcome to YouTube Downloader Bot!\n\n" +
		"Send me a YouTube link and I'll help you download it.\n\n" +
		"Type /help for more information."

	_ = b.SendMessage(u.Message.Chat.ID, text)
}

// handleHelp responds to /help with detailed usage information.
func (b *Bot) handleHelp(u *tgbotapi.Update) {
	var sb strings.Builder
	sb.WriteString("📖 **YouTube Downloader Bot — Help**\n\n")
	sb.WriteString("Available commands:\n")
	sb.WriteString("  /start — Welcome message\n")
	sb.WriteString("  /help  — This help message\n")
	sb.WriteString("  /status — Bot status and stats\n\n")
	sb.WriteString("How to use:\n")
	sb.WriteString("  1. Send a YouTube URL (youtube.com or youtu.be)\n")
	sb.WriteString("  2. Select your preferred quality\n")
	sb.WriteString("  3. Wait for the download to complete\n")
	sb.WriteString("  4. Receive the video file directly in chat\n\n")
	sb.WriteString(fmt.Sprintf("Rate limit: %d requests per user per hour\n",
		b.cfg.Downloader.RateLimitPerUser))
	sb.WriteString(fmt.Sprintf("Max file size: %d MB\n",
		b.cfg.Downloader.MaxFileSizeMB))

	_ = b.SendMessage(u.Message.Chat.ID, sb.String())
}

// handleStatus responds to /status with bot runtime info.
func (b *Bot) handleStatus(u *tgbotapi.Update) {
	uptime := time.Since(b.startTime).Truncate(time.Second)

	var sb strings.Builder
	sb.WriteString("📊 **Bot Status**\n\n")
	sb.WriteString(fmt.Sprintf("Uptime: %s\n", uptime))
	sb.WriteString(fmt.Sprintf("Messages processed: %d\n", b.messageCount.Load()))
	sb.WriteString(fmt.Sprintf("Downloads completed: %d\n", b.downloadCount.Load()))
	sb.WriteString(fmt.Sprintf("Allowed users: %d\n", len(b.cfg.Telegram.AllowedUsers)))
	sb.WriteString(fmt.Sprintf("Rate limit: %d/hour\n", b.cfg.Downloader.RateLimitPerUser))

	_ = b.SendMessage(u.Message.Chat.ID, sb.String())
}
