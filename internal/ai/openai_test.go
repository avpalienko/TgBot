package ai

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/openai/openai-go/responses"
)

func TestNormalizeRole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  responses.EasyInputMessageRole
	}{
		{"user", responses.EasyInputMessageRoleUser},
		{"assistant", responses.EasyInputMessageRoleAssistant},
		{"system", responses.EasyInputMessageRoleSystem},
		{"developer", responses.EasyInputMessageRoleDeveloper},
		{"User", responses.EasyInputMessageRoleUser},
		{"ASSISTANT", responses.EasyInputMessageRoleAssistant},
		{"unknown", responses.EasyInputMessageRoleUser},
		{"", responses.EasyInputMessageRoleUser},
		{"  assistant  ", responses.EasyInputMessageRoleAssistant},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := normalizeRole(tt.input)
			if got != tt.want {
				t.Fatalf("normalizeRole(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestImagePrompt(t *testing.T) {
	t.Parallel()

	t.Run("edit empty prompt", func(t *testing.T) {
		t.Parallel()
		got := imagePrompt("", true)
		if got != "Edit the provided image while preserving the main subject." {
			t.Fatalf("unexpected prompt: %q", got)
		}
	})

	t.Run("edit with prompt", func(t *testing.T) {
		t.Parallel()
		got := imagePrompt("remove background", true)
		want := "Edit the provided image according to this request: remove background"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("generate empty prompt", func(t *testing.T) {
		t.Parallel()
		got := imagePrompt("", false)
		if got != "Generate an image that matches the user's request." {
			t.Fatalf("unexpected prompt: %q", got)
		}
	})

	t.Run("generate with prompt", func(t *testing.T) {
		t.Parallel()
		got := imagePrompt("a neon cat", false)
		if got != "a neon cat" {
			t.Fatalf("got %q, want %q", got, "a neon cat")
		}
	})

	t.Run("whitespace-only treated as empty", func(t *testing.T) {
		t.Parallel()
		got := imagePrompt("   ", true)
		if got != "Edit the provided image while preserving the main subject." {
			t.Fatalf("unexpected prompt for whitespace input: %q", got)
		}
	})
}

func TestImageInstructions(t *testing.T) {
	t.Parallel()

	t.Run("edit mode", func(t *testing.T) {
		t.Parallel()
		got := imageInstructions(true)
		if got == "" {
			t.Fatalf("instructions should not be empty")
		}
		if got == imageInstructions(false) {
			t.Fatalf("edit and generate instructions should differ")
		}
	})

	t.Run("generate mode", func(t *testing.T) {
		t.Parallel()
		got := imageInstructions(false)
		if got == "" {
			t.Fatalf("instructions should not be empty")
		}
	})
}

func TestParseResponse(t *testing.T) {
	t.Parallel()

	t.Run("nil response returns error", func(t *testing.T) {
		t.Parallel()
		_, err := parseResponse(nil)
		if err == nil {
			t.Fatalf("expected error for nil response")
		}
	})

	t.Run("text only response", func(t *testing.T) {
		t.Parallel()
		resp := &responses.Response{
			ID: "resp-123",
			Output: []responses.ResponseOutputItemUnion{
				{
					Type: "message",
					Content: []responses.ResponseOutputMessageContentUnion{
						{
							Type: "output_text",
							Text: "hello world",
						},
					},
				},
			},
		}
		result, err := parseResponse(resp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.ResponseID != "resp-123" {
			t.Fatalf("expected response ID %q, got %q", "resp-123", result.ResponseID)
		}
		if result.Text != "hello world" {
			t.Fatalf("expected text %q, got %q", "hello world", result.Text)
		}
		if len(result.ImageBytes) != 0 {
			t.Fatalf("expected no image bytes")
		}
	})

	t.Run("response with image output", func(t *testing.T) {
		t.Parallel()
		imageData := base64.StdEncoding.EncodeToString([]byte("fake-png-data"))
		resp := &responses.Response{
			ID: "resp-456",
			Output: []responses.ResponseOutputItemUnion{
				{
					Type:   "image_generation_call",
					Result: imageData,
				},
			},
		}
		result, err := parseResponse(resp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(result.ImageBytes) != "fake-png-data" {
			t.Fatalf("image bytes mismatch: %q", string(result.ImageBytes))
		}
		if result.ImageMimeType != imageMimeTypePNG {
			t.Fatalf("expected mime type %q, got %q", imageMimeTypePNG, result.ImageMimeType)
		}
	})

	t.Run("empty response returns error", func(t *testing.T) {
		t.Parallel()
		resp := &responses.Response{ID: "resp-789", Output: nil}
		_, err := parseResponse(resp)
		if err == nil {
			t.Fatalf("expected error for empty output")
		}
	})
}

func TestBuildMessageInput(t *testing.T) {
	t.Parallel()

	t.Run("text only user message", func(t *testing.T) {
		t.Parallel()
		msg := Message{Role: "user", Content: "hello"}
		_, ok := buildMessageInput(msg)
		if !ok {
			t.Fatalf("expected ok=true for text-only user message")
		}
	})

	t.Run("image and text user message", func(t *testing.T) {
		t.Parallel()
		msg := Message{Role: "user", Content: "describe this", ImageData: "data:image/png;base64,AAA"}
		_, ok := buildMessageInput(msg)
		if !ok {
			t.Fatalf("expected ok=true for image+text user message")
		}
	})

	t.Run("assistant message with image ignores image", func(t *testing.T) {
		t.Parallel()
		msg := Message{Role: "assistant", Content: "here it is", ImageData: "data:image/png;base64,AAA"}
		_, ok := buildMessageInput(msg)
		if !ok {
			t.Fatalf("expected ok=true for assistant message with content")
		}
	})

	t.Run("empty content skipped", func(t *testing.T) {
		t.Parallel()
		msg := Message{Role: "user", Content: ""}
		_, ok := buildMessageInput(msg)
		if ok {
			t.Fatalf("expected ok=false for empty content without image")
		}
	})

	t.Run("user image without text", func(t *testing.T) {
		t.Parallel()
		msg := Message{Role: "user", Content: "", ImageData: "data:image/png;base64,AAA"}
		_, ok := buildMessageInput(msg)
		if !ok {
			t.Fatalf("expected ok=true for user image without text")
		}
	})

	t.Run("assistant empty content with image skipped", func(t *testing.T) {
		t.Parallel()
		msg := Message{Role: "assistant", Content: "", ImageData: "data:image/png;base64,AAA"}
		_, ok := buildMessageInput(msg)
		if ok {
			t.Fatalf("expected ok=false for assistant with empty content and image")
		}
	})
}

func TestBuildHistoryInput(t *testing.T) {
	t.Parallel()

	t.Run("multiple messages", func(t *testing.T) {
		t.Parallel()
		messages := []Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
			{Role: "user", Content: "how are you"},
		}
		input := buildHistoryInput(messages)
		if len(input) != 3 {
			t.Fatalf("expected 3 input items, got %d", len(input))
		}
	})

	t.Run("empty list", func(t *testing.T) {
		t.Parallel()
		input := buildHistoryInput(nil)
		if len(input) != 0 {
			t.Fatalf("expected 0 input items, got %d", len(input))
		}
	})

	t.Run("skips empty messages", func(t *testing.T) {
		t.Parallel()
		messages := []Message{
			{Role: "user", Content: "hello"},
			{Role: "user", Content: ""},
			{Role: "assistant", Content: "hi"},
		}
		input := buildHistoryInput(messages)
		if len(input) != 2 {
			t.Fatalf("expected 2 input items (1 skipped), got %d", len(input))
		}
	})
}

func TestNewOpenAIProvider(t *testing.T) {
	t.Parallel()

	t.Run("without base URL", func(t *testing.T) {
		t.Parallel()
		p := NewOpenAIProvider("test-key", "gpt-4o", "", 3)
		if p.model != "gpt-4o" {
			t.Fatalf("expected model %q, got %q", "gpt-4o", p.model)
		}
	})

	t.Run("with base URL", func(t *testing.T) {
		t.Parallel()
		p := NewOpenAIProvider("test-key", "gpt-4-turbo", "https://custom.api", 3)
		if p.model != "gpt-4-turbo" {
			t.Fatalf("expected model %q, got %q", "gpt-4-turbo", p.model)
		}
	})

	t.Run("with zero retries", func(t *testing.T) {
		t.Parallel()
		p := NewOpenAIProvider("test-key", "gpt-4o", "", 0)
		if p.model != "gpt-4o" {
			t.Fatalf("expected model %q, got %q", "gpt-4o", p.model)
		}
	})

	t.Run("with negative retries uses SDK default", func(t *testing.T) {
		t.Parallel()
		p := NewOpenAIProvider("test-key", "gpt-4o", "", -1)
		if p.model != "gpt-4o" {
			t.Fatalf("expected model %q, got %q", "gpt-4o", p.model)
		}
	})
}

func TestModelName(t *testing.T) {
	t.Parallel()

	p := NewOpenAIProvider("key", "gpt-4o-mini", "", 2)
	if got := p.ModelName(); got != "gpt-4o-mini" {
		t.Fatalf("ModelName() = %q, want %q", got, "gpt-4o-mini")
	}
}

// --------------- Respond with mocked HTTP ---------------

func newMockOpenAIServer(t *testing.T, handler http.HandlerFunc) *OpenAIProvider {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return NewOpenAIProvider("test-key", "test-model", server.URL, 0)
}

func TestRespondChat(t *testing.T) {
	t.Parallel()

	provider := newMockOpenAIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
            "id": "resp-chat-1",
            "object": "response",
            "output": [{
                "type": "message",
                "role": "assistant",
                "content": [{"type": "output_text", "text": "Hello from AI!"}]
            }]
        }`)
	})

	result, err := provider.Respond(context.Background(), Request{
		Mode:    ModeChat,
		History: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "Hello from AI!" {
		t.Fatalf("expected %q, got %q", "Hello from AI!", result.Text)
	}
	if result.ResponseID != "resp-chat-1" {
		t.Fatalf("expected response ID %q, got %q", "resp-chat-1", result.ResponseID)
	}
}

func TestRespondGenerateImage(t *testing.T) {
	t.Parallel()

	imageB64 := base64.StdEncoding.EncodeToString([]byte("fake-png-data"))
	provider := newMockOpenAIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
            "id": "resp-img-1",
            "object": "response",
            "output": [{
                "type": "image_generation_call",
                "result": "%s"
            }]
        }`, imageB64)
	})

	result, err := provider.Respond(context.Background(), Request{
		Mode: ModeGenerateImage,
		Text: "a neon cat",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result.ImageBytes) != "fake-png-data" {
		t.Fatalf("image bytes mismatch: got %q", string(result.ImageBytes))
	}
	if result.ImageMimeType != imageMimeTypePNG {
		t.Fatalf("expected mime type %q, got %q", imageMimeTypePNG, result.ImageMimeType)
	}
}

func TestRespondEditImage(t *testing.T) {
	t.Parallel()

	imageB64 := base64.StdEncoding.EncodeToString([]byte("edited-png"))
	provider := newMockOpenAIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
            "id": "resp-edit-1",
            "object": "response",
            "output": [{
                "type": "image_generation_call",
                "result": "%s"
            }]
        }`, imageB64)
	})

	result, err := provider.Respond(context.Background(), Request{
		Mode:           ModeEditImage,
		Text:           "remove background",
		InputImageData: "data:image/png;base64,AAAA",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result.ImageBytes) != "edited-png" {
		t.Fatalf("image bytes mismatch: got %q", string(result.ImageBytes))
	}
}

func TestRespondEditImageNoInput(t *testing.T) {
	t.Parallel()

	provider := newMockOpenAIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	_, err := provider.Respond(context.Background(), Request{
		Mode: ModeEditImage,
		Text: "remove background",
	})
	if err == nil {
		t.Fatalf("expected error for edit without input image")
	}
}

func TestRespondUnsupportedMode(t *testing.T) {
	t.Parallel()

	provider := newMockOpenAIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	_, err := provider.Respond(context.Background(), Request{
		Mode: "unknown_mode",
	})
	if err == nil {
		t.Fatalf("expected error for unsupported mode")
	}
}

func TestRespondChatEmpty(t *testing.T) {
	t.Parallel()

	provider := newMockOpenAIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	_, err := provider.Respond(context.Background(), Request{
		Mode:    ModeChat,
		History: nil,
	})
	if err == nil {
		t.Fatalf("expected error for empty chat request")
	}
}

func TestRespondChatHTTPError(t *testing.T) {
	t.Parallel()

	provider := newMockOpenAIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"error":{"message":"server error","type":"server_error"}}`)
	})

	_, err := provider.Respond(context.Background(), Request{
		Mode:    ModeChat,
		History: []Message{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatalf("expected error for HTTP 500")
	}
}

func TestRespondChatRetryOnTransientError(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	attempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		attempts++
		attempt := attempts
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if attempt == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error":{"message":"temporary failure","type":"server_error"}}`)
			return
		}
		fmt.Fprint(w, `{
            "id": "resp-retry-1",
            "object": "response",
            "output": [{
                "type": "message",
                "content": [{"type": "output_text", "text": "recovered"}]
            }]
        }`)
	}))
	t.Cleanup(server.Close)

	provider := NewOpenAIProvider("test-key", "test-model", server.URL, 2)

	result, err := provider.Respond(context.Background(), Request{
		Mode:    ModeChat,
		History: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("expected retry to succeed, got error: %v", err)
	}
	if result.Text != "recovered" {
		t.Fatalf("expected %q, got %q", "recovered", result.Text)
	}

	mu.Lock()
	finalAttempts := attempts
	mu.Unlock()
	if finalAttempts < 2 {
		t.Fatalf("expected at least 2 attempts, got %d", finalAttempts)
	}
}

func TestRespondChatWithPreviousResponseID(t *testing.T) {
	t.Parallel()

	provider := newMockOpenAIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
            "id": "resp-2",
            "object": "response",
            "output": [{
                "type": "message",
                "content": [{"type": "output_text", "text": "continued"}]
            }]
        }`)
	})

	result, err := provider.Respond(context.Background(), Request{
		Mode: ModeChat,
		History: []Message{
			{Role: "user", Content: "first"},
			{Role: "assistant", Content: "reply"},
			{Role: "user", Content: "second"},
		},
		PreviousResponseID: "resp-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "continued" {
		t.Fatalf("expected %q, got %q", "continued", result.Text)
	}
}
