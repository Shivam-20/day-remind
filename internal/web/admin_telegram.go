package web

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/notify"
	"github.com/shivam-20/day-remind/internal/storage"
)

func (s *Server) handleAdminListTelegramUsers(w http.ResponseWriter, r *http.Request) {
	list, err := s.db.ListTelegramLinkedUsers(r.Context(),
		r.URL.Query().Get("q"),
		queryInt(r, "limit", 100),
		queryInt(r, "offset", 0),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list telegram users")
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, info := range list {
		item := map[string]any{
			"user_id":      info.UserID.String(),
			"email":        info.Email,
			"display_name": info.DisplayName,
			"chat_id":      info.ChatID,
			"enabled":      info.Enabled,
			"username":     info.Username,
			"first_name":   info.FirstName,
		}
		if info.LinkedAt != nil {
			item["linked_at"] = info.LinkedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": out})
}

func (s *Server) handleAdminUnlinkTelegram(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.db.UnlinkTelegram(r.Context(), id); err != nil {
		if writeStorageErr(w, err, "not found") {
			return
		}
		writeError(w, http.StatusInternalServerError, "could not unlink telegram")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "unlinked"})
}

func (s *Server) handleAdminMessageTelegram(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	text := strings.TrimSpace(body.Text)
	if text == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	prefs, err := s.db.GetUserNotificationPrefs(r.Context(), id)
	if err != nil {
		if writeStorageErr(w, err, "not found") {
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load user")
		return
	}
	chatID := ""
	for _, p := range prefs {
		if p.Channel == storage.ChannelTelegram && strings.TrimSpace(p.Address) != "" {
			chatID = p.Address
			break
		}
	}
	if chatID == "" {
		writeError(w, http.StatusBadRequest, "user is not linked to telegram")
		return
	}
	delivery, err := s.deliveryConfig(r.Context())
	if err != nil || (!notify.IsGateway(delivery) && delivery.TelegramBotToken == "") {
		writeError(w, http.StatusServiceUnavailable, "telegram is not configured")
		return
	}
	if err := notify.SendTelegramMessage(r.Context(), delivery, chatID, notify.EscapeTelegramHTML(text), nil); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func (s *Server) handleAdminTelegramStatus(w http.ResponseWriter, r *http.Request) {
	delivery, _ := s.deliveryConfig(r.Context())
	usesGateway := delivery.TelegramGatewayURL != "" && delivery.TelegramGatewayAPIToken != ""
	out := map[string]any{
		"mode":         s.cfg.TelegramUpdateMode,
		"configured":   delivery.TelegramBotToken != "",
		"secret_set":   s.cfg.TelegramWebhookSecret != "",
		"webhook_url":  s.telegramWebhookURL(),
		"uses_gateway": usesGateway,
	}
	if usesGateway {
		out["gateway_url"] = delivery.TelegramGatewayURL
		out["gateway_inbound_mode"] = delivery.TelegramGatewayInboundMode
		// Query the gateway's /v1/status for the app's transport state.
		if raw, err := notify.CallGatewayStatus(r.Context(), delivery); err == nil {
			out["gateway_status"] = raw
		} else {
			out["gateway_status_error"] = err.Error()
		}
		return
	}
	if delivery.TelegramBotToken != "" {
		if info, err := notify.GetTelegramWebhookInfo(r.Context(), delivery); err == nil {
			out["webhook"] = info
		} else {
			out["webhook_error"] = err.Error()
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleAdminRegisterTelegramWebhook(w http.ResponseWriter, r *http.Request) {
	delivery, err := s.deliveryConfig(r.Context())
	if err != nil || (!notify.IsGateway(delivery) && delivery.TelegramBotToken == "") {
		writeError(w, http.StatusServiceUnavailable, "telegram is not configured")
		return
	}
	if notify.IsGateway(delivery) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "gateway-managed",
			"message": "Webhook transport is managed by telegram-gateway. Use its admin UI sync-transport endpoint.",
		})
		return
	}
	url := s.telegramWebhookURL()
	if !strings.HasPrefix(url, "https://") {
		writeError(w, http.StatusBadRequest, "telegram webhooks require an https public URL")
		return
	}
	if err := notify.SetTelegramWebhook(r.Context(), delivery, url, s.cfg.TelegramWebhookSecret); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "registered", "url": url})
}

func (s *Server) handleAdminDeleteTelegramWebhook(w http.ResponseWriter, r *http.Request) {
	delivery, err := s.deliveryConfig(r.Context())
	if err != nil || (!notify.IsGateway(delivery) && delivery.TelegramBotToken == "") {
		writeError(w, http.StatusServiceUnavailable, "telegram is not configured")
		return
	}
	if notify.IsGateway(delivery) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "gateway-managed",
			"message": "Webhook transport is managed by telegram-gateway. Use its admin UI sync-transport endpoint.",
		})
		return
	}
	if err := notify.DeleteTelegramWebhook(r.Context(), delivery); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) telegramWebhookURL() string {
	base := strings.TrimRight(strings.TrimSpace(s.cfg.WebhookBaseURL), "/")
	if base == "" {
		return ""
	}
	return base + "/api/webhooks/telegram"
}
