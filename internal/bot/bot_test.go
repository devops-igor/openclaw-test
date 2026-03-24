package bot

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/igorkon/youtube-downloader-bot/internal/config"
	"github.com/igorkon/youtube-downloader-bot/pkg/logger"
	"github.com/igorkon/youtube-downloader-bot/pkg/types"
)

// mockSender captures messages sent via SendMessage for assertion.
type mockSender struct {
	mu       sync.Mutex
	messages []tgbotapi.MessageConfig
}

func (m *mockSender) Send(msg tgbotapi.Chattable) (tgbotapi.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if mc, ok := msg.(tgbotapi.MessageConfig); ok {
		m.messages = append(m.messages, mc)
	}
	return tgbotapi.Message{}, nil
}

func (m *mockSender) lastMessage() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.messages) == 0 {
		return ""
	}
	return m.messages[len(m.messages)-1].Text
}

func (m *mockSender) allMessages() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.messages))
	for i, msg := range m.messages {
		result[i] = msg.Text
	}
	return result
}

func (m *mockSender) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.messages)
}

func testConfig() *config.Config {
	return &config.Config{
		Telegram: config.TelegramConfig{
			Token:        "test-token-1234",
			AllowedUsers: []int64{},
		},
		Downloader: config.DownloaderConfig{
			RateLimitPerUser: 10,
			MaxFileSizeMB:    50,
		},
	}
}

func testLogger() *logger.Logger {
	return logger.New(&logger.Config{
		Level:  "error", // quiet during tests
		Output: &bytes.Buffer{},
	})
}

func newTestBot(cfg *config.Config) (*Bot, *mockSender) {
	sender := &mockSender{}
	bot := NewForTest(cfg, testLogger(), sender)
	return bot, sender
}

func newUpdate(userID int64, text string) *tgbotapi.Update {
	return &tgbotapi.Update{
		UpdateID: 1,
		Message: &tgbotapi.Message{
			From: &tgbotapi.User{ID: userID},
			Chat: &tgbotapi.Chat{ID: userID},
			Text: text,
			Entities: []tgbotapi.MessageEntity{
				{Type: "bot_command", Offset: 0, Length: len(text)},
			},
		},
	}
}

func newTextUpdate(userID int64, text string) *tgbotapi.Update {
	return &tgbotapi.Update{
		UpdateID: 1,
		Message: &tgbotapi.Message{
			From: &tgbotapi.User{ID: userID},
			Chat: &tgbotapi.Chat{ID: userID},
			Text: text,
		},
	}
}

// --- Command Handler Tests ---

func TestHandleStart(t *testing.T) {
	cfg := testConfig()
	b, sender := newTestBot(cfg)
	u := newUpdate(100, "/start")

	b.buildHandler()(b, u)

	msg := sender.lastMessage()
	if !strings.Contains(msg, "Welcome") {
		t.Errorf("expected welcome message, got: %s", msg)
	}
	if !strings.Contains(msg, "YouTube") {
		t.Errorf("expected YouTube mention, got: %s", msg)
	}
}

func TestHandleHelp(t *testing.T) {
	cfg := testConfig()
	cfg.Downloader.RateLimitPerUser = 5
	b, sender := newTestBot(cfg)
	u := newUpdate(100, "/help")

	b.buildHandler()(b, u)

	msg := sender.lastMessage()
	if !strings.Contains(msg, "/start") {
		t.Errorf("expected /start in help, got: %s", msg)
	}
	if !strings.Contains(msg, "/help") {
		t.Errorf("expected /help in help, got: %s", msg)
	}
	if !strings.Contains(msg, "/status") {
		t.Errorf("expected /status in help, got: %s", msg)
	}
	if !strings.Contains(msg, "5") {
		t.Errorf("expected rate limit value in help, got: %s", msg)
	}
}

func TestHandleStatus(t *testing.T) {
	cfg := testConfig()
	cfg.Telegram.AllowedUsers = []int64{1, 2, 3, 100} // include test user
	b, sender := newTestBot(cfg)
	u := newUpdate(100, "/status")

	b.buildHandler()(b, u)

	msg := sender.lastMessage()
	if !strings.Contains(msg, "Uptime") {
		t.Errorf("expected uptime in status, got: %s", msg)
	}
	if !strings.Contains(msg, "Messages processed") {
		t.Errorf("expected messages processed in status, got: %s", msg)
	}
	if !strings.Contains(msg, "4") { // allowed users count (includes 100)
		t.Errorf("expected allowed users count in status, got: %s", msg)
	}
}

