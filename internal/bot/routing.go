package bot

import (
    "regexp"
    "strings"

    tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
    "github.com/user/tgbot/internal/ai"
)

var imageSizePattern = regexp.MustCompile(`(?i)(\d{3,4})\s*[xх]\s*(\d{3,4})`)

func extractImageSize(text string) (string, string) {
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

func isSupportedImageSize(size string) bool {
    switch size {
    case "", "1024x1024", "1024x1536", "1536x1024":
        return true
    default:
        return false
    }
}

type explicitImageCommand struct {
    Mode               ai.RequestMode
    Prompt             string
    FallbackToGenerate bool
}

func parseExplicitImageCommand(text string) (explicitImageCommand, bool) {
    trimmed := strings.TrimSpace(text)
    if trimmed == "" {
        return explicitImageCommand{}, false
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
            return explicitImageCommand{
                Mode:               candidate.mode,
                Prompt:             prompt,
                FallbackToGenerate: candidate.fallbackToGenerate,
            }, true
        }
    }

    return explicitImageCommand{}, false
}

type textMatcher struct {
    excludes []string
    prefixes []string
    phrases  []string
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
        "draw ",
        "draw me",
        "generate ",
        "generate image",
        "create image",
        "create an image",
        "make an image",
        "illustrate ",
        "render ",
    },
    phrases: []string{
        "сгенерируй изображение",
        "generate an image",
        "make me an image",
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
        "edit ",
        "edit this",
        "remove ",
        "replace ",
        "change ",
        "add ",
        "make it ",
        "turn it ",
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

func looksLikeImageGeneration(text string) bool  { return imageGenerationMatcher.matches(text) }
func looksLikeImageEdit(text string) bool         { return imageEditMatcher.matches(text) }
func looksLikeExplicitImageEdit(text string) bool { return explicitImageEditMatcher.matches(text) }

func isReplyToPhoto(msg *tgbotapi.Message) bool {
    return msg.ReplyToMessage != nil && len(msg.ReplyToMessage.Photo) > 0
}
