# TgBot - Telegram AI Assistant

## Overview

Telegram bot on Go providing access to AI through the OpenAI Responses API. The bot supports plain text chat, photo understanding, image generation, and image editing while keeping a lightweight in-memory session layer and explicit image-mode routing.

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Language | Go | Simple deployment (single binary), good performance, excellent libraries |
| Telegram SDK | go-telegram-bot-api/v5 | Mature, actively maintained library |
| OpenAI SDK | openai/openai-go | Native Responses API support and built-in hosted tools |
| Authorization | Whitelist by user_id | Simple, reliable, no database required |
| Conversation context | In-memory + `previous_response_id` + latest image lookup | Cheap continuity without a database |
| Configuration | Env vars | Container standard, secrets security |

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌────────────────────┐
│   Telegram  │────▶│   TgBot     │────▶│ OpenAI Responses   │
│   User      │◀────│   (Go)      │◀────│ API + image tool   │
└─────────────┘     └─────────────┘     └────────────────────┘
                          │
                   ┌──────┴──────┐
                   │             │
             ┌─────▼─────┐ ┌─────▼─────────────┐
             │ Whitelist │ │ Session Manager   │
             │   Auth    │ │ history + images  │
             └───────────┘ └───────────────────┘
```

### Message Processing Flow

```mermaid
sequenceDiagram
    participant U as User
    participant T as Telegram
    participant B as Bot
    participant A as Auth
    participant S as Session
    participant AI as Responses API

    U->>T: Message
    T->>B: Update
    B->>A: Check user_id
    alt Not in whitelist
        A-->>B: Deny
        B-->>T: "Access denied"
    else In whitelist
        A-->>B: OK
        B->>S: Load history + previous_response_id + latest image
        alt Natural-language or explicit image routing
            B->>B: Detect trigger / explicit prefix / optional size
            B->>AI: Responses API + image_generation tool
            AI-->>B: text +/or generated image + response.id
        else Normal text or photo analysis
            B->>AI: Responses API multimodal request
            AI-->>B: text + response.id
        end
        B->>S: Save messages, response.id, latest image
        B-->>T: Text and/or image reply
    end
    T-->>U: Response
