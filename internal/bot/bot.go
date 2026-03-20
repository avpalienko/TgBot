package bot

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/user/tgbot/internal/ai"
	"github.com/user/tgbot/internal/auth"
	"github.com/user/tgbot/internal/logger"
	"github.com/user/tgbot/internal/session"
)

var imageSizePattern = regexp.MustCompile(`(?i)(\d{3,4})\s*[xх]\s*(\d{3,4})`)

// Bot handles Telegram bot operations.
type Bot struct {
	api      *tgbotapi.BotAPI
	ai       ai.Provider
	sessions *session.Manager
	auth     *auth.Whitelist
	log      logger.Logger
	sem      chan struct{}
	userMu   sync.Map // per-user *sync.Mutex to serialize session access
}

func (b *Bot) lockUser(userID int64) func() {
	v, _ := b.userMu.LoadOrStore(userID, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// New creates a new Bot instance.
func New(token string, aiProvider ai.Provider, sessions *session.Manager, whitelist *auth.Whitelist, log logger.Logger, maxConcurrency int) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}

	log.Info("authorized", "username", api.Self.UserName)

	if maxConcurrency <= 0 {
		maxConcurrency = 20
	}

	return &Bot{
		api:      api,
		ai:       aiProvider,
		sessions: sessions,
		auth:     whitelist,
		log:      log,
		sem:      make(chan struct{}, maxConcurrency),
	}, nil
}

// Run starts the bot and processes updates.
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
			select {
			case b.sem <- struct{}{}:
				go func(msg *tgbotapi.Message) {
					defer func() { <-b.sem }()
					b.handleMessage(ctx, msg)
				}(update.Message)
			case <-ctx.Done():
				b.log.Info("shutting down")
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

	if _, err := b.api.Request(commands); err != nil {
		b.log.Error("failed to set bot commands", "error", err)
	}
}

func (b *Bot) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
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
		b.sendText(chatID, "Access denied. Contact the administrator.")
		return
	}

	if msg.IsCommand() {
		b.handleCommand(ctx, msg, log)
		return
	}

	unlock := b.lockUser(userID)
	defer unlock()

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
		b.sendText(msg.Chat.ID, "Unknown command. Use /help for available commands.")
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

	b.sendText(msg.Chat.ID, text)
}

func (b *Bot) handleTextMessage(ctx context.Context, msg *tgbotapi.Message, log logger.Logger) {
	userID := msg.From.ID
	chatID := msg.Chat.ID
	text := strings.TrimSpace(msg.Text)
	imagePrompt, imageSize := extractImageSize(text)

	log.Info("message received", "text_length", len(text))

	b.sendChatAction(chatID, tgbotapi.ChatTyping)

	if b.handleExplicitTextImageCommand(ctx, msg, log, text, imagePrompt, imageSize) {
		return
	}

	if looksLikeImageEdit(imagePrompt) && isReplyToPhoto(msg) {
		if !b.ensureSupportedImageSize(chatID, log, imageSize) {
			return
		}
		imageData, err := b.downloadTelegramPhoto(msg.ReplyToMessage.Photo)
		if err != nil {
			log.Error("failed to load reply photo", "error", err)
			b.sendText(chatID, "Failed to load the replied photo. Please try again.")
			return
		}
		b.dispatchImageEdit(ctx, userID, chatID, log, "reply_text_edit_intent", "reply_photo", imagePrompt, imageSize, imageData, session.Message{Role: "user", Content: text, ImageData: imageData})
		return
	}

	if looksLikeImageGeneration(imagePrompt) {
		if !b.ensureSupportedImageSize(chatID, log, imageSize) {
			return
		}
		b.dispatchImageGeneration(ctx, userID, chatID, log, "text_generation_intent", imagePrompt, imageSize, session.Message{Role: "user", Content: text})
		return
	}

	if looksLikeExplicitImageEdit(imagePrompt) {
		if latestImage := b.sessions.GetLatestImage(userID); latestImage != "" {
			if !b.ensureSupportedImageSize(chatID, log, imageSize) {
				return
			}
			b.dispatchImageEdit(ctx, userID, chatID, log, "text_edit_intent", "latest_session_image", imagePrompt, imageSize, latestImage, session.Message{Role: "user", Content: text, ImageData: latestImage})
			return
		}
	}

	log.Debug("using chat mode", "trigger", "default_text")
	b.handleChatRequest(ctx, userID, chatID, log, session.Message{Role: "user", Content: text})
}

