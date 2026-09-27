package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	openrouter "github.com/OpenRouterTeam/go-sdk"
	"github.com/OpenRouterTeam/go-sdk/models/components"
)

const aiPingPrompt = "Reply with the single word ok."

// PingAI sends a one-word prompt to confirm the key and model work.
func PingAI(ctx context.Context, cfg AIConfig) (string, error) {
	cfg = cfg.normalized()
	if cfg.APIKey == "" {
		return "", fmt.Errorf("ai generation is not configured")
	}
	switch cfg.Provider {
	case "openrouter":
		return pingOpenRouter(ctx, cfg)
	default:
		return pingGemini(ctx, cfg)
	}
}

func pingOpenRouter(ctx context.Context, cfg AIConfig) (string, error) {
	model := cfg.Model
	if model == "" {
		model = openRouterDefaultModel
	}
	client := openrouter.New(openRouterOptions(cfg)...)
	res, err := client.Chat.Send(ctx, components.ChatRequest{
		Model: openrouter.Pointer(model),
		Messages: []components.ChatMessages{
			components.CreateChatMessagesUser(components.ChatUserMessage{
				Role:    components.ChatUserMessageRoleUser,
				Content: components.CreateChatUserMessageContentStr(aiPingPrompt),
			}),
		},
	}, nil)
	if err != nil {
		return "", explainAITimeout(fmt.Errorf("could not reach the AI service: %w", err))
	}
	if res == nil || res.ChatResult == nil || len(res.ChatResult.GetChoices()) == 0 {
		return "", fmt.Errorf("unexpected ai response")
	}
	text := strings.TrimSpace(assistantMessageText(res.ChatResult.GetChoices()[0].GetMessage()))
	if text == "" {
		return "", fmt.Errorf("unexpected ai response")
	}
	return text, nil
}

func pingGemini(ctx context.Context, cfg AIConfig) (string, error) {
	model := cfg.Model
	if model == "" {
		model = "gemini-2.5-flash"
	}
	baseURL := cfg.GeminiBaseURL
	if baseURL == "" {
		baseURL = geminiBaseURL
	}
	payload := map[string]any{
		"contents": []map[string]any{
			{"parts": []map[string]any{{"text": aiPingPrompt}}},
		},
		"generationConfig": map[string]any{"maxOutputTokens": 16},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/v1beta/models/%s:generateContent", strings.TrimRight(baseURL, "/"), model), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", cfg.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", explainAITimeout(fmt.Errorf("could not reach the AI service: %w", err))
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ai service returned %d", resp.StatusCode)
	}
	var out struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Candidates) == 0 {
		return "", fmt.Errorf("unexpected ai response")
	}
	var text string
	for _, p := range out.Candidates[0].Content.Parts {
		text += p.Text
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("unexpected ai response")
	}
	return text, nil
}
