package bot

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/user/tgbot/internal/ai"
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
	testBot := &Bot{
		api: api,
		log: logger.New(logger.Config{
			Level:  "debug",
			Format: "text",
			Output: &logOutput,
		}),
	}

	testBot.sendChatAction(12345, tgbotapi.ChatTyping)

	if got := atomic.LoadInt32(&sendChatActionCalls); got != 1 {
		t.Fatalf("expected one sendChatAction request, got %d", got)
	}

	if strings.Contains(logOutput.String(), "failed to send chat action") {
		t.Fatalf("expected no error log, got %q", logOutput.String())
	}
}

func TestLooksLikeExplicitImageEdit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want bool
	}{
		{
			name: "natural language redraw request",
			text: "дорисуй ему зубастость",
			want: true,
		},
		{
			name: "natural language add detail request",
			text: "добавь ему очки",
			want: true,
		},
		{
			name: "plain text refinement should stay chat",
			text: "добавь пример к ответу",
			want: false,
		},
		{
			name: "explicit english image edit",
			text: "edit the latest image and remove background",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := looksLikeExplicitImageEdit(tt.text)
			if got != tt.want {
				t.Fatalf("looksLikeExplicitImageEdit(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestParseExplicitImageCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		text   string
		want   explicitImageCommand
		wantOK bool
	}{
		{
			name: "generic img command",
			text: "img: add sharp teeth",
			want: explicitImageCommand{
				Mode:               ai.ModeEditImage,
				Prompt:             "add sharp teeth",
				FallbackToGenerate: true,
			},
			wantOK: true,
		},
		{
			name: "generic img command after size extraction",
			text: "draw: hedgehog poster",
			want: explicitImageCommand{
				Mode:   ai.ModeGenerateImage,
				Prompt: "hedgehog poster",
			},
			wantOK: true,
		},
		{
			name: "explicit edit command",
			text: "edit: remove background",
			want: explicitImageCommand{
				Mode:   ai.ModeEditImage,
				Prompt: "remove background",
			},
			wantOK: true,
		},
		{
			name: "russian img alias",
			text: "фото: дорисуй клыки",
			want: explicitImageCommand{
				Mode:               ai.ModeEditImage,
				Prompt:             "дорисуй клыки",
				FallbackToGenerate: true,
			},
			wantOK: true,
		},
		{
			name: "photo caption explicit prefix remains edit command",
			text: "фото: Сделай из этой фотографии более реалистичную",
			want: explicitImageCommand{
				Mode:               ai.ModeEditImage,
				Prompt:             "Сделай из этой фотографии более реалистичную",
				FallbackToGenerate: true,
			},
			wantOK: true,
		},
		{
			name: "russian edit alias",
			text: "правь: убери фон",
			want: explicitImageCommand{
				Mode:   ai.ModeEditImage,
				Prompt: "убери фон",
			},
			wantOK: true,
		},
		{
			name: "explicit draw command",
			text: "draw: neon poster",
			want: explicitImageCommand{
				Mode:   ai.ModeGenerateImage,
				Prompt: "neon poster",
			},
			wantOK: true,
		},
		{
			name:   "normal text",
			text:   "расскажи анекдот",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := parseExplicitImageCommand(tt.text)
			if ok != tt.wantOK {
				t.Fatalf("parseExplicitImageCommand(%q) ok = %v, want %v", tt.text, ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}

			if got != tt.want {
				t.Fatalf("parseExplicitImageCommand(%q) = %+v, want %+v", tt.text, got, tt.want)
			}
		})
	}
}

func TestExtractImageSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		text     string
		wantText string
		wantSize string
	}{
		{
			name:     "latin lowercase x",
			text:     "draw hedgehog 1024x1536 please",
			wantText: "draw hedgehog please",
			wantSize: "1024x1536",
		},
		{
			name:     "latin uppercase X",
			text:     "1024X1024 draw: poster",
			wantText: "draw: poster",
			wantSize: "1024x1024",
		},
		{
			name:     "cyrillic lowercase x",
			text:     "фото: добавь фон 1536х1024",
			wantText: "фото: добавь фон",
			wantSize: "1536x1024",
		},
		{
			name:     "cyrillic uppercase x",
			text:     "правь: убери фон 1024Х1536",
			wantText: "правь: убери фон",
			wantSize: "1024x1536",
		},
		{
			name:     "no size",
			text:     "нарисуй ежика",
			wantText: "нарисуй ежика",
			wantSize: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotText, gotSize := extractImageSize(tt.text)
			if gotText != tt.wantText || gotSize != tt.wantSize {
				t.Fatalf("extractImageSize(%q) = (%q, %q), want (%q, %q)", tt.text, gotText, gotSize, tt.wantText, tt.wantSize)
			}
		})
	}
}

