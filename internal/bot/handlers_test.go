package bot

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/user/tgbot/internal/ai"
	"github.com/user/tgbot/internal/auth"
	"github.com/user/tgbot/internal/logger"
	"github.com/user/tgbot/internal/session"
	"github.com/user/tgbot/internal/telegram"
)

// --------------- mock ai.Provider ---------------

type mockProvider struct {
	mu        sync.Mutex
	respondFn func(ctx context.Context, req ai.Request) (ai.Result, error)
	calls     []ai.Request
	model     string
}

func (m *mockProvider) Respond(ctx context.Context, req ai.Request) (ai.Result, error) {
	m.mu.Lock()
	m.calls = append(m.calls, req)
	fn := m.respondFn
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, req)
	}
	return ai.Result{Text: "mock response", ResponseID: "mock-resp-id"}, nil
}

func (m *mockProvider) ModelName() string {
	if m.model != "" {
		return m.model
	}
	return "mock-model"
}

func (m *mockProvider) getCalls() []ai.Request {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]ai.Request, len(m.calls))
	copy(result, m.calls)
	return result
}

// --------------- test environment ---------------

type sentMsg struct {
	Text    string
	IsPhoto bool
}

type testEnv struct {
	bot       *Bot
	server    *httptest.Server
	provider  *mockProvider
	mu        sync.Mutex
	sent      []sentMsg
	photoFail bool
}

func (e *testEnv) getSent() []sentMsg {
	e.mu.Lock()
	defer e.mu.Unlock()
	result := make([]sentMsg, len(e.sent))
	copy(result, e.sent)
	return result
}

func newTestEnv(t *testing.T, allowedUsers ...int64) *testEnv {
	t.Helper()

	const token = "test-token"
	env := &testEnv{
		provider: &mockProvider{model: "test-model"},
	}

	testImageBytes := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	}

	env.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		if strings.HasPrefix(path, "/file/") {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(testImageBytes)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(path, "/getMe"):
			fmt.Fprint(w, `{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"Test","username":"test_bot"}}`)
		case strings.HasSuffix(path, "/getFile"):
			env.mu.Lock()
			pf := env.photoFail
			env.mu.Unlock()
			if pf {
				fmt.Fprint(w, `{"ok":false,"error_code":400,"description":"Bad Request: invalid file_id"}`)
			} else {
				fmt.Fprint(w, `{"ok":true,"result":{"file_id":"test-file-id","file_unique_id":"unique","file_size":16,"file_path":"photos/test.jpg"}}`)
			}
		case strings.HasSuffix(path, "/sendMessage"):
			text := r.FormValue("text")
			env.mu.Lock()
			env.sent = append(env.sent, sentMsg{Text: text})
			env.mu.Unlock()
			fmt.Fprint(w, `{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":1,"type":"private"}}}`)
		case strings.HasSuffix(path, "/sendChatAction"):
			fmt.Fprint(w, `{"ok":true,"result":true}`)
		case strings.HasSuffix(path, "/sendPhoto"):
			env.mu.Lock()
			env.sent = append(env.sent, sentMsg{IsPhoto: true})
			env.mu.Unlock()
			fmt.Fprint(w, `{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":1,"type":"private"}}}`)
		case strings.HasSuffix(path, "/setMyCommands"):
			fmt.Fprint(w, `{"ok":true,"result":true}`)
		default:
			fmt.Fprint(w, `{"ok":true,"result":{}}`)
		}
	}))
	t.Cleanup(env.server.Close)

	api, err := tgbotapi.NewBotAPIWithClient(token, env.server.URL+"/bot%s/%s", env.server.Client())
	if err != nil {
		t.Fatalf("failed to create bot api: %v", err)
	}

	log := logger.New(logger.Config{Level: "error", Output: io.Discard})
	env.bot = &Bot{
		tg:              telegram.NewClient(api, log),
		ai:              env.provider,
		sessions:        session.NewManager(20, 0),
		auth:            auth.NewWhitelist(allowedUsers, log),
		log:             log,
		sem:             make(chan struct{}, 20),
		requestTimeout:  60 * time.Second,
		maxPromptLength: 4000,
	}

	return env
}

// --------------- message builders ---------------

