# TgBot

Telegram bot providing access to AI via OpenAI-compatible API.

## Features

- OpenAI API integration (GPT-4o, GPT-5, etc.)
- Whitelist-based access control
- Conversation context (in-memory)
- Docker-ready deployment

## Quick Start

```bash
# 1. Copy and fill configuration
cp .env.example .env

# 2. Run
go run ./cmd/bot

# Or with Docker
docker-compose up -d
```

## Configuration

Set environment variables in `.env`:

| Variable | Required | Description |
|----------|----------|-------------|
| `TELEGRAM_BOT_TOKEN` | Yes | Bot token from @BotFather |
| `OPENAI_API_KEY` | Yes | OpenAI API key |
| `OPENAI_MODEL` | No | Model name (default: gpt-4o) |
| `ALLOWED_USERS` | No | Comma-separated user IDs |
| `MAX_HISTORY` | No | Max messages in context (default: 20) |

## Bot Commands

- `/start` - Welcome message
- `/new` - Clear conversation context
- `/model` - Show current model
- `/help` - Help

## Documentation

- [ARCHITECTURE.md](ARCHITECTURE.md) - Architecture, modules, extension guide
- [DEPLOYMENT.md](DEPLOYMENT.md) - Build and deployment to VPS

## License

MIT
