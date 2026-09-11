package web

import (
	"net/http"

	"github.com/shivam-20/day-remind/internal/notify"
	"github.com/shivam-20/day-remind/internal/storage"
)

type channelPrefRequest struct {
	Enabled *bool   `json:"enabled"`
	Address *string `json:"address"`
}

type updateChannelsRequest struct {
	Email    *channelPrefRequest `json:"email"`
	Telegram *channelPrefRequest `json:"telegram"`
}

func (s *Server) effectiveDelivery(r *http.Request) (notify.DeliveryConfig, error) {
	stored, err := s.db.GetDeliverySettings(r.Context())
	if err != nil {
		return notify.DeliveryConfig{}, err
	}
	return notify.EffectiveDelivery(s.cfg, stored), nil
}

func (s *Server) handleGetNotificationChannels(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	prefs, err := s.db.GetUserNotificationPrefs(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load notification channels")
		return
	}
	delivery, err := s.effectiveDelivery(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load delivery settings")
		return
	}
	writeJSON(w, http.StatusOK, notificationChannelsPayload(prefs, notify.ChannelAvailability(delivery)))
}

func (s *Server) handleUpdateNotificationChannels(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	var body updateChannelsRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	current, err := s.db.GetUserNotificationPrefs(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load notification channels")
		return
	}
	updated := applyChannelUpdates(current, body)
	prefs, err := s.db.UpdateUserNotificationPrefs(r.Context(), userID, updated)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	delivery, err := s.effectiveDelivery(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load delivery settings")
		return
	}
	writeJSON(w, http.StatusOK, notificationChannelsPayload(prefs, notify.ChannelAvailability(delivery)))
}

func applyChannelUpdates(current []storage.NotificationPref, body updateChannelsRequest) []storage.NotificationPref {
	byChannel := map[string]storage.NotificationPref{}
	for _, p := range current {
		byChannel[p.Channel] = p
	}
	if body.Email != nil {
		p := byChannel[storage.ChannelEmail]
		if body.Email.Enabled != nil {
			p.Enabled = *body.Email.Enabled
		}
		if body.Email.Address != nil {
			p.Address = *body.Email.Address
		}
		byChannel[storage.ChannelEmail] = p
	}
	if body.Telegram != nil {
		p := byChannel[storage.ChannelTelegram]
		if body.Telegram.Enabled != nil {
			p.Enabled = *body.Telegram.Enabled
		}
		if body.Telegram.Address != nil {
			p.Address = *body.Telegram.Address
		}
		byChannel[storage.ChannelTelegram] = p
	}
	return []storage.NotificationPref{
		byChannel[storage.ChannelEmail],
		byChannel[storage.ChannelTelegram],
	}
}

func notificationChannelsPayload(prefs []storage.NotificationPref, available map[string]bool) map[string]any {
	out := map[string]any{}
	for _, p := range prefs {
		out[p.Channel] = map[string]any{
			"enabled":   p.Enabled,
			"address":   p.Address,
			"available": available[p.Channel],
		}
	}
	if _, ok := out[storage.ChannelEmail]; !ok {
		out[storage.ChannelEmail] = map[string]any{"enabled": false, "address": "", "available": available[storage.ChannelEmail]}
	}
	if _, ok := out[storage.ChannelTelegram]; !ok {
		out[storage.ChannelTelegram] = map[string]any{"enabled": false, "address": "", "available": available[storage.ChannelTelegram]}
	}
	return map[string]any{"channels": out}
}