func makeTextMsg(userID int64, text string) *tgbotapi.Message {
	return &tgbotapi.Message{
		MessageID: 1,
		From:      &tgbotapi.User{ID: userID, UserName: "testuser", FirstName: "Test"},
		Chat:      &tgbotapi.Chat{ID: userID, Type: "private"},
		Text:      text,
		Date:      1700000000,
	}
}

func makeCommandMsg(userID int64, command string) *tgbotapi.Message {
	text := "/" + command
	return &tgbotapi.Message{
		MessageID: 1,
		From:      &tgbotapi.User{ID: userID, UserName: "testuser", FirstName: "Test"},
		Chat:      &tgbotapi.Chat{ID: userID, Type: "private"},
		Text:      text,
		Date:      1700000000,
		Entities: []tgbotapi.MessageEntity{
			{Type: "bot_command", Offset: 0, Length: len(text)},
		},
	}
}

func makePhotoMsg(userID int64, caption string) *tgbotapi.Message {
	return &tgbotapi.Message{
		MessageID: 1,
		From:      &tgbotapi.User{ID: userID, UserName: "testuser", FirstName: "Test"},
		Chat:      &tgbotapi.Chat{ID: userID, Type: "private"},
		Caption:   caption,
		Photo: []tgbotapi.PhotoSize{
			{FileID: "small-file-id", Width: 90, Height: 90},
			{FileID: "test-file-id", Width: 800, Height: 600},
		},
		Date: 1700000000,
	}
}

func enablePhotoDownload(t *testing.T, env *testEnv) {
	t.Helper()
	env.bot.tg.SetFileBaseURL(env.server.URL + "/file")
}

// --------------- access control ---------------

func TestHandleMessage_AccessDenied(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, 999)
	ctx := context.Background()

	env.bot.handleMessage(ctx, makeTextMsg(123, "hello"))

	sent := env.getSent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(sent))
	}
	if !strings.Contains(sent[0].Text, "Access denied") {
		t.Fatalf("expected access denied message, got %q", sent[0].Text)
	}
	if len(env.provider.getCalls()) != 0 {
		t.Fatalf("AI should not be called when access is denied")
	}
}

// --------------- command handlers ---------------

func TestHandleCommand_Start(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	ctx := context.Background()

	env.bot.handleMessage(ctx, makeCommandMsg(1, "start"))

	sent := env.getSent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(sent))
	}
	if !strings.Contains(sent[0].Text, "Hello") {
		t.Fatalf("expected welcome message, got %q", sent[0].Text)
	}
}

func TestHandleCommand_New(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	ctx := context.Background()

	env.bot.sessions.AddWithResponseID(1, "resp-old", session.Message{Role: "user", Content: "old"})

	env.bot.handleMessage(ctx, makeCommandMsg(1, "new"))

	sent := env.getSent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(sent))
	}
	if !strings.Contains(sent[0].Text, "cleared") {
		t.Fatalf("expected cleared message, got %q", sent[0].Text)
	}
	if msgs := env.bot.sessions.Get(1); len(msgs) != 0 {
		t.Fatalf("session should be cleared, got %d messages", len(msgs))
	}
}

func TestHandleCommand_Model(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	ctx := context.Background()

	env.bot.handleMessage(ctx, makeCommandMsg(1, "model"))

	sent := env.getSent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(sent))
	}
	if !strings.Contains(sent[0].Text, "test-model") {
		t.Fatalf("expected model name in response, got %q", sent[0].Text)
	}
}

func TestHandleCommand_Help(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	ctx := context.Background()

	env.bot.handleMessage(ctx, makeCommandMsg(1, "help"))

	sent := env.getSent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(sent))
	}
	if !strings.Contains(sent[0].Text, "Help") {
		t.Fatalf("expected help text, got %q", sent[0].Text)
	}
}

func TestHandleCommand_Unknown(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	ctx := context.Background()

	env.bot.handleMessage(ctx, makeCommandMsg(1, "nonexistent"))

	sent := env.getSent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(sent))
	}
	if !strings.Contains(sent[0].Text, "Unknown command") {
		t.Fatalf("expected unknown command message, got %q", sent[0].Text)
	}
}

// --------------- text message routing ---------------