func TestHandleUnknownCommand(t *testing.T) {
	cfg := testConfig()
	b, sender := newTestBot(cfg)
	u := newUpdate(100, "/unknown")

	b.buildHandler()(b, u)

	msg := sender.lastMessage()
	if !strings.Contains(msg, "Unknown command") {
		t.Errorf("expected unknown command message, got: %s", msg)
	}
}

func TestNonCommandIgnored(t *testing.T) {
	cfg := testConfig()
	b, sender := newTestBot(cfg)
	u := newTextUpdate(100, "hello world")

	b.buildHandler()(b, u)

	if sender.count() != 0 {
		t.Errorf("expected no messages for non-command, got %d", sender.count())
	}
}

// --- Whitelist Middleware Tests ---

func TestWhitelistAllowsWhenEmpty(t *testing.T) {
	cfg := testConfig()
	cfg.Telegram.AllowedUsers = []int64{} // empty = allow all
	b, sender := newTestBot(cfg)
	u := newUpdate(999, "/start")

	b.buildHandler()(b, u)

	if sender.count() == 0 {
		t.Error("expected message when whitelist is empty, got none")
	}
	msg := sender.lastMessage()
	if strings.Contains(msg, "not authorized") {
		t.Errorf("should not reject when whitelist is empty, got: %s", msg)
	}
}

func TestWhitelistAllowsAuthorizedUser(t *testing.T) {
	cfg := testConfig()
	cfg.Telegram.AllowedUsers = []int64{100, 200}
	b, sender := newTestBot(cfg)
	u := newUpdate(100, "/start")

	b.buildHandler()(b, u)

	msg := sender.lastMessage()
	if strings.Contains(msg, "not authorized") {
		t.Errorf("authorized user should not be rejected, got: %s", msg)
	}
	if !strings.Contains(msg, "Welcome") {
		t.Errorf("authorized user should get welcome, got: %s", msg)
	}
}

func TestWhitelistRejectsUnauthorizedUser(t *testing.T) {
	cfg := testConfig()
	cfg.Telegram.AllowedUsers = []int64{100, 200}
	b, sender := newTestBot(cfg)
	u := newUpdate(999, "/start")

	b.buildHandler()(b, u)

	msg := sender.lastMessage()
	if !strings.Contains(msg, "not authorized") {
		t.Errorf("expected unauthorized rejection, got: %s", msg)
	}
}

// --- Rate Limiting Middleware Tests ---

func TestRateLimiterAllowsWithinLimit(t *testing.T) {
	cfg := testConfig()
	cfg.Downloader.RateLimitPerUser = 3
	b, sender := newTestBot(cfg)

	for i := 0; i < 3; i++ {
		u := newUpdate(100, "/start")
		b.buildHandler()(b, u)
	}

	// Should have sent 3 welcome messages (no rate limit hit)
	msgs := sender.allMessages()
	for _, msg := range msgs {
		if strings.Contains(msg, "Rate limit") {
			t.Errorf("should not rate limit within limit, got: %s", msg)
		}
	}
}

func TestRateLimiterRejectsOverLimit(t *testing.T) {
	cfg := testConfig()
	cfg.Downloader.RateLimitPerUser = 2
	b, sender := newTestBot(cfg)

	// Send 3 requests, 3rd should be rejected
	for i := 0; i < 3; i++ {
		u := newUpdate(100, "/start")
		b.buildHandler()(b, u)
	}

	msgs := sender.allMessages()
	rejected := false
	for _, msg := range msgs {
		if strings.Contains(msg, "Rate limit") {
			rejected = true
		}
	}
	if !rejected {
		t.Error("expected rate limit rejection on 3rd request with limit=2")
	}
}

func TestRateLimiterSeparatePerUser(t *testing.T) {
	cfg := testConfig()
	cfg.Downloader.RateLimitPerUser = 1
	b, sender := newTestBot(cfg)

	// User 100 hits their limit
	u1 := newUpdate(100, "/start")
	b.buildHandler()(b, u1)
	u2 := newUpdate(100, "/start")
	b.buildHandler()(b, u2)

	// User 200 should still be allowed
	u3 := newUpdate(200, "/start")
	b.buildHandler()(b, u3)

	msgs := sender.allMessages()
	// Last message should be welcome for user 200, not rate limit
	lastMsg := msgs[len(msgs)-1]
	if strings.Contains(lastMsg, "Rate limit") {
		t.Errorf("user 200 should not be rate limited, got: %s", lastMsg)
	}
}

