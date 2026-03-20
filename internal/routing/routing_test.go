package routing

import (
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/user/tgbot/internal/ai"
)

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

			got := LooksLikeExplicitImageEdit(tt.text)
			if got != tt.want {
				t.Fatalf("LooksLikeExplicitImageEdit(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestParseExplicitImageCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		text   string
		want   ExplicitImageCommand
		wantOK bool
	}{
		{
			name: "generic img command",
			text: "img: add sharp teeth",
			want: ExplicitImageCommand{
				Mode:               ai.ModeEditImage,
				Prompt:             "add sharp teeth",
				FallbackToGenerate: true,
			},
			wantOK: true,
		},
		{
			name: "generic img command after size extraction",
			text: "draw: hedgehog poster",
			want: ExplicitImageCommand{
				Mode:   ai.ModeGenerateImage,
				Prompt: "hedgehog poster",
			},
			wantOK: true,
		},
		{
			name: "explicit edit command",
			text: "edit: remove background",
			want: ExplicitImageCommand{
				Mode:   ai.ModeEditImage,
				Prompt: "remove background",
			},
			wantOK: true,
		},
		{
			name: "russian img alias",
			text: "фото: дорисуй клыки",
			want: ExplicitImageCommand{
				Mode:               ai.ModeEditImage,
				Prompt:             "дорисуй клыки",
				FallbackToGenerate: true,
			},
			wantOK: true,
		},
		{
			name: "photo caption explicit prefix remains edit command",
			text: "фото: Сделай из этой фотографии более реалистичную",
			want: ExplicitImageCommand{
				Mode:               ai.ModeEditImage,
				Prompt:             "Сделай из этой фотографии более реалистичную",
				FallbackToGenerate: true,
			},
			wantOK: true,
		},
		{
			name: "russian edit alias",
			text: "правь: убери фон",
			want: ExplicitImageCommand{
				Mode:   ai.ModeEditImage,
				Prompt: "убери фон",
			},
			wantOK: true,
		},
		{
			name: "explicit draw command",
			text: "draw: neon poster",
			want: ExplicitImageCommand{
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

			got, ok := ParseExplicitImageCommand(tt.text)
			if ok != tt.wantOK {
				t.Fatalf("ParseExplicitImageCommand(%q) ok = %v, want %v", tt.text, ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}

			if got != tt.want {
				t.Fatalf("ParseExplicitImageCommand(%q) = %+v, want %+v", tt.text, got, tt.want)
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

			gotText, gotSize := ExtractImageSize(tt.text)
			if gotText != tt.wantText || gotSize != tt.wantSize {
				t.Fatalf("ExtractImageSize(%q) = (%q, %q), want (%q, %q)", tt.text, gotText, gotSize, tt.wantText, tt.wantSize)
			}
		})
	}
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

			if got := IsSupportedImageSize(tt.size); got != tt.want {
				t.Fatalf("IsSupportedImageSize(%q) = %v, want %v", tt.size, got, tt.want)
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
		{name: "draw a noun", text: "draw a sunset", want: true},
		{name: "draw an animal", text: "draw an owl", want: true},
		{name: "draw short prompt", text: "draw cat", want: true},
		{name: "generate image", text: "generate image of sunset", want: true},
		{name: "create image", text: "create an image of mountains", want: true},
		{name: "create a picture", text: "create a picture of a castle", want: true},
		{name: "make a picture", text: "make a picture of the sea", want: true},
		{name: "render a scene", text: "render a 3d scene of a city", want: true},
		{name: "illustrate a story", text: "illustrate a fairy tale about dragons", want: true},
		{name: "generate a poster", text: "generate a poster for a concert", want: true},
		{name: "plain text about drawing", text: "расскажи о рисовании", want: false},
		{name: "normal question", text: "what is the capital of France", want: false},
		{name: "empty string", text: "", want: false},

		// False positives that should now be rejected
		{name: "draw conclusions", text: "draw conclusions from the data", want: false},
		{name: "draw attention", text: "draw attention to the issue", want: false},
		{name: "draw the line", text: "draw the line between right and wrong", want: false},
		{name: "draw a blank", text: "draw a blank on that question", want: false},
		{name: "draw a comparison", text: "draw a comparison between the two", want: false},
		{name: "draw a parallel", text: "draw a parallel with history", want: false},
		{name: "draw upon", text: "draw upon your experience", want: false},
		{name: "generate a report", text: "generate a report for last month", want: false},
		{name: "generate code", text: "generate code for sorting", want: false},
		{name: "generate a list", text: "generate a list of names", want: false},
		{name: "generate a summary", text: "generate a summary of the article", want: false},
		{name: "generate documentation", text: "generate documentation for the API", want: false},
		{name: "render the page", text: "render the page in dark mode", want: false},
		{name: "render html", text: "render html from the template", want: false},
		{name: "render a component", text: "render a component in React", want: false},
		{name: "illustrate my point", text: "illustrate my point with an example", want: false},
		{name: "illustrate the concept", text: "illustrate the concept of recursion", want: false},
		{name: "illustrate how", text: "illustrate how this algorithm works", want: false},
		{name: "illustrate the difference", text: "illustrate the difference between the two", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := LooksLikeImageGeneration(tt.text); got != tt.want {
				t.Fatalf("LooksLikeImageGeneration(%q) = %v, want %v", tt.text, got, tt.want)
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
		{name: "edit the colors", text: "edit the colors in this photo", want: true},
		{name: "replace the sky", text: "replace the sky with a sunset", want: true},
		{name: "add sunglasses", text: "add sunglasses", want: true},
		{name: "make it brighter", text: "make it brighter and warmer", want: true},
		{name: "turn it into cartoon", text: "turn it into a cartoon", want: true},
		{name: "remove background phrase", text: "please remove background", want: true},
		{name: "make it realistic phrase", text: "make it realistic", want: true},
		{name: "plain text", text: "tell me about editing", want: false},
		{name: "empty string", text: "", want: false},

		// False positives that should now be rejected
		{name: "remove doubt", text: "remove doubt from my mind", want: false},
		{name: "change topic", text: "change topic please", want: false},
		{name: "change the subject", text: "change the subject, let's talk about AI", want: false},
		{name: "change your mind", text: "change your mind about the approach", want: false},
		{name: "edit the file", text: "edit the file to fix the bug", want: false},
		{name: "edit the code", text: "edit the code for the login page", want: false},
		{name: "remove the bug", text: "remove the bug from the function", want: false},
		{name: "remove duplicates", text: "remove duplicates from the array", want: false},
		{name: "replace the text", text: "replace the text with a new version", want: false},
		{name: "replace the variable", text: "replace the variable name", want: false},
		{name: "add a comment", text: "add a comment to the function", want: false},
		{name: "add to the list", text: "add to the list of tasks", want: false},
		{name: "add a feature", text: "add a feature for dark mode", want: false},
		{name: "make it work", text: "make it work properly", want: false},
		{name: "make it faster", text: "make it faster", want: false},
		{name: "make it compile", text: "make it compile without errors", want: false},
		{name: "turn it off", text: "turn it off", want: false},
		{name: "turn it on", text: "turn it on please", want: false},
		{name: "turn it around", text: "turn it around", want: false},
		{name: "change settings", text: "change settings for the bot", want: false},
		{name: "change the code", text: "change the code to use a map", want: false},
		{name: "add logging", text: "add logging to the handler", want: false},
		{name: "add error handling", text: "add error handling to the function", want: false},
		{name: "remove whitespace", text: "remove whitespace from the string", want: false},
		{name: "edit my code", text: "edit my code to be more efficient", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := LooksLikeImageEdit(tt.text); got != tt.want {
				t.Fatalf("LooksLikeImageEdit(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestIsReplyToPhoto(t *testing.T) {
	t.Parallel()

	t.Run("nil ReplyToMessage", func(t *testing.T) {
		t.Parallel()
		msg := &tgbotapi.Message{ReplyToMessage: nil}
		if IsReplyToPhoto(msg) {
			t.Fatalf("expected false for nil ReplyToMessage")
		}
	})

	t.Run("empty Photo slice", func(t *testing.T) {
		t.Parallel()
		msg := &tgbotapi.Message{
			ReplyToMessage: &tgbotapi.Message{Photo: nil},
		}
		if IsReplyToPhoto(msg) {
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
		if !IsReplyToPhoto(msg) {
			t.Fatalf("expected true for non-empty Photo slice")
		}
	})
}

func TestTextMatcherGuardedPrefixes(t *testing.T) {
	t.Parallel()

	m := textMatcher{
		prefixes: []string{"create image "},
		guardedPrefixes: []guardedPrefix{
			{
				prefix:         "draw ",
				rejectSuffixes: []string{"conclusion", "attention", "the line"},
			},
		},
		phrases: []string{"make me a picture"},
	}

	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "unconditional prefix still works", text: "create image of a dog", want: true},
		{name: "guarded prefix accepts valid input", text: "draw a beautiful sunset", want: true},
		{name: "guarded prefix rejects conclusion", text: "draw conclusions from this", want: false},
		{name: "guarded prefix rejects attention", text: "draw attention to the problem", want: false},
		{name: "guarded prefix rejects the line", text: "draw the line here", want: false},
		{name: "reject is prefix-matched on remainder", text: "draw conclusive evidence shows", want: true},
		{name: "guarded reject does not block phrases", text: "draw conclusions but make me a picture", want: true},
		{name: "phrase works independently", text: "can you make me a picture of a cat", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := m.matches(tt.text); got != tt.want {
				t.Fatalf("matches(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
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

func FuzzExtractImageSize(f *testing.F) {
	f.Add("draw hedgehog 1024x1536 please")
	f.Add("1024X1024 draw: poster")
	f.Add("фото: добавь фон 1536х1024")
	f.Add("правь: убери фон 1024Х1536")
	f.Add("нарисуй ежика")
	f.Add("")
	f.Add("100x200")
	f.Add("1x1")
	f.Add("99999x99999")
	f.Add("1024 x 1536")
	f.Add("abc 1024х1024 def 1536x1024 ghi")

	f.Fuzz(func(t *testing.T, input string) {
		cleaned, size := ExtractImageSize(input)

		if input == "" {
			if cleaned != "" || size != "" {
				t.Fatalf("empty input should return empty results, got (%q, %q)", cleaned, size)
			}
			return
		}

		if size != "" {
			if len(size) < 3 {
				t.Fatalf("extracted size %q is too short", size)
			}
			if cleaned == input {
				t.Fatalf("size %q was extracted but cleaned text equals original", size)
			}
		}

		_ = cleaned
	})
}

func FuzzParseExplicitImageCommand(f *testing.F) {
	f.Add("img: add sharp teeth")
	f.Add("image: generate something")
	f.Add("фото: дорисуй клыки")
	f.Add("edit: remove background")
	f.Add("правь: убери фон")
	f.Add("draw: neon poster")
	f.Add("gen: landscape")
	f.Add("расскажи анекдот")
	f.Add("")
	f.Add("IMG: UPPERCASE")
	f.Add("img:")
	f.Add("  img:  spaced  ")
	f.Add("draw:draw:draw:")

	f.Fuzz(func(t *testing.T, input string) {
		cmd, ok := ParseExplicitImageCommand(input)

		if !ok {
			return
		}

		switch cmd.Mode {
		case ai.ModeEditImage, ai.ModeGenerateImage:
		default:
			t.Fatalf("unexpected mode %q for input %q", cmd.Mode, input)
		}
	})
}

func FuzzTextMatcherMatches(f *testing.F) {
	f.Add("нарисуй кота")
	f.Add("draw me a cat")
	f.Add("draw conclusions from the data")
	f.Add("generate image of sunset")
	f.Add("generate a report for last month")
	f.Add("edit the colors in this photo")
	f.Add("edit the file to fix the bug")
	f.Add("remove background")
	f.Add("remove doubt from my mind")
	f.Add("change the background to blue")
	f.Add("change topic please")
	f.Add("add sunglasses")
	f.Add("add a comment to the function")
	f.Add("make it brighter and warmer")
	f.Add("make it work properly")
	f.Add("turn it into a cartoon")
	f.Add("turn it off")
	f.Add("illustrate a fairy tale")
	f.Add("illustrate my point")
	f.Add("render a 3d scene")
	f.Add("render the page in dark mode")
	f.Add("edit the image")
	f.Add("добавь ему очки")
	f.Add("добавь пример к ответу")
	f.Add("")

	f.Fuzz(func(t *testing.T, input string) {
		_ = LooksLikeImageGeneration(input)
		_ = LooksLikeImageEdit(input)
		_ = LooksLikeExplicitImageEdit(input)
	})
}
