package notify

import (
	"strings"

	"github.com/shivam-20/day-remind/internal/config"
	"github.com/shivam-20/day-remind/internal/storage"
)

// AIConfig selects the LLM backend for mail wording generation and future AI features.
type AIConfig struct {
	Provider string // "gemini" or "openrouter"
	APIKey   string
	Model    string
	// GeminiBaseURL overrides Google's API host in tests only.
	GeminiBaseURL string
	// OpenRouterBaseURL overrides the OpenRouter API base in tests only.
	OpenRouterBaseURL string
}

// AIConfigFromEnv builds the active LLM config from application env values.
// When AI_PROVIDER is empty, OpenRouter is chosen if OPENROUTER_API_KEY is set.
// EffectiveAI merges DB-stored admin settings with env fallbacks.
func EffectiveAI(cfg config.Config, stored storage.AISettings) AIConfig {
	provider := strings.TrimSpace(stored.Provider)
	if provider == "" {
		provider = strings.TrimSpace(cfg.AIProvider)
	}
	geminiKey := strings.TrimSpace(stored.GeminiAPIKey)
	if geminiKey == "" {
		geminiKey = strings.TrimSpace(cfg.GeminiAPIKey)
	}
	geminiModel := strings.TrimSpace(stored.GeminiModel)
	if geminiModel == "" {
		geminiModel = strings.TrimSpace(cfg.GeminiModel)
	}
	openRouterKey := strings.TrimSpace(stored.OpenRouterAPIKey)
	if openRouterKey == "" {
		openRouterKey = strings.TrimSpace(cfg.OpenRouterAPIKey)
	}
	openRouterModel := strings.TrimSpace(stored.OpenRouterModel)
	if openRouterModel == "" {
		openRouterModel = strings.TrimSpace(cfg.OpenRouterModel)
	}
	return AIConfigFromEnv(provider, geminiKey, geminiModel, openRouterKey, openRouterModel)
}

func AIConfigFromEnv(provider, geminiKey, geminiModel, openRouterKey, openRouterModel string) AIConfig {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		if strings.TrimSpace(openRouterKey) != "" {
			provider = "openrouter"
		} else {
			provider = "gemini"
		}
	}
	switch provider {
	case "openrouter":
		return AIConfig{
			Provider: "openrouter",
			APIKey:   openRouterKey,
			Model:    openRouterModel,
		}
	default:
		return AIConfig{
			Provider: "gemini",
			APIKey:   geminiKey,
			Model:    geminiModel,
		}
	}
}

func (c AIConfig) normalized() AIConfig {
	out := c
	out.Provider = strings.ToLower(strings.TrimSpace(out.Provider))
	if out.Provider == "" {
		out.Provider = "gemini"
	}
	out.APIKey = strings.TrimSpace(out.APIKey)
	out.Model = strings.TrimSpace(out.Model)
	out.GeminiBaseURL = strings.TrimSpace(out.GeminiBaseURL)
	out.OpenRouterBaseURL = strings.TrimSpace(out.OpenRouterBaseURL)
	return out
}
