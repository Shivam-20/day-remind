package web

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/shivam-20/day-remind/internal/delivery"
	"github.com/shivam-20/day-remind/internal/notify"
	"github.com/shivam-20/day-remind/internal/storage"
)

// notifyPayload describes the user's delivery setup plus what the server can
// actually deliver, so the UI can explain why a channel is unavailable rather
// than offering a toggle that silently does nothing.
//
// Credentials are never included: only whether one is stored, and a hint.
func (s *Server) notifyPayload(u storage.User, snap *delivery.Snapshot) map[string]any {
	inst := snap.Settings()
	out := map[string]any{
		"email_enabled":    u.Notify.EmailEnabled,
		"email_address":    u.Notify.EmailAddress,
		"telegram_enabled": u.Notify.TelegramEnabled,
		"telegram_linked":  u.TelegramChatID != nil,
		"advance_minutes":  u.Notify.AdvanceMinutes,
		"channels": map[string]any{
			"email_configured":    snap.EmailAvailable(u),
			"telegram_configured": snap.TelegramAvailable(u) || inst.Telegram.Complete(),
			"instance_email":      inst.SMTP.Complete(),
			"instance_telegram":   inst.Telegram.Complete(),
		},
		"own_smtp":     smtpPayload(u.Delivery.SMTP),
		"own_telegram": ownTelegramPayload(u.Delivery.Telegram),
	}
	if u.TelegramChatID != nil {
		out["telegram_chat_id"] = *u.TelegramChatID
	}
	return out
}

// smtpPayload redacts the password: callers learn that one is stored, never
// what it is.
func smtpPayload(c storage.SMTPSettings) map[string]any {
	c = c.Normalize()
	return map[string]any{
		"enabled":          c.Enabled,
		"host":             c.Host,
		"port":             c.Port,
		"username":         c.Username,
		"from":             c.From,
		"tls":              c.TLS,
		"password_set":     c.PasswordEnc != "",
		"password_present": c.PasswordEnc != "",
	}
}

func ownTelegramPayload(t storage.UserTelegram) map[string]any {
	out := map[string]any{
		"enabled":   t.Enabled,
		"token_set": t.BotTokenEnc != "",
		"chat_set":  t.OwnChatID != nil,
	}
	if t.OwnChatID != nil {
		out["chat_id"] = *t.OwnChatID
	}
	return out
}

func (s *Server) handleGetNotifySettings(w http.ResponseWriter, r *http.Request) {
	user, err := s.db.GetUserByID(r.Context(), userIDFromContext(r.Context()))
	if err != nil {
		writeStorageError(w, err, "could not load settings")
		return
	}
	snap, err := s.resolver.Snapshot(r.Context())
	if err != nil {
		writeStorageError(w, err, "could not load delivery settings")
		return
	}
	writeJSON(w, http.StatusOK, s.notifyPayload(user, snap))
}