func TestHandleTextMessage_ChatMode(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.provider.respondFn = func(_ context.Context, _ ai.Request) (ai.Result, error) {
		return ai.Result{Text: "AI says hello", ResponseID: "resp-1"}, nil
	}

	env.bot.handleMessage(context.Background(), makeTextMsg(1, "what is 2+2?"))

	calls := env.provider.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 AI call, got %d", len(calls))
	}
	if calls[0].Mode != ai.ModeChat {
		t.Fatalf("expected mode %q, got %q", ai.ModeChat, calls[0].Mode)
	}
	sent := env.getSent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(sent))
	}
	if sent[0].Text != "AI says hello" {
		t.Fatalf("expected %q, got %q", "AI says hello", sent[0].Text)
	}
}

func TestHandleTextMessage_ImageGeneration(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.provider.respondFn = func(_ context.Context, _ ai.Request) (ai.Result, error) {
		return ai.Result{
			ImageBytes:    []byte("fake-png"),
			ImageMimeType: "image/png",
			ResponseID:    "resp-img-1",
		}, nil
	}

	env.bot.handleMessage(context.Background(), makeTextMsg(1, "нарисуй кота"))

	calls := env.provider.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 AI call, got %d", len(calls))
	}
	if calls[0].Mode != ai.ModeGenerateImage {
		t.Fatalf("expected mode %q, got %q", ai.ModeGenerateImage, calls[0].Mode)
	}

	found := false
	for _, s := range env.getSent() {
		if s.IsPhoto {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a photo to be sent")
	}
}

func TestHandleTextMessage_ImageGenerationWithSize(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.provider.respondFn = func(_ context.Context, _ ai.Request) (ai.Result, error) {
		return ai.Result{ImageBytes: []byte("img"), ImageMimeType: "image/png", ResponseID: "r1"}, nil
	}

	env.bot.handleMessage(context.Background(), makeTextMsg(1, "нарисуй кота 1024x1536"))

	calls := env.provider.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 AI call, got %d", len(calls))
	}
	if calls[0].ImageSize != "1024x1536" {
		t.Fatalf("expected image size %q, got %q", "1024x1536", calls[0].ImageSize)
	}
}

func TestHandleTextMessage_UnsupportedImageSize(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)

	env.bot.handleMessage(context.Background(), makeTextMsg(1, "нарисуй кота 800x600"))

	if len(env.provider.getCalls()) != 0 {
		t.Fatalf("AI should not be called for unsupported size")
	}
	found := false
	for _, s := range env.getSent() {
		if strings.Contains(s.Text, "Unsupported image size") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected unsupported size message")
	}
}

func TestHandleTextMessage_ExplicitDrawPrefix(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.provider.respondFn = func(_ context.Context, _ ai.Request) (ai.Result, error) {
		return ai.Result{ImageBytes: []byte("img"), ImageMimeType: "image/png", ResponseID: "r1"}, nil
	}

	env.bot.handleMessage(context.Background(), makeTextMsg(1, "draw: neon poster"))

	calls := env.provider.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 AI call, got %d", len(calls))
	}
	if calls[0].Mode != ai.ModeGenerateImage {
		t.Fatalf("expected mode %q, got %q", ai.ModeGenerateImage, calls[0].Mode)
	}
	if calls[0].Text != "neon poster" {
		t.Fatalf("expected prompt %q, got %q", "neon poster", calls[0].Text)
	}
}

func TestHandleTextMessage_ExplicitEditNoImage(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)

	env.bot.handleMessage(context.Background(), makeTextMsg(1, "edit: remove background"))

	if len(env.provider.getCalls()) != 0 {
		t.Fatalf("AI should not be called when no image source available")
	}
	found := false
	for _, s := range env.getSent() {
		if strings.Contains(s.Text, "No image found") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 'no image found' message")
	}
}

func TestHandleTextMessage_ImgPrefixFallbackGenerate(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.provider.respondFn = func(_ context.Context, _ ai.Request) (ai.Result, error) {
		return ai.Result{ImageBytes: []byte("img"), ImageMimeType: "image/png", ResponseID: "r1"}, nil
	}

	env.bot.handleMessage(context.Background(), makeTextMsg(1, "img: cute hedgehog"))

	calls := env.provider.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 AI call, got %d", len(calls))
	}
	if calls[0].Mode != ai.ModeGenerateImage {
		t.Fatalf("expected fallback to %q, got %q", ai.ModeGenerateImage, calls[0].Mode)
	}
}

