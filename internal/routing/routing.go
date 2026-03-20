package routing

import (
	"regexp"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/user/tgbot/internal/ai"
)

var imageSizePattern = regexp.MustCompile(`(?i)(\d{3,4})\s*[xх]\s*(\d{3,4})`)

func ExtractImageSize(text string) (string, string) {
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

func IsSupportedImageSize(size string) bool {
	switch size {
	case "", "1024x1024", "1024x1536", "1536x1024":
		return true
	default:
		return false
	}
}

type ExplicitImageCommand struct {
	Mode               ai.RequestMode
	Prompt             string
	FallbackToGenerate bool
}

func ParseExplicitImageCommand(text string) (ExplicitImageCommand, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ExplicitImageCommand{}, false
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
			return ExplicitImageCommand{
				Mode:               candidate.mode,
				Prompt:             prompt,
				FallbackToGenerate: candidate.fallbackToGenerate,
			}, true
		}
	}

	return ExplicitImageCommand{}, false
}

type guardedPrefix struct {
	prefix         string
	rejectSuffixes []string
}

type textMatcher struct {
	excludes        []string
	prefixes        []string
	guardedPrefixes []guardedPrefix
	phrases         []string
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
	for _, gp := range m.guardedPrefixes {
		if strings.HasPrefix(lower, gp.prefix) {
			suffix := strings.TrimSpace(lower[len(gp.prefix):])
			rejected := false
			for _, rs := range gp.rejectSuffixes {
				if strings.HasPrefix(suffix, rs) {
					rejected = true
					break
				}
			}
			if !rejected {
				return true
			}
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
		"generate image",
		"generate an image",
		"create image",
		"create an image",
		"create a picture",
		"make an image",
		"make a picture",
	},
	guardedPrefixes: []guardedPrefix{
		{
			prefix: "draw ",
			rejectSuffixes: []string{
				"conclusion", "attention",
				"the line", "a line between",
				"a blank", "a breath",
				"lots", "upon", "near", "close",
				"a comparison", "a parallel", "a distinction",
				"an analogy", "an inference",
			},
		},
		{
			prefix: "generate ",
			rejectSuffixes: []string{
				"a report", "the report",
				"code", "the code",
				"a list", "the list",
				"revenue", "income",
				"text ", "the text",
				"a response", "the response",
				"a summary", "the summary",
				"documentation", "docs",
				"a function", "a class",
				"a file", "the file",
				"a query", "the query",
				"sql", "json", "csv", "html",
			},
		},
		{
			prefix: "render ",
			rejectSuffixes: []string{
				"the page", "a page", "this page",
				"html", "the template", "a template",
				"the component", "a component",
				"the view", "a view",
				"the form", "a form",
				"jsx", "tsx", "css",
				"assistance", "a verdict",
				"useless", "obsolete",
			},
		},
		{
			prefix: "illustrate ",
			rejectSuffixes: []string{
				"my point", "the point", "a point",
				"your point", "this point",
				"the concept", "a concept", "this concept",
				"how ", "why ", "what ", "that ",
				"the difference", "the idea", "this idea",
				"an example", "the example",
				"the problem", "the issue",
			},
		},
	},
	phrases: []string{
		"сгенерируй изображение",
		"generate an image",
		"make me an image",
		"make me a picture",
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
	},
	guardedPrefixes: []guardedPrefix{
		{
			prefix: "edit ",
			rejectSuffixes: []string{
				"the file", "this file", "that file",
				"the code", "this code", "that code",
				"the text", "this text", "that text",
				"the document", "this document",
				"the function", "this function",
				"the config", "the settings",
				"the script", "this script",
				"the template", "this template",
				"your ", "my ",
			},
		},
		{
			prefix: "remove ",
			rejectSuffixes: []string{
				"doubt", "from my mind",
				"from the list", "from the array",
				"the bug", "a bug",
				"the error", "an error",
				"duplicates", "whitespace",
				"the code", "this code",
				"the function", "this function",
				"the file", "this file",
				"the class", "this class",
				"the variable", "this variable",
				"the import", "the test",
			},
		},
		{
			prefix: "replace ",
			rejectSuffixes: []string{
				"the text", "this text",
				"the code", "this code",
				"the variable", "this variable",
				"the function", "this function",
				"the value", "this value",
				"the string", "this string",
				"the word", "this word",
				"the class", "this class",
				"the file", "this file",
				"the name", "this name",
				"the method", "this method",
			},
		},
		{
			prefix: "change ",
			rejectSuffixes: []string{
				"topic", "the topic",
				"subject", "the subject",
				"your mind", "my mind", "his mind", "her mind",
				"direction", "the direction",
				"the setting", "settings",
				"the code", "this code",
				"the plan", "the approach",
				"the variable", "the function",
				"the file", "the method",
				"the password", "the key",
				"the config",
				"course",
			},
		},
		{
			prefix: "add ",
			rejectSuffixes: []string{
				"to the list", "to the array",
				"to the cart", "to the queue",
				"to the database", "to the db",
				"a comment", "the comment",
				"a note", "the note",
				"a function", "the function",
				"a feature", "the feature",
				"a class", "the class",
				"a test", "the test",
				"a method", "the method",
				"a file", "the file",
				"a variable", "the variable",
				"a dependency",
				"support for",
				"logging", "error handling",
				"validation",
				"up ",
				"more detail", "more context",
			},
		},
		{
			prefix: "make it ",
			rejectSuffixes: []string{
				"work", "stop", "run", "compile", "pass",
				"happen", "possible", "impossible",
				"faster", "slower", "quicker",
				"easier", "simpler", "harder",
				"right", "correct",
				"public", "private", "static",
				"async", "generic", "abstract",
				"thread-safe", "immutable",
			},
		},
		{
			prefix: "turn it ",
			rejectSuffixes: []string{
				"off", "on", "around", "down", "up",
				"back", "over", "in ", "out ",
			},
		},
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

func LooksLikeImageGeneration(text string) bool   { return imageGenerationMatcher.matches(text) }
func LooksLikeImageEdit(text string) bool         { return imageEditMatcher.matches(text) }
func LooksLikeExplicitImageEdit(text string) bool { return explicitImageEditMatcher.matches(text) }

func IsReplyToPhoto(msg *tgbotapi.Message) bool {
	return msg.ReplyToMessage != nil && len(msg.ReplyToMessage.Photo) > 0
}