func (b *Bot) handlePhotoMessage(ctx context.Context, msg *tgbotapi.Message, log logger.Logger) {
	userID := msg.From.ID
	chatID := msg.Chat.ID
	caption := strings.TrimSpace(msg.Caption)
	imagePrompt, imageSize := extractImageSize(caption)

	log.Info("photo received", "caption_length", len(caption))

	b.sendChatAction(chatID, tgbotapi.ChatTyping)

	imageData, err := b.downloadTelegramPhoto(msg.Photo)
	if err != nil {
		log.Error("failed to process image", "error", err)
		b.sendText(chatID, "Failed to process the image. Please try again.")
		return
	}

	userMsg := session.Message{Role: "user", Content: caption, ImageData: imageData}
	if b.handleExplicitPhotoImageCommand(ctx, userID, chatID, log, userMsg, imageData, imagePrompt, imageSize) {
		return
	}

	if looksLikeImageEdit(imagePrompt) {
		if !b.ensureSupportedImageSize(chatID, log, imageSize) {
			return
		}
		b.dispatchImageEdit(ctx, userID, chatID, log, "photo_caption_edit_intent", "uploaded_photo", imagePrompt, imageSize, imageData, userMsg)
		return
	}

	log.Debug("using chat mode", "trigger", "photo_analysis")
	b.handleChatRequest(ctx, userID, chatID, log, userMsg)
}

func (b *Bot) logImageModeSelected(log logger.Logger, mode ai.RequestMode, trigger, imageSource, imageSize string) {
	log.Info("image mode selected", "mode", mode, "trigger", trigger, "image_source", imageSource, "image_size", imageSize)
}

func (b *Bot) handleExplicitTextImageCommand(ctx context.Context, msg *tgbotapi.Message, log logger.Logger, originalText, imagePrompt, imageSize string) bool {
	explicit, prompt, imageSize, ok := b.prepareExplicitImageCommand(msg.Chat.ID, log, imagePrompt, imageSize)
	if !ok {
		return false
	}

	userID := msg.From.ID
	chatID := msg.Chat.ID

	switch explicit.Mode {
	case ai.ModeGenerateImage:
		b.dispatchImageGeneration(ctx, userID, chatID, log, "explicit_prefix", prompt, imageSize, session.Message{Role: "user", Content: originalText})
		return true
	case ai.ModeEditImage:
		if isReplyToPhoto(msg) {
			imageData, err := b.downloadTelegramPhoto(msg.ReplyToMessage.Photo)
			if err != nil {
				log.Error("failed to load reply photo", "error", err)
				b.sendText(chatID, "Failed to load the replied photo. Please try again.")
				return true
			}

			b.dispatchImageEdit(ctx, userID, chatID, log, "explicit_prefix", "reply_photo", prompt, imageSize, imageData, session.Message{Role: "user", Content: originalText, ImageData: imageData})
			return true
		}

		if latestImage := b.sessions.GetLatestImage(userID); latestImage != "" {
			b.dispatchImageEdit(ctx, userID, chatID, log, "explicit_prefix", "latest_session_image", prompt, imageSize, latestImage, session.Message{Role: "user", Content: originalText, ImageData: latestImage})
			return true
		}

		if explicit.FallbackToGenerate {
			b.dispatchImageGeneration(ctx, userID, chatID, log, "explicit_prefix_fallback", prompt, imageSize, session.Message{Role: "user", Content: originalText})
			return true
		}

		log.Info("image mode requested but no image source available", "mode", ai.ModeEditImage, "trigger", "explicit_prefix", "image_size", imageSize)
		b.sendText(chatID, "No image found to edit. Reply to a photo, send a photo with a caption, or use `img:` after generating an image first.")
		return true
	default:
		return false
	}
}