func TestHandleTextMessage_ImgPrefixEditExisting(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.bot.sessions.AddWithResponseID(1, "prev-resp", session.Message{
		Role: "assistant", Content: "a cat", ImageData: "data:image/png;base64,AAAA",
	})
	env.provider.respondFn = func(_ context.Context, _ ai.Request) (ai.Result, error) {
		return ai.Result{ImageBytes: []byte("edited"), ImageMimeType: "image/png", ResponseID: "r2"}, nil
	}

	env.bot.handleMessage(context.Background(), makeTextMsg(1, "img: add hat"))

	calls := env.provider.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 AI call, got %d", len(calls))
	}
	if calls[0].Mode != ai.ModeEditImage {
		t.Fatalf("expected mode %q, got %q", ai.ModeEditImage, calls[0].Mode)
	}
	if calls[0].InputImageData != "data:image/png;base64,AAAA" {
		t.Fatalf("expected input image from session")
	}
}

func TestHandleTextMessage_AIError(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.provider.respondFn = func(_ context.Context, _ ai.Request) (ai.Result, error) {
		return ai.Result{}, fmt.Errorf("API rate limit exceeded")
	}

	env.bot.handleMessage(context.Background(), makeTextMsg(1, "hello"))

	found := false
	for _, s := range env.getSent() {
		if strings.Contains(s.Text, "error occurred") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected error message to user")
	}
}

func TestHandleTextMessage_AIError_RateLimit(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.provider.respondFn = func(_ context.Context, _ ai.Request) (ai.Result, error) {
		return ai.Result{}, &ai.AIError{Kind: ai.ErrRateLimit, Message: "rate limit exceeded"}
	}

	env.bot.handleMessage(context.Background(), makeTextMsg(1, "hello"))

	found := false
	for _, s := range env.getSent() {
		if strings.Contains(s.Text, "temporarily overloaded") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected rate limit message to user, got %v", env.getSent())
	}
}

func TestHandleTextMessage_AIError_Auth(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.provider.respondFn = func(_ context.Context, _ ai.Request) (ai.Result, error) {
		return ai.Result{}, &ai.AIError{Kind: ai.ErrAuth, Message: "invalid key"}
	}

	env.bot.handleMessage(context.Background(), makeTextMsg(1, "hello"))

	found := false
	for _, s := range env.getSent() {
		if strings.Contains(s.Text, "configuration error") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected auth error message to user, got %v", env.getSent())
	}
}

func TestHandleTextMessage_AIError_BadRequest(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.provider.respondFn = func(_ context.Context, _ ai.Request) (ai.Result, error) {
		return ai.Result{}, &ai.AIError{Kind: ai.ErrBadRequest, Message: "content policy violation"}
	}

	env.bot.handleMessage(context.Background(), makeTextMsg(1, "hello"))

	found := false
	for _, s := range env.getSent() {
		if strings.Contains(s.Text, "could not be processed") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected bad request message to user, got %v", env.getSent())
	}
}

func TestHandleTextMessage_AIError_Transient(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.provider.respondFn = func(_ context.Context, _ ai.Request) (ai.Result, error) {
		return ai.Result{}, &ai.AIError{Kind: ai.ErrTransient, Message: "server error"}
	}

	env.bot.handleMessage(context.Background(), makeTextMsg(1, "hello"))

	found := false
	for _, s := range env.getSent() {
		if strings.Contains(s.Text, "temporarily unavailable") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected transient error message to user, got %v", env.getSent())
	}
}

func TestHandleImageRequest_AIError_RateLimit(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.provider.respondFn = func(_ context.Context, _ ai.Request) (ai.Result, error) {
		return ai.Result{}, &ai.AIError{Kind: ai.ErrRateLimit, Message: "rate limit exceeded"}
	}

	env.bot.handleMessage(context.Background(), makeTextMsg(1, "draw: a cat"))

	found := false
	for _, s := range env.getSent() {
		if strings.Contains(s.Text, "temporarily overloaded") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected rate limit message for image request, got %v", env.getSent())
	}
}

// --------------- prompt length validation ---------------

