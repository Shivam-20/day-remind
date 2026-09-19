package web

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shivam-20/day-remind/internal/notify"
	"github.com/shivam-20/day-remind/internal/secret"
	"github.com/shivam-20/day-remind/internal/storage"
)

// adminSettingsPayload reports the instance configuration with credentials
// redacted. "unreadable" tells an admin that something is stored but can no
// longer be decrypted, which means SESSION_KEY changed — otherwise a rotated
// key looks identical to a blank field.
func (s *Server) adminSettingsPayload(inst storage.InstanceSettings) map[string]any {
	tg := map[string]any{
		"enabled":   inst.Telegram.Enabled,
		"token_set": inst.Telegram.BotTokenEnc != "",
	}
	if inst.Telegram.BotTokenEnc != "" {
		if tok, err := s.box.Decrypt(inst.Telegram.BotTokenEnc); err != nil {
			tg["unreadable"] = true
		} else {
			tg["token_hint"] = secret.Hint(tok)
		}
	}

	smtp := smtpPayload(inst.SMTP)
	if inst.SMTP.PasswordEnc != "" {
		if _, err := s.box.Decrypt(inst.SMTP.PasswordEnc); err != nil {
			smtp["unreadable"] = true
		}
	}

	out := map[string]any{
		"smtp":      smtp,
		"telegram":  tg,
		"templates": templatePayload(inst.Templates),
		"defaults":  templatePayload(notify.DefaultTemplates()),
		"variables": notify.TemplateVariables,
	}
	if !inst.UpdatedAt.IsZero() {
		out["updated_at"] = inst.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return out
}

func (s *Server) handleAdminGetSettings(w http.ResponseWriter, r *http.Request) {
	inst, err := s.db.GetInstanceSettings(r.Context())
	if err != nil {
		writeStorageError(w, err, "could not load settings")
		return
	}
	writeJSON(w, http.StatusOK, s.adminSettingsPayload(inst))
}

func (s *Server) handleAdminUpdateSettings(w http.ResponseWriter, r *http.Request) {
	actor := adminFromContext(r.Context())
	inst, err := s.db.GetInstanceSettings(r.Context())
	if err != nil {
		writeStorageError(w, err, "could not load settings")
		return
	}

	var body struct {
		SMTP     *smtpInput `json:"smtp"`
		Telegram *struct {
			Enabled  *bool   `json:"enabled"`
			BotToken *string `json:"bot_token"`
		} `json:"telegram"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	if body.SMTP != nil {
		next, err := body.SMTP.apply(inst.SMTP, s.box.Encrypt)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not store the credential")
			return
		}
		inst.SMTP = next
	}
	if body.Telegram != nil {
		if body.Telegram.Enabled != nil {
			inst.Telegram.Enabled = *body.Telegram.Enabled
		}
		if body.Telegram.BotToken != nil {
			if *body.Telegram.BotToken == "" {
				inst.Telegram.BotTokenEnc = ""
			} else {
				e, err := s.box.Encrypt(strings.TrimSpace(*body.Telegram.BotToken))
				if err != nil {
					writeError(w, http.StatusInternalServerError, "could not store the token")
					return
				}
				inst.Telegram.BotTokenEnc = e
			}
		}
	}

	if err := s.db.SaveInstanceSettings(r.Context(), inst, actor.ID); err != nil {
		writeStorageError(w, err, "could not save settings")
		return
	}
	writeJSON(w, http.StatusOK, s.adminSettingsPayload(inst))
}

// handleAdminTestSettings sends a sample message using the *instance*
// configuration to the admin's own address and chat, bypassing their personal
// overrides so the thing being tested is the thing being configured.
func (s *Server) handleAdminTestSettings(w http.ResponseWriter, r *http.Request) {
	actor := adminFromContext(r.Context())
	inst, err := s.db.GetInstanceSettings(r.Context())
	if err != nil {
		writeStorageError(w, err, "could not load settings")
		return
	}

	msg := notify.SampleMessage(false)
	msg.Title = "Instance delivery test"
	msg.Description = "Sent from the admin panel using the instance configuration."
	msg.Timezone = actor.Timezone
	msg.DisplayName = actor.DisplayName

	templates := notify.Merge(notify.DefaultTemplates(), inst.Templates)
	rendered, _ := notify.Render(templates, msg)

	results := map[string]any{}

	if inst.SMTP.Complete() {
		password, err := s.box.Decrypt(inst.SMTP.PasswordEnc)
		if err != nil {
			results[notify.ChannelEmail] = map[string]any{
				"ok": false, "error": "the stored password can no longer be read — re-enter it",
			}
		} else {
			dest := actor.Notify.EmailAddress
			if dest == "" {
				dest = actor.Email
			}
			results[notify.ChannelEmail] = s.runTest(r.Context(), actor,
				smtpSenderFor(inst.SMTP, password), dest, msg, rendered, notify.ChannelEmail)
		}
	}

	if inst.Telegram.Complete() && actor.TelegramChatID != nil {
		token, err := s.box.Decrypt(inst.Telegram.BotTokenEnc)
		if err != nil {
			results[notify.ChannelTelegram] = map[string]any{
				"ok": false, "error": "the stored bot token can no longer be read — re-enter it",
			}
		} else {
			results[notify.ChannelTelegram] = s.runTest(r.Context(), actor,
				notify.NewTelegramClient(token, s.cfg.TelegramAPIBase),
				strconv.FormatInt(*actor.TelegramChatID, 10), msg, rendered, notify.ChannelTelegram)
		}
	}

	if len(results) == 0 {
		writeError(w, http.StatusConflict,
			"nothing to test: configure SMTP, or link your own Telegram chat to test the bot")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (s *Server) runTest(ctx context.Context, actor storage.User, sender notify.Sender,
	dest string, msg notify.Message, rendered notify.Rendered, channel string) map[string]any {

	sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	err := sender.Send(sendCtx, dest, msg, rendered)
	entry := storage.Delivery{
		UserID: actor.ID, Channel: channel,
		Kind: "admin-test", Status: storage.DeliverySent,
	}
	out := map[string]any{"ok": err == nil, "to": dest}
	if err != nil {
		entry.Status = storage.DeliveryFailed
		entry.Detail = err.Error()
		out["error"] = err.Error()
	}
	_ = s.db.LogDelivery(ctx, entry)
	return out
}

// ----------------------------------------------------------- admin templates

func (s *Server) handleAdminUpdateTemplates(w http.ResponseWriter, r *http.Request) {
	actor := adminFromContext(r.Context())
	ts, ok := decodeTemplateSet(w, r)
	if !ok {
		return
	}
	inst, err := s.db.GetInstanceSettings(r.Context())
	if err != nil {
		writeStorageError(w, err, "could not load settings")
		return
	}
	inst.Templates = ts
	if err := s.db.SaveInstanceSettings(r.Context(), inst, actor.ID); err != nil {
		writeStorageError(w, err, "could not save templates")
		return
	}
	writeJSON(w, http.StatusOK, s.adminSettingsPayload(inst))
}

// handleAdminResetTemplates clears the overrides so the built-in defaults apply
// again. It stores empty fields rather than copying the defaults in, so future
// improvements to the defaults are picked up.
func (s *Server) handleAdminResetTemplates(w http.ResponseWriter, r *http.Request) {
	actor := adminFromContext(r.Context())
	inst, err := s.db.GetInstanceSettings(r.Context())
	if err != nil {
		writeStorageError(w, err, "could not load settings")
		return
	}
	inst.Templates = notify.TemplateSet{}
	if err := s.db.SaveInstanceSettings(r.Context(), inst, actor.ID); err != nil {
		writeStorageError(w, err, "could not reset templates")
		return
	}
	writeJSON(w, http.StatusOK, s.adminSettingsPayload(inst))
}

// handleAdminDeliveryLog exposes recent attempts so an admin can see failures
// without shell access to the database.
func (s *Server) handleAdminDeliveryLog(w http.ResponseWriter, r *http.Request) {
	entries, err := s.db.RecentDeliveries(r.Context(), 50)
	if err != nil {
		writeStorageError(w, err, "could not load the delivery log")
		return
	}
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		row := map[string]any{
			"channel":    e.Channel,
			"kind":       e.Kind,
			"status":     e.Status,
			"created_at": e.CreatedAt.UTC().Format(time.RFC3339),
		}
		if e.Detail != "" {
			row["detail"] = e.Detail
		}
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"deliveries": out})
}

func smtpSenderFor(cfg storage.SMTPSettings, password string) *notify.SMTPSender {
	cfg = cfg.Normalize()
	return &notify.SMTPSender{
		Host: cfg.Host, Port: cfg.Port, Username: cfg.Username,
		Password: password, From: cfg.From, Mode: cfg.TLS,
	}
}
