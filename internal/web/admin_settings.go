package web

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/shivam-20/day-remind/internal/config"
	"github.com/shivam-20/day-remind/internal/notify"
	"github.com/shivam-20/day-remind/internal/storage"
)

type adminDeliveryUpdateRequest struct {
	EmailEnabled  *bool   `json:"email_enabled"`
	EmailFrom     *string `json:"email_from"`
	EmailProvider *string `json:"email_provider"`
	ResendAPIKey  *string `json:"resend_api_key"`

	SMTPHost     *string `json:"smtp_host"`
	SMTPPort     *int    `json:"smtp_port"`
	SMTPUsername *string `json:"smtp_username"`
	SMTPPassword *string `json:"smtp_password"`
	SMTPTLSMode  *string `json:"smtp_tls_mode"`

	TelegramEnabled  *bool   `json:"telegram_enabled"`
	TelegramBotToken *string `json:"telegram_bot_token"`

	TelegramGatewayURL         *string `json:"telegram_gateway_url"`
	TelegramGatewayAPIToken    *string `json:"telegram_gateway_api_token"`
	TelegramGatewayInboundMode *string `json:"telegram_gateway_inbound_mode"`

	AIProvider         *string `json:"ai_provider"`
	OpenRouterAPIKey   *string `json:"openrouter_api_key"`
	OpenRouterModel    *string `json:"openrouter_model"`
	GeminiAPIKey       *string `json:"gemini_api_key"`
	GeminiModel        *string `json:"gemini_model"`
}

type adminDeliveryTestRequest struct {
	Channel string `json:"channel"`
	To      string `json:"to"`
}

func (s *Server) handleAdminGetDeliverySettings(w http.ResponseWriter, r *http.Request) {
	stored, err := s.db.GetDeliverySettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load delivery settings")
		return
	}
	aiStored, err := s.db.GetAISettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load ai settings")
		return
	}
	effective := notify.EffectiveDelivery(s.cfg, stored)
	writeJSON(w, http.StatusOK, adminDeliveryPayload(stored, effective, aiStored, s.cfg))
}