func (s *Server) handleUpdateNotifySettings(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	current, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil {
		writeStorageError(w, err, "could not load settings")
		return
	}
	snap, err := s.resolver.Snapshot(r.Context())
	if err != nil {
		writeStorageError(w, err, "could not load delivery settings")
		return
	}

	// Pointers so an omitted field keeps its stored value.
	var body struct {
		EmailEnabled    *bool   `json:"email_enabled"`
		EmailAddress    *string `json:"email_address"`
		TelegramEnabled *bool   `json:"telegram_enabled"`
		AdvanceMinutes  *int    `json:"advance_minutes"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	prefs := current.Notify
	if body.EmailEnabled != nil {
		prefs.EmailEnabled = *body.EmailEnabled
	}
	if body.EmailAddress != nil {
		prefs.EmailAddress = *body.EmailAddress
	}
	if body.TelegramEnabled != nil {
		prefs.TelegramEnabled = *body.TelegramEnabled
	}
	if body.AdvanceMinutes != nil {
		prefs.AdvanceMinutes = *body.AdvanceMinutes
	}

	// Refuse a toggle nothing can honour, so the UI never shows a channel as
	// on while neither the instance nor the user can actually deliver it.
	if prefs.EmailEnabled && !snap.EmailAvailable(current) {
		writeError(w, http.StatusConflict, "no mail server is configured — ask an admin, or add your own below")
		return
	}
	if prefs.TelegramEnabled && !snap.TelegramAvailable(current) {
		writeError(w, http.StatusConflict, "link a Telegram chat first, or add your own bot below")
		return
	}

	updated, err := s.db.SetNotifyPrefs(r.Context(), userID, prefs)
	if err != nil {
		writeStorageError(w, err, "could not save settings")
		return
	}
	writeJSON(w, http.StatusOK, s.notifyPayload(updated, snap))
}

// handleTelegramLinkCode issues the code the user sends to the bot as /start.
func (s *Server) handleTelegramLinkCode(w http.ResponseWriter, r *http.Request) {
	snap, err := s.resolver.Snapshot(r.Context())
	if err != nil {
		writeStorageError(w, err, "could not load delivery settings")
		return
	}
	if !snap.Settings().Telegram.Complete() {
		writeError(w, http.StatusConflict, "the instance Telegram bot is not configured")
		return
	}
	code, expires, err := s.db.CreateTelegramLinkCode(r.Context(), userIDFromContext(r.Context()))
	if err != nil {
		writeStorageError(w, err, "could not create link code")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code":       code,
		"expires_at": expires.UTC().Format(time.RFC3339),
		"command":    "/start " + code,
	})
}

func (s *Server) handleTelegramUnlink(w http.ResponseWriter, r *http.Request) {
	updated, err := s.db.UnlinkTelegram(r.Context(), userIDFromContext(r.Context()))
	if err != nil {
		writeStorageError(w, err, "could not unlink")
		return
	}
	snap, err := s.resolver.Snapshot(r.Context())
	if err != nil {
		writeStorageError(w, err, "could not load delivery settings")
		return
	}
	writeJSON(w, http.StatusOK, s.notifyPayload(updated, snap))
}

// handleSendTestNotification delivers a sample message over the user's own
// enabled channels, so a misconfiguration surfaces here rather than at 3am
// when a real reminder silently fails. It resolves through the same path the
// scheduler uses, including the user's templates.
func (s *Server) handleSendTestNotification(w http.ResponseWriter, r *http.Request) {
	user, err := s.db.GetUserByID(r.Context(), userIDFromContext(r.Context()))
	if err != nil {
		writeStorageError(w, err, "could not load settings")
		return
	}
	snap, err := s.resolver.Snapshot(r.Context())
	if err != nil {
		writeStorageError(w, err, "could not load delivery settings")
		return
	}

	msg := notify.SampleMessage(false)
	msg.Title = "Test notification"
	msg.Description = "If you are reading this, Day Remind can reach you on this channel."
	msg.Timezone = user.Timezone
	msg.DisplayName = user.DisplayName

	targets := snap.For(user)
	if len(targets) == 0 {
		writeError(w, http.StatusConflict, "no delivery channel is enabled")
		return
	}

	results := map[string]any{}
	for _, t := range targets {
		rendered, problems := notify.Render(t.Templates, msg)
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		err := t.Sender.Send(ctx, t.Dest, msg, rendered)
		cancel()

		entry := storage.Delivery{
			UserID: user.ID, Channel: t.Channel,
			Kind: "test", Status: storage.DeliverySent,
		}
		res := map[string]any{"ok": err == nil}
		if err != nil {
			entry.Status = storage.DeliveryFailed
			entry.Detail = err.Error()
			res["error"] = err.Error()
		}
		// Surface template problems even on a successful send: the message
		// arrived, but not with the wording the author intended.
		if len(problems) > 0 {
			notes := make([]string, 0, len(problems))
			for _, p := range problems {
				notes = append(notes, p.Error())
			}
			res["template_warnings"] = notes
		}
		results[t.Channel] = res

		if logErr := s.db.LogDelivery(r.Context(), entry); logErr != nil {
			log.Printf("test notification: could not record delivery: %v", logErr)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}
