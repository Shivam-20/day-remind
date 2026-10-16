package web

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/shivam-20/day-remind/internal/notify"
	"github.com/shivam-20/day-remind/internal/storage"
)

// smtpInput is the wire shape for a mail server, shared by the admin and user
// endpoints. Password is a pointer so three cases stay distinct: omitted keeps
// the stored one, "" clears it, and a value replaces it. A stored password is
// never sent back, so a form that round-trips cannot silently erase it.
type smtpInput struct {
	Enabled  *bool   `json:"enabled"`
	Host     *string `json:"host"`
	Port     *int    `json:"port"`
	Username *string `json:"username"`
	Password *string `json:"password"`
	From     *string `json:"from"`
	TLS      *string `json:"tls"`
}

// apply merges the request onto the stored settings and encrypts a new
// password if one was supplied.
func (in smtpInput) apply(cur storage.SMTPSettings, enc func(string) (string, error)) (storage.SMTPSettings, error) {
	if in.Enabled != nil {
		cur.Enabled = *in.Enabled
	}
	if in.Host != nil {
		cur.Host = *in.Host
	}
	if in.Port != nil {
		cur.Port = *in.Port
	}
	if in.Username != nil {
		cur.Username = *in.Username
	}
	if in.From != nil {
		cur.From = *in.From
	}
	if in.TLS != nil {
		cur.TLS = *in.TLS
	}
	if in.Password != nil {
		if *in.Password == "" {
			cur.PasswordEnc = ""
		} else {
			e, err := enc(*in.Password)
			if err != nil {
				return cur, err
			}
			cur.PasswordEnc = e
		}
	}
	return cur.Normalize(), nil
}

// allowedSMTPPorts limits where the server will open an outbound connection.
// A user-supplied host and port otherwise lets anyone point this process at an
// arbitrary internal service and observe how it responds.
var allowedSMTPPorts = map[int]bool{25: true, 465: true, 587: true, 2525: true, 1025: true}

// checkSMTPTarget guards a user-supplied mail server. Instance settings are set
// by an admin who already controls the process, so this applies to per-user
// configuration only.
func checkSMTPTarget(cfg storage.SMTPSettings, allowPrivate bool) error {
	if !cfg.Enabled {
		return nil
	}
	if !allowedSMTPPorts[cfg.Port] {
		return fmt.Errorf("port %d is not allowed; use 25, 465, 587 or 2525", cfg.Port)
	}
	if allowPrivate {
		return nil
	}
	// Resolve and reject anything pointing back inside the network.
	ips, err := net.LookupIP(cfg.Host)
	if err != nil {
		return fmt.Errorf("could not resolve %q", cfg.Host)
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return fmt.Errorf("%q resolves to a private address, which is not allowed", cfg.Host)
		}
	}
	return nil
}

// ---------------------------------------------------------------- user routes

func (s *Server) handleUpdateOwnSMTP(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil {
		writeStorageError(w, err, "could not load settings")
		return
	}
	var body smtpInput
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	next, err := body.apply(user.Delivery.SMTP, s.box.Encrypt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not store the credential")
		return
	}
	if err := checkSMTPTarget(next, s.cfg.AllowPrivateSMTP); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	updated, err := s.db.SetUserDelivery(r.Context(), userID,
		storage.UserDelivery{SMTP: next, Telegram: user.Delivery.Telegram})
	if err != nil {
		writeStorageError(w, err, "could not save your mail server")
		return
	}
	s.writeNotifySettings(w, r, updated)
}