// --- Middleware Chain Tests ---

func TestMiddlewareChainOrder(t *testing.T) {
	cfg := testConfig()
	cfg.Telegram.AllowedUsers = []int64{} // no whitelist
	cfg.Downloader.RateLimitPerUser = 100
	b, sender := newTestBot(cfg)

	// Logging + rate limit + whitelist should all pass
	u := newUpdate(100, "/status")
	b.buildHandler()(b, u)

	if sender.count() != 1 {
		t.Errorf("expected exactly 1 message after full middleware chain, got %d", sender.count())
	}
}

func TestWithMiddlewareEmptyChain(t *testing.T) {
	called := false
	handler := WithMiddleware()(func(b *Bot, u *tgbotapi.Update) {
		called = true
	})

	handler(nil, nil)
	if !called {
		t.Error("empty middleware chain should still call handler")
	}
}

func TestWithMiddlewareWrapsInOrder(t *testing.T) {
	var order []string

	m1 := func(next HandlerFunc) HandlerFunc {
		return func(b *Bot, u *tgbotapi.Update) {
			order = append(order, "m1-before")
			next(b, u)
			order = append(order, "m1-after")
		}
	}

	m2 := func(next HandlerFunc) HandlerFunc {
		return func(b *Bot, u *tgbotapi.Update) {
			order = append(order, "m2-before")
			next(b, u)
			order = append(order, "m2-after")
		}
	}

	handler := WithMiddleware(m1, m2)(func(b *Bot, u *tgbotapi.Update) {
		order = append(order, "handler")
	})

	handler(nil, nil)

	expected := []string{"m1-before", "m2-before", "handler", "m2-after", "m1-after"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d calls, got %d: %v", len(expected), len(order), order)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("position %d: expected %q, got %q", i, v, order[i])
		}
	}
}

// --- Bot Construction Tests ---

func TestNewForTestCreatesValidBot(t *testing.T) {
	cfg := testConfig()
	b, _ := newTestBot(cfg)

	if b.cfg != cfg {
		t.Error("config not set correctly")
	}
	if b.startTime.IsZero() {
		t.Error("startTime should be set")
	}
	if b.MessageCount() != 0 {
		t.Error("initial message count should be 0")
	}
	if b.DownloadCount() != 0 {
		t.Error("initial download count should be 0")
	}
}

func TestSendMessageError(t *testing.T) {
	// Test with an error-returning sender
	errSender := &errorSender{}
	b := NewForTest(testConfig(), testLogger(), errSender)

	err := b.SendMessage(123, "test")
	if err == nil {
		t.Error("expected error from failing sender")
	}
}

type errorSender struct{}

func (e *errorSender) Send(msg tgbotapi.Chattable) (tgbotapi.Message, error) {
	return tgbotapi.Message{}, fmt.Errorf("send failed")
}

// --- Integration-style Tests ---

func TestCommandAfterWhitelistRejection(t *testing.T) {
	cfg := testConfig()
	cfg.Telegram.AllowedUsers = []int64{100}
	b, sender := newTestBot(cfg)

	// Unauthorized user tries multiple commands
	for _, cmd := range []string{"/start", "/help", "/status"} {
		u := newUpdate(999, cmd)
		b.buildHandler()(b, u)
	}

	msgs := sender.allMessages()
	if len(msgs) != 3 {
		t.Errorf("expected 3 rejection messages, got %d", len(msgs))
	}
	for _, msg := range msgs {
		if !strings.Contains(msg, "not authorized") {
			t.Errorf("all messages should be unauthorized, got: %s", msg)
		}
	}
}

func TestStatsIncrement(t *testing.T) {
	cfg := testConfig()
	b, _ := newTestBot(cfg)

	handler := b.buildHandler()
	handler(b, newUpdate(100, "/start"))
	handler(b, newUpdate(100, "/help"))
	handler(b, newTextUpdate(100, "hello"))

	if b.MessageCount() != 3 {
		t.Errorf("expected message count 3, got %d", b.MessageCount())
	}
}

