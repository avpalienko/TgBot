package telegram

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/user/tgbot/internal/logger"
)

func TestSendChatActionUsesRequestForBooleanResponse(t *testing.T) {
	t.Parallel()

	const token = "test-token"

	var sendChatActionCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bot" + token + "/getMe":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"Test","username":"test_bot"}}`))
		case "/bot" + token + "/sendChatAction":
			atomic.AddInt32(&sendChatActionCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	api, err := tgbotapi.NewBotAPIWithClient(token, server.URL+"/bot%s/%s", server.Client())
	if err != nil {
		t.Fatalf("failed to create bot api: %v", err)
	}

	var logOutput bytes.Buffer
	client := NewClient(api, logger.New(logger.Config{
		Level:  "debug",
		Format: "text",
		Output: &logOutput,
	}))

	client.SendChatAction(12345, tgbotapi.ChatTyping)

	if got := atomic.LoadInt32(&sendChatActionCalls); got != 1 {
		t.Fatalf("expected one sendChatAction request, got %d", got)
	}

	if strings.Contains(logOutput.String(), "failed to send chat action") {
		t.Fatalf("expected no error log, got %q", logOutput.String())
	}
}

func TestSplitTextRuneAware(t *testing.T) {
	t.Parallel()

	t.Run("short text is returned as single part", func(t *testing.T) {
		t.Parallel()
		parts := SplitText("hello", 10)
		if len(parts) != 1 || parts[0] != "hello" {
			t.Fatalf("unexpected parts: %v", parts)
		}
	})

	t.Run("splits by rune count not byte count", func(t *testing.T) {
		t.Parallel()
		text := "абвгде"
		parts := SplitText(text, 3)
		if len(parts) != 2 {
			t.Fatalf("expected 2 parts, got %d: %v", len(parts), parts)
		}
		combined := parts[0] + parts[1]
		if combined != text {
			t.Fatalf("combined parts %q != original %q", combined, text)
		}
	})

	t.Run("does not split mid-rune", func(t *testing.T) {
		t.Parallel()
		runes := strings.Repeat("ж", 100)
		parts := SplitText(runes, 30)
		for i, p := range parts {
			for j, r := range p {
				if r == '\uFFFD' {
					t.Fatalf("part %d has replacement char at pos %d, mid-rune split occurred", i, j)
				}
			}
		}
	})

	t.Run("prefers paragraph boundary", func(t *testing.T) {
		t.Parallel()
		a := strings.Repeat("a", 20)
		b := strings.Repeat("b", 20)
		text := a + "\n\n" + b
		parts := SplitText(text, 30)
		if len(parts) != 2 {
			t.Fatalf("expected 2 parts, got %d: %v", len(parts), parts)
		}
		if parts[1] != b {
			t.Fatalf("expected second part %q, got %q", b, parts[1])
		}
	})
}

func TestEncodeImageDataURI(t *testing.T) {
	t.Parallel()

	t.Run("correct format", func(t *testing.T) {
		t.Parallel()
		data := []byte{0x89, 0x50, 0x4E, 0x47}
		got := EncodeImageDataURI("image/png", data)
		if !strings.HasPrefix(got, "data:image/png;base64,") {
			t.Fatalf("expected data URI prefix, got %q", got[:40])
		}
	})

	t.Run("default mime type fallback", func(t *testing.T) {
		t.Parallel()
		got := EncodeImageDataURI("", []byte{1, 2, 3})
		if !strings.HasPrefix(got, "data:image/png;base64,") {
			t.Fatalf("expected default image/png, got %q", got[:30])
		}
	})
}

func TestFilenameForMimeType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mimeType string
		want     string
	}{
		{"image/jpeg", "image.jpg"},
		{"image/webp", "image.webp"},
		{"image/png", "image.png"},
		{"image/gif", "image.png"},
		{"", "image.png"},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			t.Parallel()
			if got := FilenameForMimeType(tt.mimeType); got != tt.want {
				t.Fatalf("FilenameForMimeType(%q) = %q, want %q", tt.mimeType, got, tt.want)
			}
		})
	}
}

func TestDownloadAndEncodeImage(t *testing.T) {
	t.Parallel()

	imageBytes := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(imageBytes)
	}))
	defer server.Close()

	ctx := t.Context()
	dataURI, err := DownloadAndEncodeImage(ctx, server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(dataURI, "data:image/") {
		t.Fatalf("expected data URI with image mime type, got %q", dataURI[:30])
	}
	if !strings.Contains(dataURI, ";base64,") {
		t.Fatalf("expected base64 encoding in data URI")
	}
}

func TestDownloadAndEncodeImageError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	ctx := t.Context()
	_, err := DownloadAndEncodeImage(ctx, server.URL)
	if err == nil {
		t.Fatalf("expected error for 404 response")
	}
}
