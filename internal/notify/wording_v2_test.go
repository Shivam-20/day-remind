package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRenderWordingV2Fields(t *testing.T) {
	w := Wording{
		EmailSubject: `{{.Title}}`,
		EmailHTML:    `<p>Hi {{.UserName}}, {{.ListName}} / {{.Tags}}</p><a href="{{.ReminderURL}}">open</a>`,
		EmailText:    `Today is {{.Today}}`,
		TelegramText: `in {{.LeadTime}}: {{.Title}}`,
	}
	data := SampleWordingData(true)
	data.ReminderURL = "https://app.test/#/reminders?open=abc"
	data.Tags = "utilities, monthly"
	out := RenderWording(w, data)

	if !strings.Contains(out.HTML, "Hi Alex, Bills / utilities, monthly") {
		t.Fatalf("v2 fields missing from HTML: %s", out.HTML)
	}
	if !strings.Contains(out.HTML, `href="https://app.test/#/reminders?open=abc"`) {
		t.Fatalf("ReminderURL not rendered: %s", out.HTML)
	}
	if !strings.Contains(out.Text, "Today is") {
		t.Fatalf("Today missing from text: %s", out.Text)
	}
	if !strings.Contains(out.Telegram, "in 1 hour: Pay the electricity bill") {
		t.Fatalf("LeadTime/Title missing from telegram: %s", out.Telegram)
	}
	if len(out.Problems) != 0 {
		t.Fatalf("unexpected problems: %v", out.Problems)
	}
}

func TestWordingPresetsValidateAndRender(t *testing.T) {
	for _, p := range WordingPresets() {
		if err := ValidateWording(p); err != nil {
			t.Fatalf("preset %q invalid: %v", p.Name, err)
		}
		for _, advance := range []bool{false, true} {
			out := RenderWording(p, SampleWordingData(advance))
			if len(out.Problems) != 0 {
				t.Fatalf("preset %q render problems (advance=%v): %v", p.Name, advance, out.Problems)
			}
			if out.Subject == "" || out.HTML == "" || out.Text == "" || out.Telegram == "" {
				t.Fatalf("preset %q (advance=%v) rendered empty parts: %+v", p.Name, advance, out)
			}
			if strings.Contains(out.Telegram, "&#") {
				t.Fatalf("preset %q telegram text contains HTML entities: %s", p.Name, out.Telegram)
			}
		}
	}
}

func TestDefaultReminderEmail(t *testing.T) {
	data := SampleWordingData(false)
	data.ReminderURL = "https://app.test/#/reminders?open=abc"
	html := DefaultReminderEmail(data)
	for _, want := range []string{"Pay the electricity bill", "Open in Day Remind", "Sent by Day Remind", "https://app.test"} {
		if !strings.Contains(html, want) {
			t.Fatalf("default email missing %q", want)
		}
	}

	noURL := SampleWordingData(false)
	noURL.ReminderURL = ""
	if strings.Contains(DefaultReminderEmail(noURL), "Open in Day Remind") {
		t.Fatal("button must be omitted without ReminderURL")
	}
	if strings.Contains(DefaultReminderEmail(noURL), "<script") {
		t.Fatal("default email must not contain scripts")
	}

	data.Advance = true
	if !strings.Contains(DefaultReminderEmail(data), "Heads up") {
		t.Fatal("advance send should use the Heads up eyebrow")
	}
}