func TestHandleTextMessage_TooLong(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.bot.maxPromptLength = 50

	longText := strings.Repeat("a", 51)
	env.bot.handleMessage(context.Background(), makeTextMsg(1, longText))

	if len(env.provider.getCalls()) != 0 {
		t.Fatalf("AI should not be called for too-long messages")
	}
	found := false
	for _, s := range env.getSent() {
		if strings.Contains(s.Text, "Message too long") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 'Message too long' rejection message")
	}
}

func TestHandleTextMessage_ExactlyAtLimit(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.bot.maxPromptLength = 10

	env.bot.handleMessage(context.Background(), makeTextMsg(1, strings.Repeat("x", 10)))

	if len(env.provider.getCalls()) != 1 {
		t.Fatalf("expected 1 AI call for message at exact limit, got %d", len(env.provider.getCalls()))
	}
}

func TestHandleTextMessage_MultibyteTooLong(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.bot.maxPromptLength = 5

	// 6 Cyrillic runes, each 2 bytes — should be rejected by rune count, not byte count
	env.bot.handleMessage(context.Background(), makeTextMsg(1, "абвгде"))

	if len(env.provider.getCalls()) != 0 {
		t.Fatalf("AI should not be called for too-long multibyte messages")
	}
	found := false
	for _, s := range env.getSent() {
		if strings.Contains(s.Text, "Message too long") && strings.Contains(s.Text, "6") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected rejection message with rune count 6")
	}
}

// --------------- session context preservation ---------------

func TestHandleTextMessage_SessionContext(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	env.provider.respondFn = func(_ context.Context, _ ai.Request) (ai.Result, error) {
		return ai.Result{Text: "reply", ResponseID: "resp-fixed"}, nil
	}

	ctx := context.Background()
	env.bot.handleMessage(ctx, makeTextMsg(1, "hello"))
	env.bot.handleMessage(ctx, makeTextMsg(1, "follow up"))

	calls := env.provider.getCalls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 AI calls, got %d", len(calls))
	}
	if len(calls[0].History) != 1 {
		t.Fatalf("first call: expected 1 history message, got %d", len(calls[0].History))
	}
	if len(calls[1].History) != 3 {
		t.Fatalf("second call: expected 3 history messages, got %d", len(calls[1].History))
	}
	if calls[1].PreviousResponseID != "resp-fixed" {
		t.Fatalf("second call: expected previous response ID %q, got %q", "resp-fixed", calls[1].PreviousResponseID)
	}
}

// --------------- sendResult / persistResult ---------------

func TestSendResult_TextOnly(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)

	env.bot.sendResult(1, ai.Result{Text: "some text"})

	sent := env.getSent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(sent))
	}
	if sent[0].Text != "some text" {
		t.Fatalf("expected %q, got %q", "some text", sent[0].Text)
	}
	if sent[0].IsPhoto {
		t.Fatalf("expected text message, not photo")
	}
}

func TestSendResult_Image(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)

	env.bot.sendResult(1, ai.Result{
		Text:          "caption",
		ImageBytes:    []byte("png-data"),
		ImageMimeType: "image/png",
	})

	found := false
	for _, s := range env.getSent() {
		if s.IsPhoto {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a photo to be sent")
	}
}

func TestPersistResult_Text(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)

	userMsg := session.Message{Role: "user", Content: "hello"}
	result := ai.Result{Text: "response text", ResponseID: "resp-abc"}
	env.bot.persistResult(1, userMsg, result)

	msgs := env.bot.sessions.Get(1)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages in session, got %d", len(msgs))
	}
	if msgs[0].Content != "hello" {
		t.Fatalf("expected user content %q, got %q", "hello", msgs[0].Content)
	}
	if msgs[1].Content != "response text" {
		t.Fatalf("expected assistant content %q, got %q", "response text", msgs[1].Content)
	}
	if env.bot.sessions.GetPreviousResponseID(1) != "resp-abc" {
		t.Fatalf("expected previous response ID %q", "resp-abc")
	}
}

func TestPersistResult_WithImage(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)

	userMsg := session.Message{Role: "user", Content: "draw cat"}
	result := ai.Result{
		Text:          "here you go",
		ImageBytes:    []byte{0x89, 0x50, 0x4E, 0x47},
		ImageMimeType: "image/png",
		ResponseID:    "resp-img",
	}
	env.bot.persistResult(1, userMsg, result)

	msgs := env.bot.sessions.Get(1)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[1].ImageData == "" {
		t.Fatalf("expected image data in assistant message")
	}
	if !strings.HasPrefix(msgs[1].ImageData, "data:image/png;base64,") {
		t.Fatalf("expected data URI prefix, got %q", msgs[1].ImageData[:30])
	}
}