func (b *Bot) handleExplicitPhotoImageCommand(ctx context.Context, userID, chatID int64, log logger.Logger, userMsg session.Message, imageData, imagePrompt, imageSize string) bool {
	explicit, prompt, imageSize, ok := b.prepareExplicitImageCommand(chatID, log, imagePrompt, imageSize)
	if !ok {
		return false
	}

	switch explicit.Mode {
	case ai.ModeGenerateImage:
		b.dispatchImageGeneration(ctx, userID, chatID, log, "photo_caption_explicit_prefix", prompt, imageSize, userMsg)
		return true
	case ai.ModeEditImage:
		b.dispatchImageEdit(ctx, userID, chatID, log, "photo_caption_explicit_prefix", "uploaded_photo", prompt, imageSize, imageData, userMsg)
		return true
	default:
		return false
	}
}

func (b *Bot) prepareExplicitImageCommand(chatID int64, log logger.Logger, imagePrompt, imageSize string) (explicitImageCommand, string, string, bool) {
	explicit, ok := parseExplicitImageCommand(imagePrompt)
	if !ok {
		return explicitImageCommand{}, "", imageSize, false
	}

	prompt := explicit.Prompt
	if parsedPrompt, parsedSize := extractImageSize(prompt); parsedSize != "" {
		prompt = parsedPrompt
		imageSize = parsedSize
	}

	if !b.ensureSupportedImageSize(chatID, log, imageSize) {
		return explicitImageCommand{}, "", imageSize, false
	}

	return explicit, prompt, imageSize, true
}

func (b *Bot) dispatchImageGeneration(ctx context.Context, userID, chatID int64, log logger.Logger, trigger, prompt, imageSize string, userMsg session.Message) {
	b.logImageModeSelected(log, ai.ModeGenerateImage, trigger, "none", imageSize)
	b.handleNonChatRequest(ctx, userID, chatID, log, ai.Request{
		Mode:               ai.ModeGenerateImage,
		Text:               prompt,
		ImageSize:          imageSize,
		PreviousResponseID: b.sessions.GetPreviousResponseID(userID),
	}, userMsg)
}

func (b *Bot) dispatchImageEdit(ctx context.Context, userID, chatID int64, log logger.Logger, trigger, imageSource, prompt, imageSize, imageData string, userMsg session.Message) {
	b.logImageModeSelected(log, ai.ModeEditImage, trigger, imageSource, imageSize)
	b.handleNonChatRequest(ctx, userID, chatID, log, ai.Request{
		Mode:               ai.ModeEditImage,
		Text:               prompt,
		ImageSize:          imageSize,
		InputImageData:     imageData,
		PreviousResponseID: b.sessions.GetPreviousResponseID(userID),
	}, userMsg)
}

func (b *Bot) ensureSupportedImageSize(chatID int64, log logger.Logger, imageSize string) bool {
	if isSupportedImageSize(imageSize) {
		return true
	}

	log.Warn("unsupported image size requested", "image_size", imageSize)
	b.sendText(chatID, "Unsupported image size. Use one of: 1024x1024, 1024x1536, 1536x1024.")
	return false
}

func (b *Bot) handleChatRequest(ctx context.Context, userID, chatID int64, log logger.Logger, userMsg session.Message) {
	history := b.sessions.Get(userID)
	messages := append(history, userMsg)

	log.Debug("calling AI chat", "model", b.ai.ModelName(), "messages_count", len(messages))

	result, err := b.ai.Respond(ctx, ai.Request{
		Mode:               ai.ModeChat,
		History:            messages,
		PreviousResponseID: b.sessions.GetPreviousResponseID(userID),
	})
	if err != nil {
		log.Error("AI chat request failed", "error", err)
		b.sendText(chatID, "An error occurred while processing your request. Please try again later.")
		return
	}

	log.Info("AI chat response received", "response_length", len(result.Text))
	b.persistResult(userID, userMsg, result)
	b.sendResult(chatID, result)
}

func (b *Bot) handleNonChatRequest(ctx context.Context, userID, chatID int64, log logger.Logger, req ai.Request, userMsg session.Message) {
	log.Debug("calling AI image workflow", "model", b.ai.ModelName(), "mode", req.Mode)

	result, err := b.ai.Respond(ctx, req)
	if err != nil {
		log.Error("AI image request failed", "error", err, "mode", req.Mode)
		b.sendText(chatID, "An error occurred while processing your request. Please try again later.")
		return
	}

	log.Info("AI image response received", "has_text", result.Text != "", "has_image", len(result.ImageBytes) > 0, "mode", req.Mode)
	b.persistResult(userID, userMsg, result)
	b.sendResult(chatID, result)
}

