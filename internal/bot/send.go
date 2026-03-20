package bot

import (
    "context"
    "encoding/base64"
    "fmt"
    "io"
    "net/http"
    "strings"
    "time"
    "unicode/utf16"
    "unicode/utf8"

    tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var httpImageClient = &http.Client{Timeout: 30 * time.Second}

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

func (b *Bot) sendPhoto(chatID int64, imageBytes []byte, mimeType, caption string) {
    msg := tgbotapi.NewPhoto(chatID, tgbotapi.FileBytes{
        Name:  filenameForMimeType(mimeType),
        Bytes: imageBytes,
    })
    captionUTF16Len := len(utf16.Encode([]rune(caption)))
    if caption != "" && captionUTF16Len <= 1024 {
        msg.Caption = caption
    }

    if _, err := b.api.Send(msg); err != nil {
        b.log.Error("failed to send photo", "chat_id", chatID, "error", err)
        return
    }

    if caption != "" && captionUTF16Len > 1024 {
        b.sendLongText(chatID, caption)
    }
}

func (b *Bot) sendChatAction(chatID int64, action string) {
    chatAction := tgbotapi.NewChatAction(chatID, action)
    if _, err := b.api.Request(chatAction); err != nil {
        b.log.Error("failed to send chat action", "chat_id", chatID, "error", err)
    }
}

func (b *Bot) downloadTelegramPhoto(ctx context.Context, photos []tgbotapi.PhotoSize) (string, error) {
    if len(photos) == 0 {
        return "", fmt.Errorf("message does not contain a photo")
    }

    photo := photos[len(photos)-1]
    fileURL, err := b.api.GetFileDirectURL(photo.FileID)
    if err != nil {
        return "", fmt.Errorf("failed to get file URL: %w", err)
    }

    return downloadAndEncodeImage(ctx, fileURL)
}

func downloadAndEncodeImage(ctx context.Context, url string) (string, error) {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    if err != nil {
        return "", fmt.Errorf("failed to create image request: %w", err)
    }

    resp, err := httpImageClient.Do(req)
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
