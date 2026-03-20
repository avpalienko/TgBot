package bot

import (
    "context"
    "strings"

    tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
    "github.com/user/tgbot/internal/ai"
    "github.com/user/tgbot/internal/logger"
    "github.com/user/tgbot/internal/session"
)

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
        imageData, err := b.downloadTelegramPhoto(ctx, msg.ReplyToMessage.Photo)
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

    imageData, err := b.downloadTelegramPhoto(ctx, msg.Photo)
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
            imageData, err := b.downloadTelegramPhoto(ctx, msg.ReplyToMessage.Photo)
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
    messages := make([]session.Message, len(history)+1)
    copy(messages, history)
    messages[len(history)] = userMsg

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
