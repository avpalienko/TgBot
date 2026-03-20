package bot

import (
	"context"
	"fmt"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/user/tgbot/internal/ai"
	"github.com/user/tgbot/internal/auth"
	"github.com/user/tgbot/internal/logger"
	"github.com/user/tgbot/internal/session"
	"github.com/user/tgbot/internal/telegram"
)

// Bot handles Telegram bot operations.
type Bot struct {
	tg              *telegram.Client
	ai              ai.Provider
	sessions        *session.Manager
	auth            *auth.Whitelist
	log             logger.Logger
	sem             chan struct{}
	wg              sync.WaitGroup
	userMu          sync.Map // per-user *sync.Mutex to serialize session access
	requestTimeout  time.Duration
	maxPromptLength int
}

func (b *Bot) lockUser(userID int64) func() {
	v, _ := b.userMu.LoadOrStore(userID, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// New creates a new Bot instance.
func New(token string, aiProvider ai.Provider, sessions *session.Manager, whitelist *auth.Whitelist, log logger.Logger, maxConcurrency int, requestTimeout time.Duration, maxPromptLength int) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}

	log.Info("authorized", "username", api.Self.UserName)

	if maxConcurrency <= 0 {
		maxConcurrency = 20
	}
	if requestTimeout <= 0 {
		requestTimeout = 60 * time.Second
	}
	if maxPromptLength <= 0 {
		maxPromptLength = 4000
	}

	return &Bot{
		tg:              telegram.NewClient(api, log),
		ai:              aiProvider,
		sessions:        sessions,
		auth:            whitelist,
		log:             log,
		sem:             make(chan struct{}, maxConcurrency),
		requestTimeout:  requestTimeout,
		maxPromptLength: maxPromptLength,
	}, nil
}

// Run starts the bot and processes updates.
func (b *Bot) Run(ctx context.Context) error {
	b.setCommands()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.tg.API().GetUpdatesChan(u)

	b.log.Info("started listening for updates")

	for {
		select {
		case <-ctx.Done():
			b.log.Info("shutting down, waiting for in-flight handlers")
			b.wg.Wait()
			b.log.Info("all handlers finished")
			return ctx.Err()
		case update := <-updates:
			if update.Message == nil {
				continue
			}
			select {
			case b.sem <- struct{}{}:
				b.wg.Add(1)
				go func(msg *tgbotapi.Message) {
					defer b.wg.Done()
					defer func() { <-b.sem }()
					b.handleMessage(ctx, msg)
				}(update.Message)
			case <-ctx.Done():
				b.log.Info("shutting down, waiting for in-flight handlers")
				b.wg.Wait()
				b.log.Info("all handlers finished")
				return ctx.Err()
			}
		}
	}
}

func (b *Bot) setCommands() {
	commands := tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: "start", Description: "Start the bot"},
		tgbotapi.BotCommand{Command: "new", Description: "Start a new conversation"},
		tgbotapi.BotCommand{Command: "model", Description: "Show current model"},
		tgbotapi.BotCommand{Command: "help", Description: "Help"},
	)

	if _, err := b.tg.API().Request(commands); err != nil {
		b.log.Error("failed to set bot commands", "error", err)
	}
}

func (b *Bot) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
	ctx, cancel := context.WithTimeout(ctx, b.requestTimeout)
	defer cancel()

	userID := msg.From.ID
	chatID := msg.Chat.ID

	sessionID := b.sessions.GetSessionID(userID)

	log := b.log.With(
		"user_id", userID,
		"session_id", sessionID,
		"username", msg.From.UserName,
	)

	if !b.auth.IsAllowed(userID) {
		log.Warn("access denied")
		b.tg.SendText(chatID, "Access denied. Contact the administrator.")
		return
	}

	unlock := b.lockUser(userID)
	defer unlock()

	if msg.IsCommand() {
		b.handleCommand(ctx, msg, log)
		return
	}

	if len(msg.Photo) > 0 {
		b.handlePhotoMessage(ctx, msg, log)
		return
	}

	if msg.Text != "" {
		b.handleTextMessage(ctx, msg, log)
	}
}

func (b *Bot) handleCommand(ctx context.Context, msg *tgbotapi.Message, log logger.Logger) {
	cmd := msg.Command()
	log.Debug("command received", "command", cmd)

	switch cmd {
	case "start":
		b.cmdStart(msg, log)
	case "new":
		b.cmdNew(msg, log)
	case "model":
		b.cmdModel(msg)
	case "help":
		b.cmdHelp(msg)
	default:
		b.tg.SendText(msg.Chat.ID, "Unknown command. Use /help for available commands.")
	}
}

func (b *Bot) cmdStart(msg *tgbotapi.Message, log logger.Logger) {
	log.Info("user started bot")

	text := fmt.Sprintf(`Hello, %s!

I'm an AI assistant for chat, photo analysis, image generation, and image editing.

Commands:
/new - start a new conversation
/model - show current model
/help - help

Examples:
- Send any text to chat
- Send a photo to analyze it
- Ask "Draw a neon cyberpunk poster"
- Reply to a photo with "Remove the background and make it look like a sticker"
- Use "img: add sharp teeth" to force image mode
- Add a size like "1024x1024" or "1536x1024"`, msg.From.FirstName)

	b.tg.SendText(msg.Chat.ID, text)
}

func (b *Bot) cmdNew(msg *tgbotapi.Message, log logger.Logger) {
	newSessionID := b.sessions.Clear(msg.From.ID)
	log.Info("session cleared", "new_session_id", newSessionID)
	b.tg.SendText(msg.Chat.ID, "Conversation cleared. Let's start fresh!")
}

func (b *Bot) cmdModel(msg *tgbotapi.Message) {
	text := fmt.Sprintf("Current model: %s", b.ai.ModelName())
	b.tg.SendText(msg.Chat.ID, text)
}

func (b *Bot) cmdHelp(msg *tgbotapi.Message) {
	text := `Help

You can use the bot in four ways:
- Normal chat: send any text message
- Photo analysis: send a photo, optionally with a caption
- Image generation: ask naturally, for example "Draw a neon cyberpunk poster"
- Image editing:
  reply to a photo with an edit request
  send a photo with an edit caption
  ask to edit the latest image in the conversation

Explicit prefixes:
- "img: <prompt>" -> force image mode; edits latest image if available, otherwise generates a new one
- "edit: <prompt>" -> force image edit mode
- "фото: <prompt>" -> same as img:
- "правь: <prompt>" -> same as edit:
- "draw: <prompt>" or "gen: <prompt>" -> force image generation mode

Supported image sizes:
- 1024x1024
- 1024x1536
- 1536x1024
- You can place the size anywhere in the prompt, for example: "draw: hedgehog 1024x1536"

Commands:
/new - start a new conversation (clear context)
/model - show current AI model
/help - this help

Conversation context is preserved between messages. Use /new to start fresh.`

	b.tg.SendText(msg.Chat.ID, text)
}