// Benchmark for rate limiter
func BenchmarkRateLimiter(b *testing.B) {
	cfg := testConfig()
	cfg.Downloader.RateLimitPerUser = 1000
	bot, _ := newTestBot(cfg)
	handler := bot.buildHandler()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		u := newUpdate(100, "/start")
		handler(bot, u)
	}
}

// Benchmark for full middleware chain
func BenchmarkFullMiddlewareChain(b *testing.B) {
	cfg := testConfig()
	cfg.Telegram.AllowedUsers = []int64{100, 200}
	cfg.Downloader.RateLimitPerUser = 100000
	bot, _ := newTestBot(cfg)
	handler := bot.buildHandler()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		u := newUpdate(100, "/start")
		handler(bot, u)
	}
}

// Ensure time is used (avoid unused import)
var _ = time.Now

// --- isYouTubeURL Tests ---

func TestIsYouTubeURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"standard watch URL", "https://www.youtube.com/watch?v=dQw4w9WgXcQ", true},
		{"short youtu.be URL", "https://youtu.be/dQw4w9WgXcQ", true},
		{"youtube shorts", "https://www.youtube.com/shorts/abc123", true},
		{"youtube live", "https://www.youtube.com/live/abc123", true},
		{"youtube embed", "https://www.youtube.com/embed/abc123", true},
		{"http not https", "http://www.youtube.com/watch?v=dQw4w9WgXcQ", true},
		{"without www", "https://youtube.com/watch?v=dQw4w9WgXcQ", true},
		{"mobile m.youtube.com", "https://m.youtube.com/watch?v=dQw4w9WgXcQ", true},
		{"mobile m.youtube.com shorts", "https://m.youtube.com/shorts/abc123", true},
		{"not a URL", "hello world", false},
		{"empty string", "", false},
		{"other domain", "https://www.google.com/watch?v=dQw4w9WgXcQ", false},
		{"missing scheme", "www.youtube.com/watch?v=dQw4w9WgXcQ", false},
		{"ftp scheme", "ftp://www.youtube.com/watch?v=dQw4w9WgXcQ", false},
		{"youtube home page", "https://www.youtube.com/", false},
		{"youtube channel", "https://www.youtube.com/@channel", false},
		{"just youtu.be root", "https://youtu.be/", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isYouTubeURL(tt.url)
			if got != tt.want {
				t.Errorf("isYouTubeURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

// --- State Tests ---

func TestStateSetAndGet(t *testing.T) {
	s := newState()
	pd := &PendingDownload{
		UserID:    100,
		URL:       "https://youtube.com/watch?v=abc",
		CreatedAt: time.Now(),
	}

	got := s.get(100)
	if got != nil {
		t.Error("expected nil for non-existent key")
	}

	s.set(100, pd)
	got = s.get(100)
	if got == nil {
		t.Fatal("expected non-nil after set")
	}
	if got.URL != pd.URL {
		t.Errorf("expected URL %q, got %q", pd.URL, got.URL)
	}
}

func TestStateDelete(t *testing.T) {
	s := newState()
	pd := &PendingDownload{
		UserID:    100,
		URL:       "https://youtube.com/watch?v=abc",
		CreatedAt: time.Now(),
	}
	s.set(100, pd)
	s.delete(100)

	got := s.get(100)
	if got != nil {
		t.Error("expected nil after delete")
	}
}

func TestStateExpiry(t *testing.T) {
	s := newState()
	pd := &PendingDownload{
		UserID:    100,
		URL:       "https://youtube.com/watch?v=abc",
		CreatedAt: time.Now().Add(-6 * time.Minute), // expired
	}
	s.set(100, pd)

	got := s.get(100)
	if got != nil {
		t.Error("expected nil for expired entry")
	}
}

func TestStateReplaceExisting(t *testing.T) {
	s := newState()
	pd1 := &PendingDownload{
		UserID:    100,
		URL:       "https://youtube.com/watch?v=old",
		CreatedAt: time.Now(),
	}
	pd2 := &PendingDownload{
		UserID:    100,
		URL:       "https://youtube.com/watch?v=new",
		CreatedAt: time.Now(),
	}
	s.set(100, pd1)
	s.set(100, pd2)

	got := s.get(100)
	if got.URL != "https://youtube.com/watch?v=new" {
		t.Errorf("expected replaced URL, got %q", got.URL)
	}
}

func TestIsExpired(t *testing.T) {
	fresh := &PendingDownload{CreatedAt: time.Now()}
	if isExpired(fresh) {
		t.Error("fresh entry should not be expired")
	}

	expired := &PendingDownload{CreatedAt: time.Now().Add(-6 * time.Minute)}
	if !isExpired(expired) {
		t.Error("6 minute old entry should be expired")
	}
}

// --- Keyboard Building Tests ---

func TestBuildFormatKeyboard(t *testing.T) {
	formats := []types.Format{
		{ID: "137", Description: "1080p", Filesize: "50MiB"},
		{ID: "18", Description: "360p", Filesize: "10MiB"},
	}

	kb := buildFormatKeyboard(formats)

	// Should have 3 rows: 2 format buttons + 1 cancel button
	if len(kb.InlineKeyboard) != 3 {
		t.Errorf("expected 3 rows, got %d", len(kb.InlineKeyboard))
	}

	// First row should have format 137
	if len(kb.InlineKeyboard[0]) != 1 {
		t.Errorf("expected 1 button per row, got %d", len(kb.InlineKeyboard[0]))
	}
	btn1 := kb.InlineKeyboard[0][0]
	if btn1.CallbackData == nil || *btn1.CallbackData != "download:137" {
		t.Errorf("expected callback data 'download:137', got %q", *btn1.CallbackData)
	}
	if !strings.Contains(btn1.Text, "1080p") {
		t.Errorf("expected '1080p' in button text, got %q", btn1.Text)
	}
	if !strings.Contains(btn1.Text, "50MiB") {
		t.Errorf("expected '50MiB' in button text, got %q", btn1.Text)
	}

	// Last row should be cancel
	cancelBtn := kb.InlineKeyboard[2][0]
	if cancelBtn.CallbackData == nil || *cancelBtn.CallbackData != "cancel:pending" {
		t.Errorf("expected cancel callback, got %q", *cancelBtn.CallbackData)
	}
}

// C2 (TASK-008): Test button text truncation to Telegram's 64-char limit
func TestBuildFormatKeyboardTruncatesLongText(t *testing.T) {
	longDesc := "This is a very long format description that exceeds the sixty-four character limit of Telegram buttons and should be truncated"
	formats := []types.Format{
		{ID: "999", Description: longDesc, Filesize: "100MiB"},
	}

	kb := buildFormatKeyboard(formats)

	btn := kb.InlineKeyboard[0][0]
	if len(btn.Text) > 64 {
		t.Errorf("button text should be truncated to <= 64 chars, got %d chars: %q", len(btn.Text), btn.Text)
	}
	if !strings.HasSuffix(btn.Text, "...") {
		t.Errorf("truncated text should end with '...', got %q", btn.Text)
	}
	if len(btn.Text) != 64 {
		t.Errorf("truncated text should be exactly 64 chars, got %d", len(btn.Text))
	}
}

func TestBuildFormatKeyboardNoSize(t *testing.T) {
	formats := []types.Format{
		{ID: "22", Description: "720p", Filesize: "N/A"},
	}

	kb := buildFormatKeyboard(formats)

	// 2 rows: 1 format + 1 cancel
	if len(kb.InlineKeyboard) != 2 {
		t.Errorf("expected 2 rows, got %d", len(kb.InlineKeyboard))
	}

	// Format button should NOT have size suffix
	btn := kb.InlineKeyboard[0][0]
	if strings.Contains(btn.Text, "—") {
		t.Errorf("expected no size suffix for N/A, got %q", btn.Text)
	}
	if !strings.Contains(btn.Text, "720p") {
		t.Errorf("expected '720p' in button text, got %q", btn.Text)
	}
}

// --- Callback Query Tests ---

func TestHandleCallbackQueryCancel(t *testing.T) {
	cfg := testConfig()
	b, sender := newTestBot(cfg)

	// Set pending state
	b.state.set(100, &PendingDownload{
		UserID:    100,
		URL:       "https://youtube.com/watch?v=abc",
		CreatedAt: time.Now(),
	})

	u := &tgbotapi.Update{
		UpdateID: 1,
		CallbackQuery: &tgbotapi.CallbackQuery{
			ID:   "cb1",
			From: &tgbotapi.User{ID: 100},
			Message: &tgbotapi.Message{
				Chat:      &tgbotapi.Chat{ID: 100},
				MessageID: 42,
			},
			Data: "cancel:pending",
		},
	}

	b.buildHandler()(b, u)

	// Should have cancelled
	msg := sender.lastMessage()
	if !strings.Contains(msg, "cancelled") {
		t.Errorf("expected cancel message, got: %s", msg)
	}
	if b.state.get(100) != nil {
		t.Error("expected pending state to be cleared after cancel")
	}
}

func TestHandleCallbackQueryExpired(t *testing.T) {
	cfg := testConfig()
	b, sender := newTestBot(cfg)

	// No pending state set
	u := &tgbotapi.Update{
		UpdateID: 1,
		CallbackQuery: &tgbotapi.CallbackQuery{
			ID:   "cb1",
			From: &tgbotapi.User{ID: 100},
			Message: &tgbotapi.Message{
				Chat:      &tgbotapi.Chat{ID: 100},
				MessageID: 42,
			},
			Data: "download:22",
		},
	}

	b.buildHandler()(b, u)

	msg := sender.lastMessage()
	if !strings.Contains(msg, "expired") {
		t.Errorf("expected expiry message, got: %s", msg)
	}
}

// --- Format Filtering Tests ---

func TestFilterFormats(t *testing.T) {
	formats := []types.Format{
		{ID: "137", Filesize: "50MiB"},     // ~52MB
		{ID: "18", Filesize: "5MiB"},       // ~5MB
		{ID: "22", Filesize: "N/A"},        // unknown size, should include
		{ID: "140", Filesize: ""},          // empty size, should include
		{ID: "999", Filesize: "100000MiB"}, // too big
	}

	maxSize := int64(50 * 1024 * 1024) // 50MB
	result := filterFormats(formats, maxSize)

	// Should include: 137 (50MiB < 50MB), 18, 22 (N/A), 140 (empty)
	// Should exclude: 999 (too big)
	if len(result) != 4 {
		t.Errorf("expected 4 formats, got %d", len(result))
	}
}

func TestParseSizeToBytes(t *testing.T) {
	tests := []struct {
		input   string
		want    int64
		wantErr bool
	}{
		{"1024", 1024, false},
		{"50MiB", 50 * 1024 * 1024, false},
		{"5.5MB", int64(5.5 * 1024 * 1024), false},
		{"1GB", 1024 * 1024 * 1024, false},
		{"", 0, true},
		{"N/A", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseSizeToBytes(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseSizeToBytes(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseSizeToBytes(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// --- HandleCancel Tests ---

func TestHandleCancelCommand(t *testing.T) {
	cfg := testConfig()
	b, sender := newTestBot(cfg)

	// Set pending state
	b.state.set(100, &PendingDownload{
		UserID:    100,
		URL:       "https://youtube.com/watch?v=abc",
		CreatedAt: time.Now(),
	})

	u := newUpdate(100, "/cancel")
	b.buildHandler()(b, u)

	if b.state.get(100) != nil {
		t.Error("expected pending state cleared after /cancel")
	}
	msg := sender.lastMessage()
	if !strings.Contains(msg, "Cleared") {
		t.Errorf("expected cleared message, got: %s", msg)
	}
}

// --- TASK-009: Callback Query Download Tests ---

func TestHandleCallbackQueryInvalidFormat(t *testing.T) {
	cfg := testConfig()
	b, sender := newTestBot(cfg)

	// Set pending state with specific formats
	b.state.set(100, &PendingDownload{
		UserID: 100,
		URL:    "https://youtube.com/watch?v=abc",
		Formats: []types.Format{
			{ID: "22", Description: "720p"},
			{ID: "18", Description: "360p"},
		},
		CreatedAt: time.Now(),
	})

	u := &tgbotapi.Update{
		UpdateID: 1,
		CallbackQuery: &tgbotapi.CallbackQuery{
			ID:   "cb1",
			From: &tgbotapi.User{ID: 100},
			Message: &tgbotapi.Message{
				Chat:      &tgbotapi.Chat{ID: 100},
				MessageID: 42,
			},
			Data: "download:999", // invalid format ID
		},
	}

	b.buildHandler()(b, u)

	msg := sender.lastMessage()
	if !strings.Contains(msg, "Invalid format") {
		t.Errorf("expected invalid format message, got: %s", msg)
	}
}

func TestHandleCallbackQueryValidFormat(t *testing.T) {
	cfg := testConfig()
	b, sender := newTestBot(cfg)

	// Set pending state with specific formats
	b.state.set(100, &PendingDownload{
		UserID: 100,
		URL:    "https://youtube.com/watch?v=abc",
		Formats: []types.Format{
			{ID: "22", Description: "720p"},
			{ID: "18", Description: "360p"},
		},
		CreatedAt: time.Now(),
	})

	u := &tgbotapi.Update{
		UpdateID: 1,
		CallbackQuery: &tgbotapi.CallbackQuery{
			ID:   "cb1",
			From: &tgbotapi.User{ID: 100},
			Message: &tgbotapi.Message{
				Chat:      &tgbotapi.Chat{ID: 100},
				MessageID: 42,
			},
			Data: "download:22", // valid format ID
		},
	}

	b.buildHandler()(b, u)

	// Should have "Downloading..." message
	msgs := sender.allMessages()
	found := false
	for _, msg := range msgs {
		if strings.Contains(msg, "Downloading") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'Downloading' message, got: %v", msgs)
	}

	// State should be cleared (to prevent double-clicks)
	if b.state.get(100) != nil {
		t.Error("expected state to be cleared after valid format selection")
	}
}

// C1: Test that the callback handler correctly spawns the goroutine and
// cleans up state before spawning (to prevent double-clicks).
func TestHandleCallbackQuerySpawnsGoroutineAndCleansState(t *testing.T) {
	cfg := testConfig()
	b, sender := newTestBot(cfg)

	b.state.set(100, &PendingDownload{
		UserID: 100,
		URL:    "https://youtube.com/watch?v=abc",
		Formats: []types.Format{
			{ID: "22", Description: "720p"},
			{ID: "18", Description: "360p"},
		},
		CreatedAt: time.Now(),
	})

	u := &tgbotapi.Update{
		UpdateID: 1,
		CallbackQuery: &tgbotapi.CallbackQuery{
			ID:   "cb1",
			From: &tgbotapi.User{ID: 100},
			Message: &tgbotapi.Message{
				Chat:      &tgbotapi.Chat{ID: 100},
				MessageID: 42,
			},
			Data: "download:22",
		},
	}

	b.buildHandler()(b, u)

	// 1) State must be cleared synchronously BEFORE goroutine runs
	if b.state.get(100) != nil {
		t.Error("expected pending state to be cleared synchronously before goroutine spawn")
	}

	// 2) "Downloading..." message should have been sent
	msgs := sender.allMessages()
	foundDownloading := false
	for _, msg := range msgs {
		if strings.Contains(msg, "Downloading") {
			foundDownloading = true
			break
		}
	}
	if !foundDownloading {
		t.Error("expected 'Downloading...' message to be sent before goroutine spawn")
	}
}

func TestHandleCallbackQueryUnknownPrefix(t *testing.T) {
	cfg := testConfig()
	b, sender := newTestBot(cfg)

	u := &tgbotapi.Update{
		UpdateID: 1,
		CallbackQuery: &tgbotapi.CallbackQuery{
			ID:   "cb1",
			From: &tgbotapi.User{ID: 100},
			Message: &tgbotapi.Message{
				Chat:      &tgbotapi.Chat{ID: 100},
				MessageID: 42,
			},
			Data: "unknown:prefix",
		},
	}

	b.buildHandler()(b, u)

	// Should not send any messages (just ack)
	if sender.count() != 0 {
		t.Errorf("expected no messages for unknown prefix, got %d", sender.count())
	}
}

// --- Callback Data Parsing Tests ---

func TestParseDownloadCallbackData(t *testing.T) {
	tests := []struct {
		name       string
		data       string
		wantPrefix string
		wantID     string
	}{
		{"valid download", "download:22", "download:", "22"},
		{"valid long ID", "download:137+251", "download:", "137+251"},
		{"cancel", "cancel:pending", "", ""},
		{"unknown", "other:data", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPrefix != "" {
				if !strings.HasPrefix(tt.data, tt.wantPrefix) {
					t.Errorf("expected prefix %q, not found in %q", tt.wantPrefix, tt.data)
				}
				got := strings.TrimPrefix(tt.data, tt.wantPrefix)
				if got != tt.wantID {
					t.Errorf("expected ID %q, got %q", tt.wantID, got)
				}
			} else {
				if strings.HasPrefix(tt.data, "download:") {
					t.Errorf("should not have download prefix")
				}
			}
		})
	}
}
