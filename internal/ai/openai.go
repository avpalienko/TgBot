package ai

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/responses"
	"github.com/user/tgbot/internal/session"
)

const (
	chatInstructions = "You are a helpful Telegram bot assistant. Answer naturally and concisely. If the user shares a photo, analyze it directly."
	imageModel       = "gpt-image-1"
	imageMimeTypePNG = "image/png"
)

// OpenAIProvider implements Provider for OpenAI Responses API.
type OpenAIProvider struct {
	client openai.Client
	model  string
}

// NewOpenAIProvider creates a new OpenAI provider.
func NewOpenAIProvider(apiKey, model, baseURL string) *OpenAIProvider {
	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}

	return &OpenAIProvider{
		client: openai.NewClient(opts...),
		model:  model,
	}
}

// Respond sends a request to OpenAI Responses API and returns the parsed result.
func (p *OpenAIProvider) Respond(ctx context.Context, req Request) (Result, error) {
	switch req.Mode {
	case "", ModeChat:
		return p.respondChat(ctx, req)
	case ModeGenerateImage:
		return p.respondWithImageTool(ctx, req, false)
	case ModeEditImage:
		return p.respondWithImageTool(ctx, req, true)
	default:
		return Result{}, fmt.Errorf("unsupported request mode: %s", req.Mode)
	}
}

// ModelName returns the current model name.
func (p *OpenAIProvider) ModelName() string {
	return p.model
}

func (p *OpenAIProvider) respondChat(ctx context.Context, req Request) (Result, error) {
	messages := req.History
	if req.PreviousResponseID != "" && len(messages) > 0 {
		messages = messages[len(messages)-1:]
	}

	input := buildHistoryInput(messages)
	if len(input) == 0 {
		return Result{}, fmt.Errorf("chat request is empty")
	}

	params := responses.ResponseNewParams{
		Model:        responses.ResponsesModel(p.model),
		Store:        openai.Bool(true),
		Truncation:   responses.ResponseNewParamsTruncationAuto,
		Instructions: openai.String(chatInstructions),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: input,
		},
	}
	if req.PreviousResponseID != "" {
		params.PreviousResponseID = openai.String(req.PreviousResponseID)
	}

	resp, err := p.client.Responses.New(ctx, params)
	if err != nil {
		return Result{}, fmt.Errorf("OpenAI Responses API error: %w", err)
	}

	return parseResponse(resp)
}

func (p *OpenAIProvider) respondWithImageTool(ctx context.Context, req Request, isEdit bool) (Result, error) {
	if isEdit && req.InputImageData == "" {
		return Result{}, fmt.Errorf("image edit request requires input image data")
	}

	input := buildImageToolInput(req.Text, req.InputImageData, isEdit)
	tool := responses.ToolImageGenerationParam{
		Model:        imageModel,
		OutputFormat: "png",
		Quality:      "auto",
		Size:         "auto",
		Background:   "auto",
	}
	if req.ImageSize != "" {
		tool.Size = req.ImageSize
	}
	if isEdit {
		tool.InputFidelity = "high"
	}

	params := responses.ResponseNewParams{
		Model:        responses.ResponsesModel(p.model),
		Store:        openai.Bool(true),
		Truncation:   responses.ResponseNewParamsTruncationAuto,
		Instructions: openai.String(imageInstructions(isEdit)),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: input,
		},
		MaxToolCalls: openai.Int(1),
		Tools: []responses.ToolUnionParam{
			{OfImageGeneration: &tool},
		},
		ToolChoice: responses.ResponseNewParamsToolChoiceUnion{
			OfHostedTool: &responses.ToolChoiceTypesParam{
				Type: responses.ToolChoiceTypesTypeImageGeneration,
			},
		},
	}
	if req.PreviousResponseID != "" {
		params.PreviousResponseID = openai.String(req.PreviousResponseID)
	}

	resp, err := p.client.Responses.New(ctx, params)
	if err != nil {
		return Result{}, fmt.Errorf("OpenAI Responses API error: %w", err)
	}

	return parseResponse(resp)
}