func (s *Server) handleUpdateOwnTelegram(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil {
		writeStorageError(w, err, "could not load settings")
		return
	}
	var body struct {
		Enabled  *bool   `json:"enabled"`
		BotToken *string `json:"bot_token"`
		ChatID   *int64  `json:"chat_id"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	tg := user.Delivery.Telegram
	if body.Enabled != nil {
		tg.Enabled = *body.Enabled
	}
	if body.BotToken != nil {
		if *body.BotToken == "" {
			tg.BotTokenEnc = ""
			tg.OwnChatID = nil
		} else {
			e, err := s.box.Encrypt(strings.TrimSpace(*body.BotToken))
			if err != nil {
				writeError(w, http.StatusInternalServerError, "could not store the token")
				return
			}
			// A new token means a different bot, so any discovered chat is stale.
			tg.BotTokenEnc = e
			tg.OwnChatID = nil
		}
	}
	if body.ChatID != nil {
		id := *body.ChatID
		tg.OwnChatID = &id
	}

	updated, err := s.db.SetUserDelivery(r.Context(), userID,
		storage.UserDelivery{SMTP: user.Delivery.SMTP, Telegram: tg})
	if err != nil {
		writeStorageError(w, err, "could not save your bot")
		return
	}
	s.writeNotifySettings(w, r, updated)
}

// handleDiscoverOwnTelegramChat finds the chat that has messaged the user's own
// bot. A chat id is per-bot, so the one linked through the instance bot is not
// valid for theirs; they have to start their bot, and this reads that update.
//
// This is a single getUpdates call, not a poller: no long-running consumer is
// attached to the user's token.
func (s *Server) handleDiscoverOwnTelegramChat(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil {
		writeStorageError(w, err, "could not load settings")
		return
	}
	if user.Delivery.Telegram.BotTokenEnc == "" {
		writeError(w, http.StatusConflict, "save your bot token first")
		return
	}
	token, err := s.box.Decrypt(user.Delivery.Telegram.BotTokenEnc)
	if err != nil {
		writeError(w, http.StatusConflict, "your stored bot token can no longer be read — re-enter it")
		return
	}

	client := notify.NewTelegramClient(token, s.cfg.TelegramAPIBase)
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	if _, err := client.GetMe(ctx); err != nil {
		writeError(w, http.StatusBadRequest, "Telegram rejected that token: "+err.Error())
		return
	}
	// timeout 0: return immediately with whatever is queued.
	updates, err := client.GetUpdates(ctx, 0, 0)
	if err != nil {
		writeError(w, http.StatusBadGateway, "could not read updates: "+err.Error())
		return
	}
	var chatID int64
	for _, u := range updates {
		if u.Message != nil && u.Message.Chat.ID != 0 {
			chatID = u.Message.Chat.ID
		}
	}
	if chatID == 0 {
		writeError(w, http.StatusNotFound, "no message found — send /start to your bot, then try again")
		return
	}

	updated, err := s.db.SetUserTelegramOwnChat(r.Context(), userID, chatID)
	if err != nil {
		writeStorageError(w, err, "could not save the chat")
		return
	}
	s.writeNotifySettings(w, r, updated)
}

// writeNotifySettings returns the full settings payload after a change, so the
// UI always re-renders from server truth rather than guessing.
func (s *Server) writeNotifySettings(w http.ResponseWriter, r *http.Request, u storage.User) {
	snap, err := s.resolver.Snapshot(r.Context())
	if err != nil {
		writeStorageError(w, err, "could not load delivery settings")
		return
	}
	writeJSON(w, http.StatusOK, s.notifyPayload(u, snap))
}

// ------------------------------------------------------------ user templates

func (s *Server) handleGetOwnTemplates(w http.ResponseWriter, r *http.Request) {
	user, err := s.db.GetUserByID(r.Context(), userIDFromContext(r.Context()))
	if err != nil {
		writeStorageError(w, err, "could not load templates")
		return
	}
	snap, err := s.resolver.Snapshot(r.Context())
	if err != nil {
		writeStorageError(w, err, "could not load templates")
		return
	}
	// "inherited" is what they get with no override, so the editor can show
	// what a blank field will actually produce.
	writeJSON(w, http.StatusOK, map[string]any{
		"own":       templatePayload(user.Templates),
		"inherited": templatePayload(notify.Merge(notify.DefaultTemplates(), snap.Settings().Templates)),
		"variables": notify.TemplateVariables,
	})
}

func (s *Server) handleUpdateOwnTemplates(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	ts, ok := decodeTemplateSet(w, r)
	if !ok {
		return
	}
	if _, err := s.db.SetUserTemplates(r.Context(), userID, ts); err != nil {
		writeStorageError(w, err, "could not save templates")
		return
	}
	s.handleGetOwnTemplates(w, r)
}

func templatePayload(ts notify.TemplateSet) map[string]any {
	return map[string]any{
		"email_subject":       ts.EmailSubject,
		"email_html":          ts.EmailHTML,
		"email_css":           ts.EmailCSS,
		"email_text":          ts.EmailText,
		"telegram_text":       ts.TelegramText,
		"telegram_parse_mode": ts.TelegramParseMode,
	}
}

// decodeTemplateSet reads and validates a template set, reporting the offending
// field so the editor can point at it.
func decodeTemplateSet(w http.ResponseWriter, r *http.Request) (notify.TemplateSet, bool) {
	var body struct {
		EmailSubject      string `json:"email_subject"`
		EmailHTML         string `json:"email_html"`
		EmailCSS          string `json:"email_css"`
		EmailText         string `json:"email_text"`
		TelegramText      string `json:"telegram_text"`
		TelegramParseMode string `json:"telegram_parse_mode"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return notify.TemplateSet{}, false
	}
	ts := notify.TemplateSet{
		EmailSubject:      body.EmailSubject,
		EmailHTML:         body.EmailHTML,
		EmailCSS:          body.EmailCSS,
		EmailText:         body.EmailText,
		TelegramText:      body.TelegramText,
		TelegramParseMode: body.TelegramParseMode,
	}
	if errs := notify.Validate(ts); len(errs) > 0 {
		msgs := make([]string, 0, len(errs))
		for _, e := range errs {
			msgs = append(msgs, e.Error())
		}
		writeError(w, http.StatusBadRequest, strings.Join(msgs, "; "))
		return notify.TemplateSet{}, false
	}
	return ts, true
}

// handleTemplatePreview renders a template set against sample content without
// saving or sending anything.
func (s *Server) handleTemplatePreview(w http.ResponseWriter, r *http.Request) {
	ts, ok := decodeTemplateSet(w, r)
	if !ok {
		return
	}
	out := map[string]any{}
	for _, advance := range []bool{false, true} {
		rendered, problems := notify.Render(ts, notify.SampleMessage(advance))
		key := "due"
		if advance {
			key = "advance"
		}
		warnings := make([]string, 0, len(problems))
		for _, p := range problems {
			warnings = append(warnings, p.Error())
		}
		out[key] = map[string]any{
			"subject":  rendered.Subject,
			"html":     rendered.HTML,
			"text":     rendered.Text,
			"telegram": rendered.Telegram,
			"warnings": warnings,
		}
	}
	writeJSON(w, http.StatusOK, out)
}
