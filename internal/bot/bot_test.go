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