func buildHistoryInput(messages []session.Message) responses.ResponseInputParam {
	input := make(responses.ResponseInputParam, 0, len(messages))
	for _, msg := range messages {
		item, ok := buildMessageInput(msg)
		if !ok {
			continue
		}
		input = append(input, item)
	}
	return input
}

func buildMessageInput(msg session.Message) (responses.ResponseInputItemUnionParam, bool) {
	role := normalizeRole(msg.Role)

	if msg.ImageData == "" {
		if msg.Content == "" {
			return responses.ResponseInputItemUnionParam{}, false
		}
		return responses.ResponseInputItemParamOfMessage(msg.Content, role), true
	}

	if role != responses.EasyInputMessageRoleUser {
		if msg.Content == "" {
			return responses.ResponseInputItemUnionParam{}, false
		}
		return responses.ResponseInputItemParamOfMessage(msg.Content, role), true
	}

	content := responses.ResponseInputMessageContentListParam{}
	if msg.Content != "" {
		content = append(content, responses.ResponseInputContentUnionParam{
			OfInputText: &responses.ResponseInputTextParam{
				Text: msg.Content,
			},
		})
	}
	content = append(content, responses.ResponseInputContentUnionParam{
		OfInputImage: &responses.ResponseInputImageParam{
			Detail:   responses.ResponseInputImageDetailAuto,
			ImageURL: openai.String(msg.ImageData),
		},
	})

	return responses.ResponseInputItemParamOfMessage(content, role), true
}

func buildImageToolInput(prompt, imageData string, isEdit bool) responses.ResponseInputParam {
	content := responses.ResponseInputMessageContentListParam{
		{
			OfInputText: &responses.ResponseInputTextParam{
				Text: imagePrompt(prompt, isEdit),
			},
		},
	}

	if imageData != "" {
		content = append(content, responses.ResponseInputContentUnionParam{
			OfInputImage: &responses.ResponseInputImageParam{
				Detail:   responses.ResponseInputImageDetailHigh,
				ImageURL: openai.String(imageData),
			},
		})
	}

	return responses.ResponseInputParam{
		responses.ResponseInputItemParamOfMessage(content, responses.EasyInputMessageRoleUser),
	}
}

func imagePrompt(prompt string, isEdit bool) string {
	prompt = strings.TrimSpace(prompt)
	if isEdit {
		if prompt == "" {
			return "Edit the provided image while preserving the main subject."
		}
		return "Edit the provided image according to this request: " + prompt
	}
	if prompt == "" {
		return "Generate an image that matches the user's request."
	}
	return prompt
}

func imageInstructions(isEdit bool) string {
	if isEdit {
		return "You are helping a Telegram user edit an image. Use the image_generation tool to modify the provided image and preserve unchanged details unless the request says otherwise."
	}
	return "You are helping a Telegram user generate an image. Use the image_generation tool to create the requested image."
}

func parseResponse(resp *responses.Response) (Result, error) {
	if resp == nil {
		return Result{}, fmt.Errorf("empty response from OpenAI")
	}

	result := Result{
		Text:       strings.TrimSpace(resp.OutputText()),
		ResponseID: resp.ID,
	}

	for _, item := range resp.Output {
		if item.Type != "image_generation_call" || item.Result == "" {
			continue
		}

		imageBytes, err := base64.StdEncoding.DecodeString(item.Result)
		if err != nil {
			return Result{}, fmt.Errorf("failed to decode generated image: %w", err)
		}

		result.ImageBytes = imageBytes
		result.ImageMimeType = imageMimeTypePNG
		break
	}

	if result.Text == "" && len(result.ImageBytes) == 0 {
		return Result{}, fmt.Errorf("OpenAI returned no text or image output")
	}

	return result, nil
}

func normalizeRole(role string) responses.EasyInputMessageRole {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "assistant":
		return responses.EasyInputMessageRoleAssistant
	case "system":
		return responses.EasyInputMessageRoleSystem
	case "developer":
		return responses.EasyInputMessageRoleDeveloper
	default:
		return responses.EasyInputMessageRoleUser
	}
}
