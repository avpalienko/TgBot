package bot

import (
	"context"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/user/tgbot/internal/ai"
	"github.com/user/tgbot/internal/logger"
	"github.com/user/tgbot/internal/routing"
	"github.com/user/tgbot/internal/session"
	"github.com/user/tgbot/internal/telegram"
)

func (b *Bot) isPromptTooLong(chatID int64, log logger.Logger, text string) bool {
	if len([]rune(text)) > b.maxPromptLength {
		log.Warn("message rejected: too long", "length", len([]rune(text)), "max", b.maxPromptLength)
		b.tg.SendText(chatID, fmt.Sprintf("Message too long (%d characters). Maximum allowed: %d.", len([]rune(text)), b.maxPromptLength))
		return true
	}
	return false
}

func (b *Bot) handleTextMessage(ctx context.Context, msg *tgbotapi.Message, log logger.Logger) {
	userID := msg.From.ID
	chatID := msg.Chat.ID
	text := strings.TrimSpace(msg.Text)

	if b.isPromptTooLong(chatID, log, text) {
		return
	}

	imagePrompt, imageSize := routing.ExtractImageSize(text)

	log.Info("message received", "text_length", len(text))

	b.tg.SendChatAction(chatID, tgbotapi.ChatTyping)

	if b.handleExplicitTextImageCommand(ctx, msg, log, text, imagePrompt, imageSize) {
		return
	}

	if routing.LooksLikeImageEdit(imagePrompt) && routing.IsReplyToPhoto(msg) {
		if !b.ensureSupportedImageSize(chatID, log, imageSize) {
			return
		}
		imageData, err := b.tg.DownloadPhoto(ctx, msg.ReplyToMessage.Photo)
		if err != nil {
			log.Error("failed to load reply photo", "error", err)
			b.tg.SendText(chatID, "Failed to load the replied photo. Please try again.")
			return
		}
		b.dispatchImageEdit(ctx, userID, chatID, log, "reply_text_edit_intent", "reply_photo", imagePrompt, imageSize, imageData, session.Message{Role: "user", Content: text, ImageData: imageData})
		return
	}

	if routing.LooksLikeImageGeneration(imagePrompt) {
		if !b.ensureSupportedImageSize(chatID, log, imageSize) {
			return
		}
		b.dispatchImageGeneration(ctx, userID, chatID, log, "text_generation_intent", imagePrompt, imageSize, session.Message{Role: "user", Content: text})
		return
	}

	if routing.LooksLikeExplicitImageEdit(imagePrompt) {
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

	if b.isPromptTooLong(chatID, log, caption) {
		return
	}

	imagePrompt, imageSize := routing.ExtractImageSize(caption)

	log.Info("photo received", "caption_length", len(caption))

	b.tg.SendChatAction(chatID, tgbotapi.ChatTyping)

	imageData, err := b.tg.DownloadPhoto(ctx, msg.Photo)
	if err != nil {
		log.Error("failed to process image", "error", err)
		b.tg.SendText(chatID, "Failed to process the image. Please try again.")
		return
	}

	userMsg := session.Message{Role: "user", Content: caption, ImageData: imageData}
	if b.handleExplicitPhotoImageCommand(ctx, userID, chatID, log, userMsg, imageData, imagePrompt, imageSize) {
		return
	}

	if routing.LooksLikeImageEdit(imagePrompt) {
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
		if routing.IsReplyToPhoto(msg) {
			imageData, err := b.tg.DownloadPhoto(ctx, msg.ReplyToMessage.Photo)
			if err != nil {
				log.Error("failed to load reply photo", "error", err)
				b.tg.SendText(chatID, "Failed to load the replied photo. Please try again.")
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
		b.tg.SendText(chatID, "No image found to edit. Reply to a photo, send a photo with a caption, or use `img:` after generating an image first.")
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

func (b *Bot) prepareExplicitImageCommand(chatID int64, log logger.Logger, imagePrompt, imageSize string) (routing.ExplicitImageCommand, string, string, bool) {
	explicit, ok := routing.ParseExplicitImageCommand(imagePrompt)
	if !ok {
		return routing.ExplicitImageCommand{}, "", imageSize, false
	}

	prompt := explicit.Prompt
	if parsedPrompt, parsedSize := routing.ExtractImageSize(prompt); parsedSize != "" {
		prompt = parsedPrompt
		imageSize = parsedSize
	}

	if !b.ensureSupportedImageSize(chatID, log, imageSize) {
		return routing.ExplicitImageCommand{}, "", imageSize, false
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
	if routing.IsSupportedImageSize(imageSize) {
		return true
	}

	log.Warn("unsupported image size requested", "image_size", imageSize)
	b.tg.SendText(chatID, "Unsupported image size. Use one of: 1024x1024, 1024x1536, 1536x1024.")
	return false
}

func (b *Bot) handleChatRequest(ctx context.Context, userID, chatID int64, log logger.Logger, userMsg session.Message) {
	history := b.sessions.Get(userID)
	aiMessages := make([]ai.Message, len(history)+1)
	for i, m := range history {
		aiMessages[i] = ai.Message{Role: m.Role, Content: m.Content, ImageData: m.ImageData}
	}
	aiMessages[len(history)] = ai.Message{Role: userMsg.Role, Content: userMsg.Content, ImageData: userMsg.ImageData}

	log.Debug("calling AI chat", "model", b.ai.ModelName(), "messages_count", len(aiMessages))

	result, err := b.ai.Respond(ctx, ai.Request{
		Mode:               ai.ModeChat,
		History:            aiMessages,
		PreviousResponseID: b.sessions.GetPreviousResponseID(userID),
	})
	if err != nil {
		b.handleAIError(chatID, log, err)
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
		b.handleAIError(chatID, log, err)
		return
	}

	log.Info("AI image response received", "has_text", result.Text != "", "has_image", len(result.ImageBytes) > 0, "mode", req.Mode)
	b.persistResult(userID, userMsg, result)
	b.sendResult(chatID, result)
}

func (b *Bot) handleAIError(chatID int64, log logger.Logger, err error) {
	switch {
	case ai.IsRateLimit(err):
		log.Warn("AI rate limit hit", "error", err)
		b.tg.SendText(chatID, "The AI service is temporarily overloaded. Please wait a moment and try again.")
	case ai.IsAuth(err):
		log.Error("AI authentication failure", "error", err)
		b.tg.SendText(chatID, "AI service configuration error. Please contact the administrator.")
	case ai.IsBadRequest(err):
		log.Warn("AI bad request", "error", err)
		b.tg.SendText(chatID, "Your request could not be processed. Try rephrasing or simplifying your message.")
	case ai.IsTransient(err):
		log.Warn("AI transient error", "error", err)
		b.tg.SendText(chatID, "The AI service is temporarily unavailable. Please try again in a few seconds.")
	default:
		log.Error("AI request failed", "error", err)
		b.tg.SendText(chatID, "An error occurred while processing your request. Please try again later.")
	}
}

func (b *Bot) persistResult(userID int64, userMsg session.Message, result ai.Result) {
	assistantMsg := session.Message{
		Role:    "assistant",
		Content: result.Text,
	}
	if len(result.ImageBytes) > 0 {
		assistantMsg.ImageData = telegram.EncodeImageDataURI(result.ImageMimeType, result.ImageBytes)
	}

	b.sessions.AddWithResponseID(userID, result.ResponseID, userMsg, assistantMsg)
}

func (b *Bot) sendResult(chatID int64, result ai.Result) {
	if len(result.ImageBytes) > 0 {
		b.tg.SendPhoto(chatID, result.ImageBytes, result.ImageMimeType, result.Text)
		return
	}

	if result.Text != "" {
		b.tg.SendLongText(chatID, result.Text)
	}
}
