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

    tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
    "github.com/user/tgbot/internal/ai"
    "github.com/user/tgbot/internal/auth"
    "github.com/user/tgbot/internal/logger"
    "github.com/user/tgbot/internal/session"
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
    bot      *Bot
    server   *httptest.Server
    provider *mockProvider
    mu       sync.Mutex
    sent     []sentMsg
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

    env.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        path := r.URL.Path
        switch {
        case strings.HasSuffix(path, "/getMe"):
            fmt.Fprint(w, `{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"Test","username":"test_bot"}}`)
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
        api:      api,
        ai:       env.provider,
        sessions: session.NewManager(20),
        auth:     auth.NewWhitelist(allowedUsers, log),
        log:      log,
        sem:      make(chan struct{}, 20),
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
    // After first call: session has user + assistant = 2 messages; second call adds new user = 3
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