func TestSplitTextRuneAware(t *testing.T) {
	t.Parallel()

	t.Run("short text is returned as single part", func(t *testing.T) {
		t.Parallel()
		parts := splitText("hello", 10)
		if len(parts) != 1 || parts[0] != "hello" {
			t.Fatalf("unexpected parts: %v", parts)
		}
	})

	t.Run("splits by rune count not byte count", func(t *testing.T) {
		t.Parallel()
		// 6 Cyrillic runes = 12 bytes; maxLen=3 runes should produce 2 parts
		text := "абвгде"
		parts := splitText(text, 3)
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
		// Each Cyrillic rune is 2 bytes; splitting should never produce invalid UTF-8
		runes := strings.Repeat("ж", 100)
		parts := splitText(runes, 30)
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
		parts := splitText(text, 30)
		if len(parts) != 2 {
			t.Fatalf("expected 2 parts, got %d: %v", len(parts), parts)
		}
		if parts[1] != b {
			t.Fatalf("expected second part %q, got %q", b, parts[1])
		}
	})
}

func TestIsSupportedImageSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		size string
		want bool
	}{
		{size: "", want: true},
		{size: "1024x1024", want: true},
		{size: "1024x1536", want: true},
		{size: "1536x1024", want: true},
		{size: "800x600", want: false},
		{size: "2048x2048", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.size, func(t *testing.T) {
			t.Parallel()

			if got := isSupportedImageSize(tt.size); got != tt.want {
				t.Fatalf("isSupportedImageSize(%q) = %v, want %v", tt.size, got, tt.want)
			}
		})
	}
}

func TestLooksLikeImageGeneration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "russian draw prefix", text: "нарисуй кота", want: true},
		{name: "english draw prefix", text: "draw me a cat", want: true},
		{name: "generate image", text: "generate image of sunset", want: true},
		{name: "create image", text: "create an image of mountains", want: true},
		{name: "plain text about drawing", text: "расскажи о рисовании", want: false},
		{name: "normal question", text: "what is the capital of France", want: false},
		{name: "empty string", text: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := looksLikeImageGeneration(tt.text); got != tt.want {
				t.Fatalf("looksLikeImageGeneration(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestLooksLikeImageEdit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "russian edit prefix", text: "отредактируй фон", want: true},
		{name: "english remove", text: "remove background", want: true},
		{name: "english add", text: "add shadow to the image", want: true},
		{name: "change background", text: "change the background to blue", want: true},
		{name: "plain text", text: "tell me about editing", want: false},
		{name: "empty string", text: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := looksLikeImageEdit(tt.text); got != tt.want {
				t.Fatalf("looksLikeImageEdit(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestIsReplyToPhoto(t *testing.T) {
	t.Parallel()

	t.Run("nil ReplyToMessage", func(t *testing.T) {
		t.Parallel()
		msg := &tgbotapi.Message{ReplyToMessage: nil}
		if isReplyToPhoto(msg) {
			t.Fatalf("expected false for nil ReplyToMessage")
		}
	})

	t.Run("empty Photo slice", func(t *testing.T) {
		t.Parallel()
		msg := &tgbotapi.Message{
			ReplyToMessage: &tgbotapi.Message{Photo: nil},
		}
		if isReplyToPhoto(msg) {
			t.Fatalf("expected false for empty Photo slice")
		}
	})

	t.Run("non-empty Photo slice", func(t *testing.T) {
		t.Parallel()
		msg := &tgbotapi.Message{
			ReplyToMessage: &tgbotapi.Message{
				Photo: []tgbotapi.PhotoSize{{FileID: "abc", Width: 100, Height: 100}},
			},
		}
		if !isReplyToPhoto(msg) {
			t.Fatalf("expected true for non-empty Photo slice")
		}
	})
}

func TestTextMatcherExcludes(t *testing.T) {
	t.Parallel()

	m := textMatcher{
		excludes: []string{"пример", "объясн"},
		phrases:  []string{"добавь ему"},
	}

	t.Run("match without exclude", func(t *testing.T) {
		t.Parallel()
		if !m.matches("добавь ему очки") {
			t.Fatalf("expected match")
		}
	})

	t.Run("exclude blocks match", func(t *testing.T) {
		t.Parallel()
		if m.matches("добавь ему пример") {
			t.Fatalf("expected exclude to block match")
		}
	})

	t.Run("another exclude blocks match", func(t *testing.T) {
		t.Parallel()
		if m.matches("объясни и добавь ему") {
			t.Fatalf("expected exclude to block match")
		}
	})
}

func TestEncodeImageDataURI(t *testing.T) {
	t.Parallel()

	t.Run("correct format", func(t *testing.T) {
		t.Parallel()
		data := []byte{0x89, 0x50, 0x4E, 0x47}
		got := encodeImageDataURI("image/png", data)
		if !strings.HasPrefix(got, "data:image/png;base64,") {
			t.Fatalf("expected data URI prefix, got %q", got[:40])
		}
	})

	t.Run("default mime type fallback", func(t *testing.T) {
		t.Parallel()
		got := encodeImageDataURI("", []byte{1, 2, 3})
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
			if got := filenameForMimeType(tt.mimeType); got != tt.want {
				t.Fatalf("filenameForMimeType(%q) = %q, want %q", tt.mimeType, got, tt.want)
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
	dataURI, err := downloadAndEncodeImage(ctx, server.URL)
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
	_, err := downloadAndEncodeImage(ctx, server.URL)
	if err == nil {
		t.Fatalf("expected error for 404 response")
	}
}
