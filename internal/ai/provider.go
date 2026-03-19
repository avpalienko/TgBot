package ai

import (
	"context"

	"github.com/user/tgbot/internal/session"
)

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
	History            []session.Message
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