func (b *Bot) persistResult(userID int64, userMsg session.Message, result ai.Result) {
	assistantMsg := session.Message{
		Role:    "assistant",
		Content: result.Text,
	}
	if len(result.ImageBytes) > 0 {
		assistantMsg.ImageData = encodeImageDataURI(result.ImageMimeType, result.ImageBytes)
	}

	b.sessions.AddWithResponseID(userID, result.ResponseID, userMsg, assistantMsg)
}

func (b *Bot) sendResult(chatID int64, result ai.Result) {
	if len(result.ImageBytes) > 0 {
		b.sendPhoto(chatID, result.ImageBytes, result.ImageMimeType, result.Text)
		return
	}

	if result.Text != "" {
		b.sendLongText(chatID, result.Text)
	}
}

func (b *Bot) sendPhoto(chatID int64, imageBytes []byte, mimeType, caption string) {
	msg := tgbotapi.NewPhoto(chatID, tgbotapi.FileBytes{
		Name:  filenameForMimeType(mimeType),
		Bytes: imageBytes,
	})
	if caption != "" && len([]rune(caption)) <= 1024 {
		msg.Caption = caption
	}

	if _, err := b.api.Send(msg); err != nil {
		b.log.Error("failed to send photo", "chat_id", chatID, "error", err)
		return
	}

	if caption != "" && len([]rune(caption)) > 1024 {
		b.sendLongText(chatID, caption)
	}
}

func (b *Bot) sendChatAction(chatID int64, action string) {
	chatAction := tgbotapi.NewChatAction(chatID, action)
	if _, err := b.api.Request(chatAction); err != nil {
		b.log.Error("failed to send chat action", "chat_id", chatID, "error", err)
	}
}

func (b *Bot) downloadTelegramPhoto(photos []tgbotapi.PhotoSize) (string, error) {
	if len(photos) == 0 {
		return "", fmt.Errorf("message does not contain a photo")
	}

	photo := photos[len(photos)-1]
	fileURL, err := b.api.GetFileDirectURL(photo.FileID)
	if err != nil {
		return "", fmt.Errorf("failed to get file URL: %w", err)
	}

	return downloadAndEncodeImage(fileURL)
}

func isReplyToPhoto(msg *tgbotapi.Message) bool {
	return msg.ReplyToMessage != nil && len(msg.ReplyToMessage.Photo) > 0
}

func extractImageSize(text string) (string, string) {
	if text == "" {
		return "", ""
	}

	match := imageSizePattern.FindStringSubmatchIndex(text)
	if match == nil {
		return strings.TrimSpace(text), ""
	}

	width := text[match[2]:match[3]]
	height := text[match[4]:match[5]]
	size := width + "x" + height

	cleaned := strings.TrimSpace(text[:match[0]] + " " + text[match[1]:])
	cleaned = strings.Join(strings.Fields(cleaned), " ")

	return cleaned, size
}

func isSupportedImageSize(size string) bool {
	switch size {
	case "", "1024x1024", "1024x1536", "1536x1024":
		return true
	default:
		return false
	}
}

type explicitImageCommand struct {
	Mode               ai.RequestMode
	Prompt             string
	FallbackToGenerate bool
}

func parseExplicitImageCommand(text string) (explicitImageCommand, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return explicitImageCommand{}, false
	}

	prefixes := []struct {
		prefix             string
		mode               ai.RequestMode
		fallbackToGenerate bool
	}{
		{prefix: "img:", mode: ai.ModeEditImage, fallbackToGenerate: true},
		{prefix: "image:", mode: ai.ModeEditImage, fallbackToGenerate: true},
		{prefix: "фото:", mode: ai.ModeEditImage, fallbackToGenerate: true},
		{prefix: "edit:", mode: ai.ModeEditImage},
		{prefix: "правь:", mode: ai.ModeEditImage},
		{prefix: "draw:", mode: ai.ModeGenerateImage},
		{prefix: "gen:", mode: ai.ModeGenerateImage},
	}

	lower := strings.ToLower(trimmed)
	for _, candidate := range prefixes {
		if strings.HasPrefix(lower, candidate.prefix) {
			prompt := strings.TrimSpace(trimmed[len(candidate.prefix):])
			return explicitImageCommand{
				Mode:               candidate.mode,
				Prompt:             prompt,
				FallbackToGenerate: candidate.fallbackToGenerate,
			}, true
		}
	}

	return explicitImageCommand{}, false
}