// --------------- photo message handling ---------------

func TestHandlePhotoMessage_ChatAnalysis(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	enablePhotoDownload(t, env)
	env.provider.respondFn = func(_ context.Context, _ ai.Request) (ai.Result, error) {
		return ai.Result{Text: "I see a cat", ResponseID: "resp-photo-1"}, nil
	}

	env.bot.handleMessage(context.Background(), makePhotoMsg(1, ""))

	calls := env.provider.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 AI call, got %d", len(calls))
	}
	if calls[0].Mode != ai.ModeChat {
		t.Fatalf("expected mode %q, got %q", ai.ModeChat, calls[0].Mode)
	}
	lastMsg := calls[0].History[len(calls[0].History)-1]
	if lastMsg.ImageData == "" {
		t.Fatalf("expected image data in user message")
	}
	if !strings.HasPrefix(lastMsg.ImageData, "data:image/") {
		t.Fatalf("expected data URI, got %q", lastMsg.ImageData[:30])
	}

	sent := env.getSent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(sent))
	}
	if sent[0].Text != "I see a cat" {
		t.Fatalf("expected %q, got %q", "I see a cat", sent[0].Text)
	}
}

func TestHandlePhotoMessage_ChatAnalysisWithCaption(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	enablePhotoDownload(t, env)
	env.provider.respondFn = func(_ context.Context, _ ai.Request) (ai.Result, error) {
		return ai.Result{Text: "analysis result", ResponseID: "resp-1"}, nil
	}

	env.bot.handleMessage(context.Background(), makePhotoMsg(1, "what is this?"))

	calls := env.provider.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 AI call, got %d", len(calls))
	}
	if calls[0].Mode != ai.ModeChat {
		t.Fatalf("expected mode %q, got %q", ai.ModeChat, calls[0].Mode)
	}
	lastMsg := calls[0].History[len(calls[0].History)-1]
	if lastMsg.Content != "what is this?" {
		t.Fatalf("expected caption %q, got %q", "what is this?", lastMsg.Content)
	}
	if lastMsg.ImageData == "" {
		t.Fatalf("expected image data in user message")
	}
}

func TestHandlePhotoMessage_EditIntent(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	enablePhotoDownload(t, env)
	env.provider.respondFn = func(_ context.Context, _ ai.Request) (ai.Result, error) {
		return ai.Result{ImageBytes: []byte("edited"), ImageMimeType: "image/png", ResponseID: "r1"}, nil
	}

	env.bot.handleMessage(context.Background(), makePhotoMsg(1, "remove background"))

	calls := env.provider.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 AI call, got %d", len(calls))
	}
	if calls[0].Mode != ai.ModeEditImage {
		t.Fatalf("expected mode %q, got %q", ai.ModeEditImage, calls[0].Mode)
	}
	if calls[0].InputImageData == "" {
		t.Fatalf("expected input image data for edit")
	}
}

func TestHandlePhotoMessage_ExplicitEditPrefix(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	enablePhotoDownload(t, env)
	env.provider.respondFn = func(_ context.Context, _ ai.Request) (ai.Result, error) {
		return ai.Result{ImageBytes: []byte("edited"), ImageMimeType: "image/png", ResponseID: "r1"}, nil
	}

	env.bot.handleMessage(context.Background(), makePhotoMsg(1, "edit: make it brighter"))

	calls := env.provider.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 AI call, got %d", len(calls))
	}
	if calls[0].Mode != ai.ModeEditImage {
		t.Fatalf("expected mode %q, got %q", ai.ModeEditImage, calls[0].Mode)
	}
	if calls[0].Text != "make it brighter" {
		t.Fatalf("expected prompt %q, got %q", "make it brighter", calls[0].Text)
	}
}

