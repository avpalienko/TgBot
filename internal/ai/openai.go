package ai

import (
	"context"
	"fmt"

	"github.com/sashabaranov/go-openai"
	"github.com/user/tgbot/internal/session"
)

// OpenAIProvider implements Provider for OpenAI API
type OpenAIProvider struct {
	client *openai.Client
	model  string
}

// NewOpenAIProvider creates a new OpenAI provider
func NewOpenAIProvider(apiKey, model, baseURL string) *OpenAIProvider {
	config := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		config.BaseURL = baseURL
	}

	return &OpenAIProvider{
		client: openai.NewClientWithConfig(config),
		model:  model,
	}
}

// Complete sends messages to OpenAI and returns the response
func (p *OpenAIProvider) Complete(ctx context.Context, messages []session.Message) (string, error) {
	// Convert session messages to OpenAI format
	chatMessages := make([]openai.ChatCompletionMessage, len(messages))
	for i, msg := range messages {
		if msg.ImageData != "" {
			// Multimodal message: optional text + image
			parts := []openai.ChatMessagePart{}
			if msg.Content != "" {
				parts = append(parts, openai.ChatMessagePart{
					Type: openai.ChatMessagePartTypeText,
					Text: msg.Content,
				})
			}
			parts = append(parts, openai.ChatMessagePart{
				Type: openai.ChatMessagePartTypeImageURL,
				ImageURL: &openai.ChatMessageImageURL{
					URL:    msg.ImageData,
					Detail: openai.ImageURLDetailAuto,
				},
			})
			chatMessages[i] = openai.ChatCompletionMessage{
				Role:         msg.Role,
				MultiContent: parts,
			}
		} else {
			chatMessages[i] = openai.ChatCompletionMessage{
				Role:    msg.Role,
				Content: msg.Content,
			}
		}
	}

	resp, err := p.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:    p.model,
		Messages: chatMessages,
	})
	if err != nil {
		return "", fmt.Errorf("OpenAI API error: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response from OpenAI")
	}

	return resp.Choices[0].Message.Content, nil
}

// ModelName returns the current model name
func (p *OpenAIProvider) ModelName() string {
	return p.model
}
