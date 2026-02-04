package ai

import (
	"context"

	"github.com/user/tgbot/internal/session"
)

// Provider defines the interface for AI providers
type Provider interface {
	// Complete generates a response for the given conversation
	Complete(ctx context.Context, messages []session.Message) (string, error)

	// ModelName returns the name of the current model
	ModelName() string
}
