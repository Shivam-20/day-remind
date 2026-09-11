package web

import (
	"net/http"
	"time"

	"github.com/shivam-20/day-remind/internal/config"
	"github.com/shivam-20/day-remind/internal/notify"
	"github.com/shivam-20/day-remind/internal/storage"
)

type adminDeliveryUpdateRequest struct {
	EmailEnabled     *bool   `json:"email_enabled"`
	EmailFrom        *string `json:"email_from"`
	ResendAPIKey     *string `json:"resend_api_key"`
	TelegramEnabled  *bool   `json:"telegram_enabled"`
	TelegramBotToken *string `json:"telegram_bot_token"`
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
	effective := notify.EffectiveDelivery(s.cfg, stored)
	writeJSON(w, http.StatusOK, adminDeliveryPayload(stored, effective, s.cfg))
}

func (s *Server) handleAdminUpdateDeliverySettings(w http.ResponseWriter, r *http.Request) {
	var body adminDeliveryUpdateRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	stored, err := s.db.UpdateDeliverySettings(r.Context(), storage.DeliverySettingsUpdate{
		EmailEnabled:     body.EmailEnabled,
		EmailFrom:        body.EmailFrom,
		ResendAPIKey:     body.ResendAPIKey,
		TelegramEnabled:  body.TelegramEnabled,
		TelegramBotToken: body.TelegramBotToken,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	effective := notify.EffectiveDelivery(s.cfg, stored)
	writeJSON(w, http.StatusOK, adminDeliveryPayload(stored, effective, s.cfg))
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

func adminDeliveryPayload(stored storage.DeliverySettings, effective notify.DeliveryConfig, cfg config.Config) map[string]any {
	avail := notify.ChannelAvailability(effective)
	out := map[string]any{
		"email": map[string]any{
			"enabled":           effective.EmailEnabled,
			"from":              effective.EmailFrom,
			"configured":        effective.ResendAPIKey != "" && effective.EmailFrom != "",
			"available":         avail[storage.ChannelEmail],
			"api_key_hint":      notify.MaskSecret(effective.ResendAPIKey),
			"uses_env_fallback": stored.ResendAPIKey == "" && cfg.ResendAPIKey != "",
		},
		"telegram": map[string]any{
			"enabled":           effective.TelegramEnabled,
			"configured":        effective.TelegramBotToken != "",
			"available":         avail[storage.ChannelTelegram],
			"bot_token_hint":    notify.MaskSecret(effective.TelegramBotToken),
			"uses_env_fallback": stored.TelegramBotToken == "" && cfg.TelegramBotToken != "",
		},
	}
	if !stored.UpdatedAt.IsZero() {
		out["updated_at"] = stored.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return out
}
