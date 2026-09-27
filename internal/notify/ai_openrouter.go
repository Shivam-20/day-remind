package notify

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	openrouter "github.com/OpenRouterTeam/go-sdk"
	"github.com/OpenRouterTeam/go-sdk/models/components"
	"github.com/OpenRouterTeam/go-sdk/optionalnullable"
)

const aiCallTimeout = 3 * time.Minute

const openRouterDefaultModel = "google/gemini-2.5-flash"

func generateWordingOpenRouter(ctx context.Context, cfg AIConfig, brief string) (Wording, error) {
	cfg = cfg.normalized()
	if cfg.APIKey == "" {
		return Wording{}, fmt.Errorf("ai generation is not configured")
	}
	model := cfg.Model
	if model == "" {
		model = openRouterDefaultModel
	}

	client := openrouter.New(openRouterOptions(cfg)...)

	temp := optionalnullable.From(openrouter.Pointer(0.9))
	req := components.ChatRequest{
		Model: openrouter.Pointer(model),
		Messages: []components.ChatMessages{
			components.CreateChatMessagesUser(
				components.ChatUserMessage{
					Role: components.ChatUserMessageRoleUser,
					Content: components.CreateChatUserMessageContentStr(
						aiWordingPrompt + brief,
					),
				},
			),
		},
		Temperature: temp,
	}

	res, err := client.Chat.Send(ctx, req, nil)
	if err != nil {
		return Wording{}, fmt.Errorf("could not reach the AI service: %w", err)
	}
	if res == nil || res.ChatResult == nil {
		return Wording{}, fmt.Errorf("unexpected ai response")
	}
	choices := res.ChatResult.GetChoices()
	if len(choices) == 0 {
		return Wording{}, fmt.Errorf("unexpected ai response")
	}
	text := assistantMessageText(choices[0].GetMessage())
	if strings.TrimSpace(text) == "" {
		return Wording{}, fmt.Errorf("unexpected ai response")
	}
	return ParseAIWording(text)
}

func assistantMessageText(msg components.ChatAssistantMessage) string {
	content, ok := msg.GetContent().GetOrZero()
	if !ok {
		return ""
	}
	if content.Str != nil {
		return *content.Str
	}
	return ""
}

func explainAITimeout(err error) error {
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("the AI request timed out — try again or choose a faster model")
}

func openRouterOptions(cfg AIConfig) []openrouter.SDKOption {
	opts := []openrouter.SDKOption{
		openrouter.WithSecurity(cfg.APIKey),
		openrouter.WithTimeout(aiCallTimeout),
		openrouter.WithClient(&http.Client{Timeout: aiCallTimeout}),
	}
	if base := cfg.OpenRouterBaseURL; base != "" {
		opts = append(opts, openrouter.WithServerURL(strings.TrimRight(base, "/")))
	}
	return opts
}
