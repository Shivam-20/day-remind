package notify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shivam-20/day-remind/internal/config"
	"github.com/shivam-20/day-remind/internal/storage"
)

func TestEffectiveAIStoredOverridesEnv(t *testing.T) {
	cfg := config.Config{
		AIProvider:       "gemini",
		OpenRouterAPIKey: "env-or",
		GeminiAPIKey:     "env-gem",
	}
	stored := storage.AISettings{
		Provider:         "openrouter",
		OpenRouterAPIKey: "db-or",
	}
	ai := EffectiveAI(cfg, stored)
	if ai.Provider != "openrouter" || ai.APIKey != "db-or" {
		t.Fatalf("unexpected effective ai: %+v", ai)
	}
}

func TestPingGemini(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "gem-key" {
			t.Errorf("missing key")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`))
	}))
	defer srv.Close()
	reply, err := PingAI(context.Background(), AIConfig{
		Provider:      "gemini",
		APIKey:        "gem-key",
		Model:         "gemini-2.5-flash",
		GeminiBaseURL: srv.URL,
	})
	if err != nil || reply != "ok" {
		t.Fatalf("reply=%q err=%v", reply, err)
	}
}