```

## Project Structure

```
TgBot/
├── .github/
│   └── workflows/
│       └── ci.yml               # CI pipeline: lint + test + coverage gate
├── cmd/
│   └── bot/
│       └── main.go              # Entry point, component initialization
├── internal/
│   ├── ai/
│   │   ├── provider.go          # AI provider interface and Message type
│   │   ├── openai.go            # OpenAI implementation
│   │   └── errors.go            # Structured AI error types and classification
│   ├── auth/
│   │   └── whitelist.go         # Access control by user_id
│   ├── bot/
│   │   ├── bot.go               # Bot struct, Run loop, command dispatch
│   │   └── handlers.go          # Text/photo message handling, AI dispatch
│   ├── config/
│   │   └── config.go            # Configuration loading from env
│   ├── logger/
│   │   └── logger.go            # Logging abstraction (slog implementation)
│   ├── routing/
│   │   └── routing.go           # Image prefix parsing, intent matchers, size extraction
│   ├── session/
│   │   └── session.go           # In-memory conversation context storage
│   ├── telegram/
│   │   └── client.go            # Telegram send helpers, photo download/encode
│   └── version/
│       └── version.go           # Build version info (git commit, date)
├── scripts/
│   └── deploy-docker.sh         # Pull and run Docker container on VPS
├── Makefile                     # Build, test, lint, fmt, cover, docker targets
├── .env.example                 # Configuration example
├── Dockerfile                   # Multi-stage build
├── docker-compose.yml           # For local development
├── README.md                    # Brief description
├── ARCHITECTURE.md              # This file
└── DEPLOYMENT.md                # Build and deployment guide
```

## Modules

### `internal/config`

Loads configuration from environment variables:
- `TELEGRAM_BOT_TOKEN` - bot token from @BotFather
- `OPENAI_API_KEY` - OpenAI API key
- `OPENAI_MODEL` - model (default: gpt-4o)
- `OPENAI_BASE_URL` - API base URL (for compatible providers)
- `OPENAI_MAX_RETRIES` - max automatic retries for transient API errors (default: 3)
- `ALLOWED_USERS` - comma-separated list of allowed user_id
- `MAX_HISTORY` - max messages in context (default: 20)
- `SESSION_TTL` - idle session lifetime, Go duration (default: 24h)
- `MAX_CONCURRENCY` - max concurrent message handlers (default: 20)
- `REQUEST_TIMEOUT` - per-request timeout, Go duration (default: 60s)
- `MAX_PROMPT_LENGTH` - max prompt length in characters (default: 4000)
- `LOG_LEVEL` - logging level: debug, info, warn, error (default: info)
- `LOG_FORMAT` - log format: text, json (default: text)

### `internal/auth`

Whitelist authorization:
- Parses user_id list at startup
- Method `IsAllowed(userID int64) bool`
- Logs unauthorized access attempts

### `internal/session`

Conversation context management:
- Stores history by user_id in `map[int64]*Session`
- Thread-safe via `sync.RWMutex`; most methods acquire write locks to update `LastActivity` on every access
- Stores `PreviousResponseID` for Responses API continuity
- Can retrieve the latest image stored in the session
- Methods: `GetSessionID`, `Get`, `AddWithResponseID`, `GetPreviousResponseID`, `GetLatestImage`, `Clear`, `SessionCount`, `StartCleanup`
- `AddWithResponseID` appends messages and updates `PreviousResponseID` in a single lock acquisition
- `Clear` returns a new session ID for log tracing
- Automatic history depth limiting
- TTL-based session eviction: each session tracks `LastActivity`; a background goroutine (`StartCleanup`) periodically removes sessions idle longer than `SESSION_TTL`

### `internal/ai`

Provider interface and OpenAI implementation:

```go
type Provider interface {
    Respond(ctx context.Context, req Request) (Result, error)
    ModelName() string
}
```

`Request` includes:

- request mode (`chat`, `generate_image`, `edit_image`)
- text prompt
- optional image size (`1024x1024`, `1024x1536`, `1536x1024`)
- message history
- input image data
- `previous_response_id`

`Result` includes:

- text output
- raw image bytes + mime type
- response ID for multi-turn continuity

The `ai` package defines its own `ai.Message` type (independent of `session.Message`), so it does not import the `session` package. Conversion between `session.Message` and `ai.Message` happens in `internal/bot/handlers.go`.

The OpenAI provider uses `github.com/openai/openai-go` and sends all chat, vision, generation, and editing flows through the Responses API. Image generation and editing use the built-in `image_generation` tool with:

- `gpt-image-1` as the hosted image model
- `png` output
- `auto` quality/background by default
- user-selected image size when a supported `<width>x<height>` pattern is detected

#### Error Classification (`errors.go`)

Structured error handling for upstream API errors:

- `AIError` wraps an API error with a classified `ErrorKind`
- `ErrorKind` enum: `ErrTransient` (5xx, 408, 409), `ErrRateLimit` (429), `ErrAuth` (401/403), `ErrBadRequest` (400), `ErrUnknown`
- `classifyError()` inspects `openai.Error` and wraps it as `*AIError` with the appropriate kind
- Predicate functions: `IsRateLimit(err)`, `IsAuth(err)`, `IsTransient(err)`, `IsBadRequest(err)`
- Non-SDK errors (network/connection) are classified as `ErrTransient` by default

### `internal/logger`

Logging abstraction with slog implementation:

```go
type Logger interface {
    Debug(msg string, args ...any)
    Info(msg string, args ...any)
    Warn(msg string, args ...any)
    Error(msg string, args ...any)
    With(args ...any) Logger
}
```

Features:
- Swappable implementation (slog, zerolog, zap, logrus)
- Configurable level and format (text/JSON)
- Context propagation via `With()`
- Global and per-component loggers

Configuration:
- `LOG_LEVEL` - debug, info, warn, error (default: info)
- `LOG_FORMAT` - text, json (default: text)

### `internal/version`

Build-time version information injected via ldflags:

```go
var (
    GitCommit = "unknown"  // git rev-parse HEAD
    GitDate   = "unknown"  // git log -1 --format=%ci
    GitBranch = "unknown"  // git rev-parse --abbrev-ref HEAD
    BuildDate = "unknown"  // build timestamp
)
```

Logged at startup:
```
level=INFO msg="starting TgBot" git_commit=abc123 git_date="2026-02-04" git_branch=main ...
```

### `internal/bot`

Split into two files:
- `bot.go` - Bot struct, `New`, `Run` loop, command dispatch, per-user mutex
- `handlers.go` - text/photo message handling, AI dispatch helpers, `session.Message` ↔ `ai.Message` conversion

Uses `internal/routing` for image intent detection and prefix parsing, and `internal/telegram` for Telegram API interaction (sending messages, downloading photos).

Per-request timeouts are applied via `context.WithTimeout` using the configured `REQUEST_TIMEOUT`. Prompt length is validated against `MAX_PROMPT_LENGTH` before dispatching to the AI provider.

Concurrency model:
- Semaphore (`chan struct{}`) limits concurrent handlers to `MAX_CONCURRENCY`
- Per-user `sync.Mutex` (via `sync.Map`) serializes session access per user
- `sync.WaitGroup` for graceful shutdown on context cancellation

Commands:
- `/start` - welcome message
- `/new` - clear conversation context
- `/model` - current model
- `/help` - help

Message handling:
- Text messages → normal chat, image generation, or image editing based on natural-language triggers
- Explicit image prefixes:
  - `img:` / `image:` / `фото:` - force image mode, edit latest image if available, otherwise generate
  - `edit:` / `правь:` - force image edit mode
  - `draw:` / `gen:` - force image generation mode
- Photo messages → photo analysis or image editing based on caption intent
- Reply-to-photo edit flow
- Image size extraction from prompt via `<число>x<число>` with support for `x`, `X`, `х`, `Х`
- Validation of supported image sizes before calling OpenAI
- Routing logs that explicitly show when image mode was selected
- Telegram photo upload for generated/edited images

### `internal/routing`

Image prefix parsing, natural-language intent matchers, and image size extraction. Extracted from the former `internal/bot/routing.go` into its own package so it can be used and tested independently.

Key functions:
- `ExtractImageSize(text)` - parses `<width>x<height>` patterns from text, returns cleaned text and size string
- `IsSupportedImageSize(size)` - validates against supported sizes (`1024x1024`, `1024x1536`, `1536x1024`)
- `ParseExplicitImageCommand(text)` - detects explicit prefixes (`img:`, `image:`, `фото:`, `edit:`, `правь:`, `draw:`, `gen:`) and returns the mode and prompt
- `LooksLikeImageGeneration(text)` - natural-language detection for image generation intent
- `LooksLikeImageEdit(text)` - natural-language detection for image edit intent
- `LooksLikeExplicitImageEdit(text)` - detects explicit references to editing "the image" / "the photo"
- `IsReplyToPhoto(msg)` - checks if a message is a reply to a photo

Intent matchers use `guardedPrefixes` with `rejectSuffixes` to reduce false positives on ambiguous verbs like "draw", "generate", "edit", "remove", "add", "change", "make it", "turn it", "render", "illustrate", and "replace".

### `internal/telegram`

Telegram send helpers and photo download/encoding. Extracted from the former `internal/bot/send.go` into its own package. Wraps `tgbotapi.BotAPI` via a `Client` struct.

Key functions:
- `SendText(chatID, text)` - sends a text message
- `SendLongText(chatID, text)` - splits text exceeding 4000 runes into multiple messages at natural boundaries
- `SendPhoto(chatID, imageBytes, mimeType, caption)` - sends a photo with optional caption (respects Telegram's 1024 UTF-16 unit caption limit)
- `SendChatAction(chatID, action)` - sends a chat action (e.g. "typing", "upload_photo")
- `DownloadPhoto(ctx, photos)` - downloads the highest-resolution photo and returns a base64 data URI
- `DownloadAndEncodeImage(ctx, url)` - downloads an image from URL and encodes as base64 data URI with auto-detected MIME type
- `EncodeImageDataURI(mimeType, data)` - encodes raw image bytes as a base64 data URI

### Routing Rules

Current routing in `internal/routing/routing.go` and `internal/bot/handlers.go`:

1. Text message with explicit prefix:
   - `draw:` / `gen:` -> generate image
   - `edit:` / `правь:` -> edit replied/latest image
   - `img:` / `image:` / `фото:` -> edit replied/latest image, otherwise fallback to generation
2. Reply to photo + edit-like text -> edit replied photo
3. Natural-language generation trigger -> generate image
4. Natural-language edit trigger + latest image in session -> edit latest image
5. Uploaded photo + edit-like caption -> edit uploaded photo
6. Otherwise:
   - text -> normal chat
   - photo -> photo analysis

### Image Size Handling

The bot parses image size directly from the user prompt before intent routing.

Supported forms:

- `1024x1024`
- `1024X1536`
- `1024х1536`
- `1024Х1536`

Supported values:

- `1024x1024`
- `1024x1536`
- `1536x1024`

Unsupported sizes are rejected in the bot layer with a user-friendly error instead of being passed through to OpenAI.

### Session Image Semantics

`GetLatestImage()` returns the latest image in session history regardless of role:

- user-uploaded images
- assistant-generated or assistant-edited images

This enables iterative refinement of the last visual result, not just the last uploaded photo.

## Dependencies

```go
require (
    github.com/go-telegram-bot-api/telegram-bot-api/v5 v5.5.1
    github.com/openai/openai-go v1.12.0
    github.com/joho/godotenv v1.5.1
)
```

## Build & Development

All build, test, and Docker workflows are managed through the `Makefile`:

```bash
make help           # Show all available targets
make build          # Build binary for current platform (with version ldflags)
make build-linux    # Cross-compile for linux/amd64
make test           # Run tests with -race
make lint           # go vet + golangci-lint
make fmt            # Format code (gofmt -s -w)
make cover          # Tests + coverage report (HTML + 60% threshold gate)
make docker-build   # Build Docker image (requires DOCKER_USER)
make docker-push    # Build and push Docker image (requires DOCKER_USER)
make clean          # Remove build artifacts
```

CI pipeline (`.github/workflows/ci.yml`) runs on push and pull requests to `main`/`master`:
- **Lint** job: `go vet` + `golangci-lint`
- **Test** job: tests with `-race`, coverage report, 60% coverage threshold gate

Quick start:

```bash
cp .env.example .env   # Create config, fill in tokens
make build             # Build binary
./tgbot                # Run
```

For detailed deployment instructions, see **[DEPLOYMENT.md](DEPLOYMENT.md)**.

## Extension

### Adding New AI Provider

1. Create file `internal/ai/newprovider.go`
2. Implement `Provider` interface:

```go
type MyProvider struct {
    client *myclient.Client
    model  string
}

