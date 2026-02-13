package bot

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/user/tgbot/internal/ai"
	"github.com/user/tgbot/internal/auth"
	"github.com/user/tgbot/internal/logger"
	"github.com/user/tgbot/internal/session"
)

// Bot handles Telegram bot operations
type Bot struct {
	api      *tgbotapi.BotAPI
	ai       ai.Provider
	sessions *session.Manager
	auth     *auth.Whitelist
	log      logger.Logger
}

// New creates a new Bot instance
func New(token string, aiProvider ai.Provider, sessions *session.Manager, whitelist *auth.Whitelist, log logger.Logger) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}

	log.Info("authorized", "username", api.Self.UserName)

	return &Bot{
		api:      api,
		ai:       aiProvider,
		sessions: sessions,
		auth:     whitelist,
		log:      log,
	}, nil
}

// Run starts the bot and processes updates
func (b *Bot) Run(ctx context.Context) error {
	b.setCommands()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	b.log.Info("started listening for updates")

	for {
		select {
		case <-ctx.Done():
			b.log.Info("shutting down")
			return ctx.Err()
		case update := <-updates:
			if update.Message == nil {
				continue
			}
			go b.handleMessage(ctx, update.Message)
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

	if _, err := b.api.Request(commands); err != nil {
		b.log.Error("failed to set bot commands", "error", err)
	}
}

func (b *Bot) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
	userID := msg.From.ID
	chatID := msg.Chat.ID

	// Get or create session ID for this user
	sessionID := b.sessions.GetSessionID(userID)

	// Create logger with session context
	log := b.log.With(
		"user_id", userID,
		"session_id", sessionID,
		"username", msg.From.UserName,
	)

	// Check authorization
	if !b.auth.IsAllowed(userID) {
		log.Warn("access denied")
		b.sendText(chatID, "Access denied. Contact the administrator.")
		return
	}

	// Handle commands
	if msg.IsCommand() {
		b.handleCommand(ctx, msg, log)
		return
	}

	// Handle photo messages
	if len(msg.Photo) > 0 {
		b.handlePhotoMessage(ctx, msg, log)
		return
	}

	// Handle text messages
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
		b.sendText(msg.Chat.ID, "Unknown command. Use /help for available commands.")
	}
}

func (b *Bot) cmdStart(msg *tgbotapi.Message, log logger.Logger) {
	log.Info("user started bot")

	text := fmt.Sprintf(`Hello, %s!

I'm an AI assistant. Just send me a message and I'll respond.

Commands:
/new - start a new conversation
/model - show current model
/help - help`, msg.From.FirstName)

	b.sendText(msg.Chat.ID, text)
}

func (b *Bot) cmdNew(msg *tgbotapi.Message, log logger.Logger) {
	newSessionID := b.sessions.Clear(msg.From.ID)
	log.Info("session cleared", "new_session_id", newSessionID)
	b.sendText(msg.Chat.ID, "Conversation cleared. Let's start fresh!")
}

func (b *Bot) cmdModel(msg *tgbotapi.Message) {
	text := fmt.Sprintf("Current model: %s", b.ai.ModelName())
	b.sendText(msg.Chat.ID, text)
}

func (b *Bot) cmdHelp(msg *tgbotapi.Message) {
	text := `Help

Just send a text message to get a response from AI.

Commands:
/new - start a new conversation (clear context)
/model - show current AI model
/help - this help

Conversation context is preserved between messages. Use /new to start fresh.`

	b.sendText(msg.Chat.ID, text)
}

func (b *Bot) handleTextMessage(ctx context.Context, msg *tgbotapi.Message, log logger.Logger) {
	userID := msg.From.ID
	chatID := msg.Chat.ID

	log.Info("message received", "text_length", len(msg.Text))

	// Send typing indicator
	typing := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	b.api.Send(typing)

	// Get conversation history
	history := b.sessions.Get(userID)
	log.Debug("loaded history", "messages_count", len(history))

	// Add user message
	userMsg := session.Message{Role: "user", Content: msg.Text}
	messages := append(history, userMsg)

	// Call AI
	log.Debug("calling AI", "model", b.ai.ModelName(), "messages_count", len(messages))

	response, err := b.ai.Complete(ctx, messages)
	if err != nil {
		log.Error("AI request failed", "error", err)
		b.sendText(chatID, "An error occurred while processing your request. Please try again later.")
		return
	}

	log.Info("AI response received", "response_length", len(response))

	// Save messages to session
	assistantMsg := session.Message{Role: "assistant", Content: response}
	b.sessions.Add(userID, userMsg, assistantMsg)

	// Send response (split if too long)
	b.sendLongText(chatID, response)
}