func TestParseAIWording(t *testing.T) {
	t.Run("plain json", func(t *testing.T) {
		raw := `{"name":"Test","email_subject":"s","email_html":"<p>hi</p>","email_css":"","email_text":"hi","telegram_text":"hi"}`
		w, err := ParseAIWording(raw)
		if err != nil || w.Name != "Test" {
			t.Fatalf("parse: %v %+v", err, w)
		}
	})
	t.Run("fenced json with commentary", func(t *testing.T) {
		raw := "Here you go!\n```json\n{\"email_html\":\"<p>x</p>\",\"email_text\":\"x\",\"telegram_text\":\"x\"}\n```\nHope that helps"
		w, err := ParseAIWording(raw)
		if err != nil || w.Name != "AI template" {
			t.Fatalf("parse: %v %+v", err, w)
		}
	})
	t.Run("garbage rejected", func(t *testing.T) {
		if _, err := ParseAIWording("not json at all"); err == nil {
			t.Fatal("expected error for garbage")
		}
	})
	t.Run("empty template rejected", func(t *testing.T) {
		if _, err := ParseAIWording(`{"name":"Empty"}`); err == nil {
			t.Fatal("expected error for empty template")
		}
	})
	t.Run("unknown template action rejected", func(t *testing.T) {
		if _, err := ParseAIWording(`{"email_html":"<p>{{.Bogus}}</p>","email_text":"x"}`); err == nil {
			t.Fatal("expected validation error")
		}
	})
}

// geminiFixture mirrors the web package's non-secret testing convention.
const geminiFixture = "fixture-gemini-value-not-usable"

func TestGenerateWordingWithAI(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("x-goog-api-key")
		if !strings.Contains(r.URL.Path, ":generateContent") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		var req struct {
			Contents []struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"contents"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		brief := req.Contents[0].Parts[0].Text
		if !strings.Contains(brief, "cozy birthday nudge") {
			t.Errorf("brief not forwarded in prompt")
		}
		resp := map[string]any{
			"candidates": []map[string]any{{
				"content": map[string]any{
					"parts": []map[string]any{{
						"text": `{"name":"Birthday","email_subject":"Birthday: {{.Title}}","email_html":"<p>{{.UserName}}</p>","email_css":"","email_text":"{{.Title}}","telegram_text":"<b>{{.Title}}</b>"}`,
					}},
				},
			}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cfg := AIConfig{
		Provider:      "gemini",
		APIKey:        geminiFixture,
		Model:         "fixture-model",
		GeminiBaseURL: srv.URL,
	}
	w, err := GenerateWordingWithAI(context.Background(), cfg, "A cozy birthday nudge")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if gotAuth != geminiFixture {
		t.Fatal("api key not sent")
	}
	if w.Name != "Birthday" || !strings.Contains(w.EmailSubject, "{{.Title}}") {
		t.Fatalf("unexpected wording: %+v", w)
	}
}

func TestGenerateWordingWithAIRetriesOnServerError(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cfg := AIConfig{Provider: "gemini", APIKey: geminiFixture, GeminiBaseURL: srv.URL}
	_, err := GenerateWordingWithAI(context.Background(), cfg, "brief")
	if err == nil {
		t.Fatal("expected error when the service keeps failing")
	}
	if calls != 2 {
		t.Fatalf("expected exactly one retry (2 calls), got %d", calls)
	}
}

func TestGenerateWordingWithoutKeyFails(t *testing.T) {
	if _, err := GenerateWordingWithAI(context.Background(), AIConfig{Provider: "gemini"}, "brief"); err == nil {
		t.Fatal("expected not-configured error")
	}
}

func TestGenerateWordingWithOpenRouter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			t.Errorf("expected bearer auth, got %q", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"role":    "assistant",
					"content": `{"name":"OR","email_subject":"Hi {{.Title}}","email_html":"<p>x</p>","email_css":"","email_text":"x","telegram_text":"<b>x</b>"}`,
				},
			}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cfg := AIConfig{
		Provider:          "openrouter",
		APIKey:            "sk-or-test",
		Model:             "openai/gpt-4o-mini",
		OpenRouterBaseURL: srv.URL,
	}
	w, err := GenerateWordingWithAI(context.Background(), cfg, "minimal template")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if w.Name != "OR" {
		t.Fatalf("unexpected wording: %+v", w)
	}
}

func TestAIConfigFromEnvPrefersOpenRouterWhenKeySet(t *testing.T) {
	cfg := AIConfigFromEnv("", "", "", "or-key", "google/gemini-2.5-flash")
	if cfg.Provider != "openrouter" || cfg.APIKey != "or-key" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}