func (p *MyProvider) Respond(ctx context.Context, req ai.Request) (ai.Result, error) {
    // Convert Request and call provider API
}

func (p *MyProvider) ModelName() string {
    return p.model
}
```

3. Add configuration to `internal/config/config.go`
4. Register in `cmd/bot/main.go`

### Persistent Sessions

Replace in-memory `session.Manager` with DB implementation:

```go
type DBManager struct {
    db *sql.DB
}

func (m *DBManager) Get(userID int64) []Message {
    // SELECT from database
}

func (m *DBManager) Add(userID int64, messages ...Message) {
    // INSERT into database
}
```

Options:
- SQLite for simplicity
- PostgreSQL for scaling
- Redis for performance

## Security

- Tokens stored only in env vars, never in code
- `.env` added to `.gitignore`
- Whitelist prevents unauthorized access
- Access attempt logging for audit
- Service runs as dedicated user (non-root)

## Coding Conventions

| Rule | Description |
|------|-------------|
| Indentation | 4 spaces, no tabs |
| Language | All comments and messages in code in English |
| README.md | Always present in project root |
| Structure | Standard Go layout (cmd/, internal/) |
| Secrets | Only via env vars, never hardcoded |
| Thread safety | Mutex for shared state |
| AI providers | Implement `ai.Provider` interface |
