package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const geminiBaseURL = "https://generativelanguage.googleapis.com"

// aiWordingPrompt describes the contract for the model: exact JSON keys, the
// real template variables, and the design quality bar.
const aiWordingPrompt = `You design reminder notification templates for "Day Remind", a reminder app.
Return ONLY a JSON object (no markdown fences, no commentary) with exactly these keys:
"name", "email_subject", "email_html", "email_css", "email_text", "telegram_text"

Template variables — Go template actions, escape nothing yourself:
{{.Title}} (reminder title), {{.Description}} (details, may be empty), {{.DueAt}} (formatted due date+time),
{{.Timezone}}, {{.Priority}} (low/normal/high/urgent), {{.Advance}} (bool: true for an early-warning send),
{{.LeadTime}} (e.g. "1 hour", empty on the due send), {{.UserName}} (recipient name, may be empty),
{{.ListName}} (list name, may be empty), {{.Tags}} (comma-separated, may be empty),
{{.ReminderURL}} (deep link to the app, may be empty), {{.Today}} (today's date).

Use {{if .Advance}}...{{else}}...{{end}} to differ between the early warning and the due notice.

Content rules:
- email_html: a complete, mobile-friendly HTML email. Table-based layout, all styles INLINE
  (mail clients strip <style> and classes), max-width 560px, system font stack. No <script>.
- email_css: optional small extra stylesheet (rules only, no <style> wrapper). May be "".
- email_text: plain-text alternative.
- telegram_text: short HTML for Telegram (only <b>, <i>, <a> tags; no headings, no tables).
  Use one or two emoji for character.
- email_subject: under 60 characters, uses the advance/due distinction.
- name: 2-4 word label for the template.

Sample data the design must look good with:
Title "Pay the electricity bill", Description "Account 12345", Priority "high",
DueAt "Fri, 25 Dec 2026 09:00 IST", Advance true, LeadTime "1 hour", UserName "Alex", ListName "Bills".

User's brief for this template:
"`

// GenerateWordingWithAI asks the configured LLM (Gemini or OpenRouter) for a
// complete mail wording from a short brief.
func GenerateWordingWithAI(ctx context.Context, cfg AIConfig, brief string) (Wording, error) {
	cfg = cfg.normalized()
	if cfg.APIKey == "" {
		return Wording{}, fmt.Errorf("ai generation is not configured")
	}
	brief = strings.TrimSpace(brief)
	if brief == "" {
		return Wording{}, fmt.Errorf("a short description of the design is required")
	}
	if len(brief) > 500 {
		brief = brief[:500]
	}

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return Wording{}, explainAITimeout(ctx.Err())
			case <-time.After(500 * time.Millisecond):
			}
		}
		var wording Wording
		var err error
		switch cfg.Provider {
		case "openrouter":
			wording, err = generateWordingOpenRouter(ctx, cfg, brief)
		default:
			wording, err = generateWordingGemini(ctx, cfg, brief)
		}
		if err == nil {
			return wording, nil
		}
		lastErr = err
	}
	return Wording{}, explainAITimeout(lastErr)
}

func generateWordingGemini(ctx context.Context, cfg AIConfig, brief string) (Wording, error) {
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
			{"parts": []map[string]any{{"text": aiWordingPrompt + brief}}},
		},
		"generationConfig": map[string]any{
			"temperature":      0.9,
			"maxOutputTokens":  8192,
			"responseMimeType": "application/json",
		},
	}
	body, _ := json.Marshal(payload)
	return generateWordingAttempt(ctx, baseURL, cfg.APIKey, model, body)
}

func generateWordingAttempt(ctx context.Context, baseURL, apiKey, model string, body []byte) (Wording, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/v1beta/models/%s:generateContent", baseURL, model), bytes.NewReader(body))
	if err != nil {
		return Wording{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Wording{}, fmt.Errorf("could not reach the AI service: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return Wording{}, fmt.Errorf("ai service returned %d", resp.StatusCode)
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
	if err := json.Unmarshal(raw, &out); err != nil {
		return Wording{}, fmt.Errorf("unexpected ai response")
	}
	text := ""
	for _, p := range out.Candidates[0].Content.Parts {
		text += p.Text
	}
	return ParseAIWording(text)
}

// ParseAIWording extracts and validates a Wording from a model response.
func ParseAIWording(raw string) (Wording, error) {
	text := strings.TrimSpace(raw)
	if fence := strings.Index(text, "```"); fence >= 0 {
		text = strings.Trim(text, "` \n\t")
		if i := strings.Index(text, "\n"); i >= 0 && !strings.Contains(text[:i], "{") {
			text = text[i+1:]
		}
	}
	if i := strings.Index(text, "{"); i >= 0 {
		if j := strings.LastIndex(text, "}"); j > i {
			text = text[i : j+1]
		}
	}
	var data struct {
		Name         string `json:"name"`
		EmailSubject string `json:"email_subject"`
		EmailHTML    string `json:"email_html"`
		EmailCSS     string `json:"email_css"`
		EmailText    string `json:"email_text"`
		TelegramText string `json:"telegram_text"`
	}
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		return Wording{}, fmt.Errorf("ai returned unusable JSON")
	}
	if strings.TrimSpace(data.EmailHTML) == "" && strings.TrimSpace(data.EmailText) == "" {
		return Wording{}, fmt.Errorf("ai returned an empty template")
	}
	name := strings.TrimSpace(data.Name)
	if name == "" {
		name = "AI template"
	}
	wording := Wording{
		Name:         name,
		EmailSubject: data.EmailSubject,
		EmailHTML:    data.EmailHTML,
		EmailCSS:     data.EmailCSS,
		EmailText:    data.EmailText,
		TelegramText: data.TelegramText,
	}
	if err := ValidateWording(wording); err != nil {
		return Wording{}, fmt.Errorf("ai template failed validation: %w", err)
	}
	return wording, nil
}
