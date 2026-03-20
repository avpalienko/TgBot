package telegram

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
	"github.com/user/tgbot/internal/logger"
)

var httpImageClient = &http.Client{Timeout: 30 * time.Second}

// Client wraps the Telegram Bot API for sending messages and downloading files.
type Client struct {
	api         *tgbotapi.BotAPI
	log         logger.Logger
	fileBaseURL string
}

func NewClient(api *tgbotapi.BotAPI, log logger.Logger) *Client {
	return &Client{api: api, log: log}
}

// SetFileBaseURL overrides the base URL used for file downloads.
// When set, file URLs are constructed as fileBaseURL + "/" + filePath
// instead of using the library's default FileEndpoint constant.
func (c *Client) SetFileBaseURL(url string) {
	c.fileBaseURL = url
}

// API returns the underlying BotAPI for operations not covered by Client
// (e.g. receiving updates, setting commands).
func (c *Client) API() *tgbotapi.BotAPI {
	return c.api
}

func (c *Client) SendText(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := c.api.Send(msg); err != nil {
		c.log.Error("failed to send message", "chat_id", chatID, "error", err)
	}
}

func (c *Client) SendLongText(chatID int64, text string) {
	const maxLen = 4000

	if utf8.RuneCountInString(text) <= maxLen {
		c.SendText(chatID, text)
		return
	}

	parts := SplitText(text, maxLen)
	for _, part := range parts {
		c.SendText(chatID, part)
	}
}

func SplitText(text string, maxLen int) []string {
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

func (c *Client) SendPhoto(chatID int64, imageBytes []byte, mimeType, caption string) {
	msg := tgbotapi.NewPhoto(chatID, tgbotapi.FileBytes{
		Name:  FilenameForMimeType(mimeType),
		Bytes: imageBytes,
	})
	captionUTF16Len := len(utf16.Encode([]rune(caption)))
	if caption != "" && captionUTF16Len <= 1024 {
		msg.Caption = caption
	}

	if _, err := c.api.Send(msg); err != nil {
		c.log.Error("failed to send photo", "chat_id", chatID, "error", err)
		return
	}

	if caption != "" && captionUTF16Len > 1024 {
		c.SendLongText(chatID, caption)
	}
}

func (c *Client) SendChatAction(chatID int64, action string) {
	chatAction := tgbotapi.NewChatAction(chatID, action)
	if _, err := c.api.Request(chatAction); err != nil {
		c.log.Error("failed to send chat action", "chat_id", chatID, "error", err)
	}
}

func (c *Client) DownloadPhoto(ctx context.Context, photos []tgbotapi.PhotoSize) (string, error) {
	if len(photos) == 0 {
		return "", fmt.Errorf("message does not contain a photo")
	}

	photo := photos[len(photos)-1]
	fileURL, err := c.fileDirectURL(photo.FileID)
	if err != nil {
		return "", fmt.Errorf("failed to get file URL: %w", err)
	}

	return DownloadAndEncodeImage(ctx, fileURL)
}

func (c *Client) fileDirectURL(fileID string) (string, error) {
	if c.fileBaseURL == "" {
		return c.api.GetFileDirectURL(fileID)
	}
	file, err := c.api.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		return "", err
	}
	return c.fileBaseURL + "/" + file.FilePath, nil
}

func DownloadAndEncodeImage(ctx context.Context, url string) (string, error) {
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

	// Detect content type from file content (magic bytes), not from header.
	// Telegram often returns application/octet-stream which OpenAI rejects.
	contentType := http.DetectContentType(data)
	if !strings.HasPrefix(contentType, "image/") {
		contentType = "image/jpeg"
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	dataURI := fmt.Sprintf("data:%s;base64,%s", contentType, encoded)

	return dataURI, nil
}

func EncodeImageDataURI(mimeType string, data []byte) string {
	if mimeType == "" {
		mimeType = "image/png"
	}

	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data))
}

func FilenameForMimeType(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return "image.jpg"
	case "image/webp":
		return "image.webp"
	default:
		return "image.png"
	}
}