func (b *Bot) handlePhotoMessage(ctx context.Context, msg *tgbotapi.Message, log logger.Logger) {
	userID := msg.From.ID
	chatID := msg.Chat.ID

	log.Info("photo received")

	// Send typing indicator
	typing := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	b.api.Send(typing)

	// Pick the largest photo (last in the array)
	photo := msg.Photo[len(msg.Photo)-1]

	// Get direct URL for the file
	fileURL, err := b.api.GetFileDirectURL(photo.FileID)
	if err != nil {
		log.Error("failed to get file URL", "error", err)
		b.sendText(chatID, "Failed to process the image. Please try again.")
		return
	}

	// Download and encode as base64 data URI
	dataURI, err := downloadAndEncodeImage(fileURL)
	if err != nil {
		log.Error("failed to download image", "error", err)
		b.sendText(chatID, "Failed to download the image. Please try again.")
		return
	}

	// Use caption as text content (may be empty)
	caption := msg.Caption

	// Get conversation history
	history := b.sessions.Get(userID)
	log.Debug("loaded history", "messages_count", len(history))

	// Add user message with image
	userMsg := session.Message{Role: "user", Content: caption, ImageData: dataURI}
	messages := append(history, userMsg)

	// Call AI
	log.Debug("calling AI", "model", b.ai.ModelName(), "messages_count", len(messages))

	response, err := b.ai.Complete(ctx, messages)
	if err != nil {
		log.Error("AI request failed", "error", err)
		b.sendText(chatID, "An error occurred while processing your request. Please try again later.")
		return
	}

	log.Info("AI response received", "response_length", len(response))

	// Save messages to session
	assistantMsg := session.Message{Role: "assistant", Content: response}
	b.sessions.Add(userID, userMsg, assistantMsg)

	// Send response (split if too long)
	b.sendLongText(chatID, response)
}

func downloadAndEncodeImage(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read image data: %w", err)
	}

	// Detect content type from file content (magic bytes), not from header
	// Telegram often returns application/octet-stream which OpenAI rejects
	contentType := http.DetectContentType(data)
	if !strings.HasPrefix(contentType, "image/") {
		contentType = "image/jpeg"
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	dataURI := fmt.Sprintf("data:%s;base64,%s", contentType, encoded)

	return dataURI, nil
}

func (b *Bot) sendText(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.api.Send(msg); err != nil {
		b.log.Error("failed to send message", "chat_id", chatID, "error", err)
	}
}

func (b *Bot) sendLongText(chatID int64, text string) {
	// Telegram message limit is 4096 characters
	const maxLen = 4000

	if len(text) <= maxLen {
		b.sendText(chatID, text)
		return
	}

	// Split by paragraphs or sentences
	parts := splitText(text, maxLen)
	for _, part := range parts {
		b.sendText(chatID, part)
	}
}

func splitText(text string, maxLen int) []string {
	var parts []string

	for len(text) > 0 {
		if len(text) <= maxLen {
			parts = append(parts, text)
			break
		}

		// Try to split at paragraph
		splitIdx := strings.LastIndex(text[:maxLen], "\n\n")
		if splitIdx == -1 {
			// Try to split at newline
			splitIdx = strings.LastIndex(text[:maxLen], "\n")
		}
		if splitIdx == -1 {
			// Try to split at sentence
			splitIdx = strings.LastIndex(text[:maxLen], ". ")
		}
		if splitIdx == -1 {
			// Force split at maxLen
			splitIdx = maxLen - 1
		}

		parts = append(parts, text[:splitIdx+1])
		text = strings.TrimSpace(text[splitIdx+1:])
	}

	return parts
}