type textMatcher struct {
	excludes []string
	prefixes []string
	phrases  []string
}

func (m *textMatcher) matches(text string) bool {
	if text == "" {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(text))
	for _, ex := range m.excludes {
		if strings.Contains(lower, ex) {
			return false
		}
	}
	for _, p := range m.prefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	for _, p := range m.phrases {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

var sharedEditPhrases = []string{
	"remove background",
	"убери фон",
	"сделай фон",
}

var imageGenerationMatcher = textMatcher{
	prefixes: []string{
		"нарисуй",
		"сгенерируй",
		"создай изображение",
		"создай картинку",
		"создай иллюстрацию",
		"draw ",
		"draw me",
		"generate ",
		"generate image",
		"create image",
		"create an image",
		"make an image",
		"illustrate ",
		"render ",
	},
	phrases: []string{
		"сгенерируй изображение",
		"generate an image",
		"make me an image",
		"create a poster",
	},
}

var imageEditMatcher = textMatcher{
	prefixes: []string{
		"отредактируй",
		"измени",
		"убери",
		"удали",
		"замени",
		"добавь",
		"перекрась",
		"edit ",
		"edit this",
		"remove ",
		"replace ",
		"change ",
		"add ",
		"make it ",
		"turn it ",
	},
	phrases: append([]string{
		"сделай реалистичнее",
		"make it realistic",
		"make it look like",
		"change the background",
	}, sharedEditPhrases...),
}

var explicitImageEditMatcher = textMatcher{
	excludes: []string{"пример", "объясн", "ответ"},
	phrases: append([]string{
		"edit the image",
		"edit this image",
		"edit the photo",
		"edit this photo",
		"change the image",
		"change the photo",
		"change the latest image",
		"change the latest photo",
		"update the image",
		"make the image",
		"make the photo",
		"измени изображение",
		"измени фото",
		"измени картинку",
		"измени последнее изображение",
		"отредактируй изображение",
		"отредактируй фото",
		"отредактируй картинку",
		"дорисуй",
		"пририсуй",
		"добавь ему",
		"добавь ей",
		"добавь им",
		"сделай изображение",
		"сделай картинку",
	}, sharedEditPhrases...),
}

func looksLikeImageGeneration(text string) bool  { return imageGenerationMatcher.matches(text) }
func looksLikeImageEdit(text string) bool         { return imageEditMatcher.matches(text) }
func looksLikeExplicitImageEdit(text string) bool { return explicitImageEditMatcher.matches(text) }

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

func encodeImageDataURI(mimeType string, data []byte) string {
	if mimeType == "" {
		mimeType = "image/png"
	}

	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data))
}

func filenameForMimeType(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return "image.jpg"
	case "image/webp":
		return "image.webp"
	default:
		return "image.png"
	}
}

func (b *Bot) sendText(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.api.Send(msg); err != nil {
		b.log.Error("failed to send message", "chat_id", chatID, "error", err)
	}
}

func (b *Bot) sendLongText(chatID int64, text string) {
	const maxLen = 4000

	if utf8.RuneCountInString(text) <= maxLen {
		b.sendText(chatID, text)
		return
	}

	parts := splitText(text, maxLen)
	for _, part := range parts {
		b.sendText(chatID, part)
	}
}

func splitText(text string, maxLen int) []string {
	var parts []string

	for len(text) > 0 {
		if utf8.RuneCountInString(text) <= maxLen {
			parts = append(parts, text)
			break
		}

		// Find the byte offset that corresponds to maxLen runes
		byteLimit := 0
		for i := 0; i < maxLen; i++ {
			_, size := utf8.DecodeRuneInString(text[byteLimit:])
			byteLimit += size
		}

		splitIdx := strings.LastIndex(text[:byteLimit], "\n\n")
		if splitIdx == -1 {
			splitIdx = strings.LastIndex(text[:byteLimit], "\n")
		}
		if splitIdx == -1 {
			splitIdx = strings.LastIndex(text[:byteLimit], ". ")
		}
		if splitIdx == -1 {
			splitIdx = byteLimit - 1
		}

		parts = append(parts, text[:splitIdx+1])
		text = strings.TrimSpace(text[splitIdx+1:])
	}

	return parts
}
