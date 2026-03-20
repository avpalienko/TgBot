package ai

import "context"

// Message represents a chat message at the AI provider boundary.
type Message struct {
	Role      string // "user" or "assistant"
	Content   string
	ImageData string // optional: base64 data URI for images
}

type RequestMode string

const (
	ModeChat          RequestMode = "chat"
	ModeGenerateImage RequestMode = "generate_image"
	ModeEditImage     RequestMode = "edit_image"
)

type Request struct {
	Mode               RequestMode
	Text               string
	ImageSize          string
	History            []Message
	InputImageData     string
	PreviousResponseID string
}

type Result struct {
	Text          string
	ImageBytes    []byte
	ImageMimeType string
	ResponseID    string
}

// Provider defines the interface for AI providers.
type Provider interface {
	// Respond generates a response for the given request.
	Respond(ctx context.Context, req Request) (Result, error)

	// ModelName returns the name of the current model.
	ModelName() string
}