func (s *Server) handleAdminUpdateDeliverySettings(w http.ResponseWriter, r *http.Request) {
	var body adminDeliveryUpdateRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	cur, err := s.db.GetDeliverySettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load delivery settings")
		return
	}
	if err := validateDeliveryProviderSelection(cur, s.cfg, body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.AIProvider != nil || body.OpenRouterAPIKey != nil || body.OpenRouterModel != nil ||
		body.GeminiAPIKey != nil || body.GeminiModel != nil {
		if _, err := s.db.UpdateAISettings(r.Context(), storage.AISettingsUpdate{
			Provider:         body.AIProvider,
			OpenRouterAPIKey: body.OpenRouterAPIKey,
			OpenRouterModel:  body.OpenRouterModel,
			GeminiAPIKey:     body.GeminiAPIKey,
			GeminiModel:      body.GeminiModel,
		}); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	stored, err := s.db.UpdateDeliverySettings(r.Context(), storage.DeliverySettingsUpdate{
		EmailEnabled:     body.EmailEnabled,
		EmailFrom:        body.EmailFrom,
		EmailProvider:    body.EmailProvider,
		ResendAPIKey:     body.ResendAPIKey,
		SMTPHost:         body.SMTPHost,
		SMTPPort:         body.SMTPPort,
		SMTPUsername:     body.SMTPUsername,
		SMTPPassword:     body.SMTPPassword,
		SMTPTLSMode:      body.SMTPTLSMode,
		TelegramEnabled:  body.TelegramEnabled,
		TelegramBotToken: body.TelegramBotToken,

		TelegramGatewayURL:         body.TelegramGatewayURL,
		TelegramGatewayAPIToken:    body.TelegramGatewayAPIToken,
		TelegramGatewayInboundMode: body.TelegramGatewayInboundMode,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	aiStored, err := s.db.GetAISettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load ai settings")
		return
	}
	effective := notify.EffectiveDelivery(s.cfg, stored)
	writeJSON(w, http.StatusOK, adminDeliveryPayload(stored, effective, aiStored, s.cfg))
}

func (s *Server) handleAdminListOpenRouterModels(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(s.cfg.OpenRouterAPIKey)
	if s.db != nil {
		stored, err := s.db.GetAISettings(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not load ai settings")
			return
		}
		if strings.TrimSpace(stored.OpenRouterAPIKey) != "" {
			key = strings.TrimSpace(stored.OpenRouterAPIKey)
		}
	}
	models, err := notify.ListOpenRouterModels(r.Context(), key, "")
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

type adminAITestRequest struct {
	Provider string `json:"provider"`
	APIKey   string `json:"api_key"`
	Model    string `json:"model"`
}

func (s *Server) handleAdminTestAI(w http.ResponseWriter, r *http.Request) {
	var body adminAITestRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	provider := strings.ToLower(strings.TrimSpace(body.Provider))
	if provider != "openrouter" && provider != "gemini" {
		writeError(w, http.StatusBadRequest, "provider must be openrouter or gemini")
		return
	}
	key := strings.TrimSpace(body.APIKey)
	model := strings.TrimSpace(body.Model)
	var stored storage.AISettings
	if s.db != nil {
		var err error
		stored, err = s.db.GetAISettings(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not load ai settings")
			return
		}
	}
	if provider == "openrouter" {
		if key == "" {
			key = strings.TrimSpace(stored.OpenRouterAPIKey)
		}
		if key == "" {
			key = strings.TrimSpace(s.cfg.OpenRouterAPIKey)
		}
		if model == "" {
			model = strings.TrimSpace(stored.OpenRouterModel)
		}
		if model == "" {
			model = strings.TrimSpace(s.cfg.OpenRouterModel)
		}
	} else {
		if key == "" {
			key = strings.TrimSpace(stored.GeminiAPIKey)
		}
		if key == "" {
			key = strings.TrimSpace(s.cfg.GeminiAPIKey)
		}
		if model == "" {
			model = strings.TrimSpace(stored.GeminiModel)
		}
		if model == "" {
			model = strings.TrimSpace(s.cfg.GeminiModel)
		}
	}
	reply, err := notify.PingAI(r.Context(), notify.AIConfig{
		Provider: provider,
		APIKey:   key,
		Model:    model,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(reply) > 120 {
		reply = reply[:120]
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "reply": reply, "model": model})
}

func (s *Server) handleAdminTestDelivery(w http.ResponseWriter, r *http.Request) {
	var body adminDeliveryTestRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	delivery, err := s.effectiveDelivery(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load delivery settings")
		return
	}
	switch body.Channel {
	case storage.ChannelEmail:
		if err := notify.SendEmail(r.Context(), delivery, body.To, "Day Remind test", "<p>This is a test alert from Day Remind.</p>"); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	case storage.ChannelTelegram:
		if err := notify.SendTelegram(r.Context(), delivery, body.To, "Day Remind test: this is a test alert."); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "channel must be email or telegram")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func adminDeliveryPayload(stored storage.DeliverySettings, effective notify.DeliveryConfig, aiStored storage.AISettings, cfg config.Config) map[string]any {
	aiEffective := notify.EffectiveAI(cfg, aiStored)
	avail := notify.ChannelAvailability(effective)
	emailProvider := effective.EmailProvider
	if emailProvider == "" {
		emailProvider = notify.EmailProviderResend
	}
	orKey := aiStored.OpenRouterAPIKey
	if strings.TrimSpace(orKey) == "" {
		orKey = cfg.OpenRouterAPIKey
	}
	gemKey := aiStored.GeminiAPIKey
	if strings.TrimSpace(gemKey) == "" {
		gemKey = cfg.GeminiAPIKey
	}
	uiAIProvider := strings.TrimSpace(aiStored.Provider)
	if uiAIProvider == "" {
		uiAIProvider = strings.TrimSpace(cfg.AIProvider)
	}
	out := map[string]any{
		"email": map[string]any{
			"enabled":           effective.EmailEnabled,
			"from":              effective.EmailFrom,
			"provider":          emailProvider,
			"configured":        notify.EmailProviderReady(effective),
			"available":         avail[storage.ChannelEmail],
			"api_key_hint":      notify.MaskSecret(effective.ResendAPIKey),
			"uses_env_fallback": stored.ResendAPIKey == "" && cfg.ResendAPIKey != "",
			"resend_ready":      effective.ResendAPIKey != "",
			"smtp_ready":        effective.SMTPHost != "",
		},
		"smtp": map[string]any{
			"host":              effective.SMTPHost,
			"port":              effective.SMTPPort,
			"username":          effective.SMTPUsername,
			"tls_mode":          notify.NormalizeSMTPTLSMode(effective.SMTPTLSMode),
			"configured":        effective.SMTPHost != "" && effective.EmailFrom != "",
			"password_hint":     notify.MaskSecret(effective.SMTPPassword),
			"uses_env_fallback": stored.SMTPHost == "" && cfg.SMTPHost != "",
		},
		"telegram": map[string]any{
			"enabled":                effective.TelegramEnabled,
			"configured":             effective.TelegramBotToken != "",
			"available":              avail[storage.ChannelTelegram],
			"bot_token_hint":         notify.MaskSecret(effective.TelegramBotToken),
			"uses_env_fallback":      stored.TelegramBotToken == "" && cfg.TelegramBotToken != "",
			"gateway_url":            effective.TelegramGatewayURL,
			"gateway_configured":     effective.TelegramGatewayURL != "" && effective.TelegramGatewayAPIToken != "",
			"gateway_api_token_hint": notify.MaskSecret(effective.TelegramGatewayAPIToken),
			"gateway_inbound_mode":   effective.TelegramGatewayInboundMode,
			"uses_gateway":           effective.TelegramGatewayURL != "" && effective.TelegramGatewayAPIToken != "",
		},
	}
	out["ai"] = map[string]any{
		"provider":                uiAIProvider,
		"active_provider":         aiEffective.Provider,
		"configured":              strings.TrimSpace(aiEffective.APIKey) != "",
		"openrouter_model":        firstNonEmpty(aiStored.OpenRouterModel, cfg.OpenRouterModel, "google/gemini-2.5-flash"),
		"openrouter_api_key_hint": notify.MaskSecret(orKey),
		"openrouter_key_saved":    aiStored.OpenRouterAPIKey != "",
		"openrouter_uses_env":     aiStored.OpenRouterAPIKey == "" && cfg.OpenRouterAPIKey != "",
		"gemini_model":            firstNonEmpty(aiStored.GeminiModel, cfg.GeminiModel, "gemini-2.5-flash"),
		"gemini_api_key_hint":     notify.MaskSecret(gemKey),
		"gemini_key_saved":        aiStored.GeminiAPIKey != "",
		"gemini_uses_env":         aiStored.GeminiAPIKey == "" && cfg.GeminiAPIKey != "",
	}
	if aiStored.UpdatedAt.IsZero() {
		// omit
	} else {
		out["ai_updated_at"] = aiStored.UpdatedAt.UTC().Format(time.RFC3339)
	}
	if !stored.UpdatedAt.IsZero() {
		out["updated_at"] = stored.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// validateDeliveryProviderSelection blocks the recurring misconfiguration of
// selecting a provider whose credentials are absent while the other provider's
// are present (e.g. choosing "Resend API" when only SMTP/Mailtrap is set up).
// Values are fixture-friendly: nothing here reads or embeds real secrets.
func validateDeliveryProviderSelection(cur storage.DeliverySettings, cfg config.Config, body adminDeliveryUpdateRequest) error {
	provider := strings.ToLower(strings.TrimSpace(cur.EmailProvider))
	if body.EmailProvider != nil {
		provider = strings.ToLower(strings.TrimSpace(*body.EmailProvider))
	}
	resendValue := strings.TrimSpace(cur.ResendAPIKey)
	if body.ResendAPIKey != nil && strings.TrimSpace(*body.ResendAPIKey) != "" {
		resendValue = strings.TrimSpace(*body.ResendAPIKey)
	}
	if resendValue == "" {
		resendValue = strings.TrimSpace(cfg.ResendAPIKey)
	}
	smtpHostValue := strings.TrimSpace(cur.SMTPHost)
	if body.SMTPHost != nil {
		smtpHostValue = strings.TrimSpace(*body.SMTPHost)
	}
	if smtpHostValue == "" {
		smtpHostValue = strings.TrimSpace(cfg.SMTPHost)
	}
	switch provider {
	case "resend":
		if resendValue == "" && smtpHostValue != "" {
			return fmt.Errorf("provider \"Resend API\" selected but no Resend API key is set, while SMTP is configured — select \"SMTP server\" if you are sending via SMTP/Mailtrap")
		}
	case "smtp":
		if smtpHostValue == "" && resendValue != "" {
			return fmt.Errorf("provider \"SMTP server\" selected but no SMTP host is set, while a Resend API key is configured — select \"Resend API\" if you are sending via Resend")
		}
	}
	return nil
}