func TestHandlePhotoMessage_ExplicitDrawPrefix(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	enablePhotoDownload(t, env)
	env.provider.respondFn = func(_ context.Context, _ ai.Request) (ai.Result, error) {
		return ai.Result{ImageBytes: []byte("generated"), ImageMimeType: "image/png", ResponseID: "r1"}, nil
	}

	env.bot.handleMessage(context.Background(), makePhotoMsg(1, "draw: a sunset"))

	calls := env.provider.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 AI call, got %d", len(calls))
	}
	if calls[0].Mode != ai.ModeGenerateImage {
		t.Fatalf("expected mode %q, got %q", ai.ModeGenerateImage, calls[0].Mode)
	}
	if calls[0].Text != "a sunset" {
		t.Fatalf("expected prompt %q, got %q", "a sunset", calls[0].Text)
	}
}

func TestHandlePhotoMessage_ExplicitImgPrefixEditsUploadedPhoto(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	enablePhotoDownload(t, env)
	env.provider.respondFn = func(_ context.Context, _ ai.Request) (ai.Result, error) {
		return ai.Result{ImageBytes: []byte("edited"), ImageMimeType: "image/png", ResponseID: "r1"}, nil
	}

	env.bot.handleMessage(context.Background(), makePhotoMsg(1, "img: add a hat"))

	calls := env.provider.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 AI call, got %d", len(calls))
	}
	if calls[0].Mode != ai.ModeEditImage {
		t.Fatalf("expected mode %q, got %q", ai.ModeEditImage, calls[0].Mode)
	}
	if calls[0].InputImageData == "" {
		t.Fatalf("expected uploaded photo as input image data")
	}
	if !strings.HasPrefix(calls[0].InputImageData, "data:image/") {
		t.Fatalf("expected data URI for input image, got %q", calls[0].InputImageData[:30])
	}
}

func TestHandlePhotoMessage_EditWithImageSize(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	enablePhotoDownload(t, env)
	env.provider.respondFn = func(_ context.Context, _ ai.Request) (ai.Result, error) {
		return ai.Result{ImageBytes: []byte("edited"), ImageMimeType: "image/png", ResponseID: "r1"}, nil
	}

	env.bot.handleMessage(context.Background(), makePhotoMsg(1, "edit: change colors 1024x1536"))

	calls := env.provider.getCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 AI call, got %d", len(calls))
	}
	if calls[0].ImageSize != "1024x1536" {
		t.Fatalf("expected image size %q, got %q", "1024x1536", calls[0].ImageSize)
	}
}

func TestHandlePhotoMessage_DownloadFailure(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	enablePhotoDownload(t, env)

	env.mu.Lock()
	env.photoFail = true
	env.mu.Unlock()

	env.bot.handleMessage(context.Background(), makePhotoMsg(1, "analyze this"))

	if len(env.provider.getCalls()) != 0 {
		t.Fatalf("AI should not be called when photo download fails")
	}
	found := false
	for _, s := range env.getSent() {
		if strings.Contains(s.Text, "Failed to process the image") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected error message about failed image processing")
	}
}

func TestHandlePhotoMessage_CaptionTooLong(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	enablePhotoDownload(t, env)
	env.bot.maxPromptLength = 20

	env.bot.handleMessage(context.Background(), makePhotoMsg(1, strings.Repeat("a", 21)))

	if len(env.provider.getCalls()) != 0 {
		t.Fatalf("AI should not be called for too-long caption")
	}
	found := false
	for _, s := range env.getSent() {
		if strings.Contains(s.Text, "Message too long") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 'Message too long' rejection")
	}
}

func TestHandlePhotoMessage_SessionPersistence(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	enablePhotoDownload(t, env)
	env.provider.respondFn = func(_ context.Context, _ ai.Request) (ai.Result, error) {
		return ai.Result{Text: "I see a photo", ResponseID: "resp-photo"}, nil
	}

	env.bot.handleMessage(context.Background(), makePhotoMsg(1, "describe this"))

	msgs := env.bot.sessions.Get(1)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages in session, got %d", len(msgs))
	}
	if msgs[0].ImageData == "" {
		t.Fatalf("expected image data persisted in user message")
	}
	if msgs[0].Content != "describe this" {
		t.Fatalf("expected caption in user message, got %q", msgs[0].Content)
	}
	if msgs[1].Content != "I see a photo" {
		t.Fatalf("expected assistant response in session, got %q", msgs[1].Content)
	}
	if env.bot.sessions.GetPreviousResponseID(1) != "resp-photo" {
		t.Fatalf("expected previous response ID %q", "resp-photo")
	}
}
