# TgBot

Telegram bot providing access to AI through the OpenAI Responses API.

## Features

- Text chat with conversation continuity
- Photo analysis with multimodal input
- Natural-language image generation
- Natural-language image editing from:
  - a replied photo
  - an uploaded photo with caption
  - the latest image stored in the current session
- Whitelist-based access control
- In-memory conversation context with stored `previous_response_id`
- Docker-ready deployment

## Quick Start

```bash
# 1. Copy and fill configuration
cp .env.example .env

# 2. Build and run
make build
./tgbot

# Or run directly
go run ./cmd/bot

# Or with Docker
docker-compose up -d
```

## Development

The project uses a `Makefile` for all build, test, and Docker workflows:

```bash
make help           # Show all available targets
make build          # Build binary for current platform
make build-linux    # Cross-compile for linux/amd64
make test           # Run tests with race detector
make lint           # Run go vet + golangci-lint
make fmt            # Format code with gofmt
make cover          # Tests + coverage report (HTML + threshold gate)
make docker-build   # Build Docker image (DOCKER_USER required)
make docker-push    # Build and push Docker image (DOCKER_USER required)
make clean          # Remove build artifacts
```

Docker targets require `DOCKER_USER`:

```bash
make docker-build DOCKER_USER=myuser
make docker-push  DOCKER_USER=myuser DOCKER_TAG=v1.0.0
```

## Configuration

Set environment variables in `.env`:

| Variable | Required | Description |
|----------|----------|-------------|
| `TELEGRAM_BOT_TOKEN` | Yes | Bot token from @BotFather |
| `OPENAI_API_KEY` | Yes | OpenAI API key |
| `OPENAI_MODEL` | No | Responses API model for chat and intent orchestration (default: `gpt-4o`) |
| `OPENAI_BASE_URL` | No | Custom OpenAI-compatible base URL |
| `ALLOWED_USERS` | No | Comma-separated user IDs |
| `MAX_HISTORY` | No | Max messages in context (default: 20) |
| `SESSION_TTL` | No | Idle session lifetime, Go duration string (default: `24h`) |
| `MAX_CONCURRENCY` | No | Max concurrent message handlers (default: 20) |
| `OPENAI_MAX_RETRIES` | No | Max automatic retries for transient API errors (default: 3) |
| `REQUEST_TIMEOUT` | No | Per-request timeout, Go duration (default: `60s`) |
| `MAX_PROMPT_LENGTH` | No | Max prompt length in characters (default: 4000) |
| `LOG_LEVEL` | No | Logging level (`debug`, `info`, `warn`, `error`) |
| `LOG_FORMAT` | No | Log format (`text`, `json`) |

## Usage

The bot keeps normal text chat behavior and photo analysis, and also routes image workflows by natural-language triggers.

Examples:

- Text chat: `Explain how mutexes work in Go`
- Photo analysis: send a photo with caption `What is shown here?`
- Image generation: `Draw a neon cyberpunk poster for a cafe`
- Image editing from uploaded photo: send a photo with caption `Remove the background and make it look like a sticker`
- Image editing from reply: reply to a photo with `Make it more realistic`
- Image editing from session context: `Change the latest image to a watercolor style`
- Forced image mode: `img: add sharp teeth`
- Forced image edit mode: `правь: убери фон`
- Size control: `draw: hedgehog poster 1024x1536`
- Size control with edit: `фото: добавь фон 1536х1024`

Supported image sizes:

- `1024x1024`
- `1024x1536`
- `1536x1024`

The bot detects size patterns like `1024x1024`, `1024X1536`, `1024х1536`, and `1024Х1536`.

## OpenAI SDK

The project uses:

- `github.com/openai/openai-go`
- `Responses API` for chat, vision, image generation, and image editing
- built-in `image_generation` tool backed by `gpt-image-1`

`OPENAI_MODEL` stays user-controlled. Pick a model that supports the Responses API and hosted tool workflows.

## Bot Commands

- `/start` - Welcome message
- `/new` - Clear conversation context
- `/model` - Show current model
- `/help` - Help

## Documentation

- [ARCHITECTURE.md](ARCHITECTURE.md) - Architecture, modules, extension guide
- [DEPLOYMENT.md](DEPLOYMENT.md) - Build and deployment to VPS
- `make help` - List all Makefile targets

## License

MIT
