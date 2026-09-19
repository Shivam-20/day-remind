// Package delivery decides which sender and which templates apply to a given
// user, resolving the instance configuration against that user's overrides.
//
// It exists to keep internal/notify free of any storage dependency: notify
// renders and transmits, storage persists, and this package joins the two.
package delivery

import (
	"context"
	"log"
	"strconv"

	"github.com/shivam-20/day-remind/internal/notify"
	"github.com/shivam-20/day-remind/internal/secret"
	"github.com/shivam-20/day-remind/internal/storage"
)

// Target is one resolved destination: which sender, which address, and the
// templates to render with.
type Target struct {
	Sender    notify.Sender
	Dest      string
	Channel   string
	Templates notify.TemplateSet
}

// Resolver builds senders on demand from stored configuration.
type Resolver struct {
	db      *storage.DB
	box     *secret.Box
	apiBase string
}

func NewResolver(db *storage.DB, box *secret.Box, telegramAPIBase string) *Resolver {
	return &Resolver{db: db, box: box, apiBase: telegramAPIBase}
}

// Snapshot is the instance configuration read once, then applied to many users.
// The scheduler takes one per sweep so a settings document is not re-read for
// every reminder.
type Snapshot struct {
	r        *Resolver
	settings storage.InstanceSettings

	// Decrypted once per snapshot rather than per user.
	smtpPassword  string
	telegramToken string
	// Set when a stored credential could not be decrypted, which in practice
	// means SESSION_KEY changed.
	SMTPUnreadable     bool
	TelegramUnreadable bool
}

// Snapshot reads the instance settings and decrypts its credentials.
func (r *Resolver) Snapshot(ctx context.Context) (*Snapshot, error) {
	s, err := r.db.GetInstanceSettings(ctx)
	if err != nil {
		return nil, err
	}
	snap := &Snapshot{r: r, settings: s}

	if s.SMTP.PasswordEnc != "" {
		pw, err := r.box.Decrypt(s.SMTP.PasswordEnc)
		if err != nil {
			// Not fatal: an unauthenticated relay still works, and a channel
			// that genuinely needs the password will report a clear auth error.
			snap.SMTPUnreadable = true
			log.Printf("delivery: instance SMTP password is unreadable: %v", err)
		}
		snap.smtpPassword = pw
	}
	if s.Telegram.BotTokenEnc != "" {
		tok, err := r.box.Decrypt(s.Telegram.BotTokenEnc)
		if err != nil {
			snap.TelegramUnreadable = true
			log.Printf("delivery: instance Telegram token is unreadable: %v", err)
		}
		snap.telegramToken = tok
	}
	return snap, nil
}

// Settings exposes the instance configuration this snapshot was built from.
func (s *Snapshot) Settings() storage.InstanceSettings { return s.settings }

// InstanceTelegramToken is the decrypted bot token, empty when unset or
// unreadable. The command bot uses it to detect a token change.
func (s *Snapshot) InstanceTelegramToken() string {
	if !s.settings.Telegram.Complete() {
		return ""
	}
	return s.telegramToken
}

// EmailAvailable and TelegramAvailable report whether a channel can be used at
// all — either the instance provides it, or this user brought their own.
func (s *Snapshot) EmailAvailable(u storage.User) bool {
	return u.Delivery.SMTP.Complete() || s.settings.SMTP.Complete()
}

func (s *Snapshot) TelegramAvailable(u storage.User) bool {
	return u.Delivery.Telegram.Complete() || (s.settings.Telegram.Complete() && u.TelegramChatID != nil)
}

// For resolves the destinations for one user, honouring their channel toggles.
// Precedence is the user's own configuration first, then the instance's.
func (s *Snapshot) For(u storage.User) []Target {
	templates := notify.Merge(s.settings.Templates, u.Templates)
	var out []Target

	if u.Notify.EmailEnabled && u.Notify.EmailAddress != "" {
		if sender := s.emailSender(u); sender != nil {
			out = append(out, Target{
				Sender: sender, Dest: u.Notify.EmailAddress,
				Channel: notify.ChannelEmail, Templates: templates,
			})
		}
	}

	if u.Notify.TelegramEnabled {
		if sender, dest := s.telegramSender(u); sender != nil {
			out = append(out, Target{
				Sender: sender, Dest: dest,
				Channel: notify.ChannelTelegram, Templates: templates,
			})
		}
	}
	return out
}

func (s *Snapshot) emailSender(u storage.User) notify.Sender {
	if cfg := u.Delivery.SMTP; cfg.Complete() {
		pw, err := s.r.box.Decrypt(cfg.PasswordEnc)
		if err != nil {
			log.Printf("delivery: %s has an unreadable SMTP password, skipping their own server", u.Email)
			return nil
		}
		return smtpFrom(cfg, pw)
	}
	if s.settings.SMTP.Complete() {
		return smtpFrom(s.settings.SMTP, s.smtpPassword)
	}
	return nil
}

func (s *Snapshot) telegramSender(u storage.User) (notify.Sender, string) {
	if cfg := u.Delivery.Telegram; cfg.Complete() {
		tok, err := s.r.box.Decrypt(cfg.BotTokenEnc)
		if err != nil {
			log.Printf("delivery: %s has an unreadable bot token, skipping their own bot", u.Email)
			return nil, ""
		}
		return notify.NewTelegramClient(tok, s.r.apiBase), strconv.FormatInt(*cfg.OwnChatID, 10)
	}
	if s.settings.Telegram.Complete() && s.telegramToken != "" && u.TelegramChatID != nil {
		return notify.NewTelegramClient(s.telegramToken, s.r.apiBase), strconv.FormatInt(*u.TelegramChatID, 10)
	}
	return nil, ""
}

func smtpFrom(cfg storage.SMTPSettings, password string) *notify.SMTPSender {
	cfg = cfg.Normalize()
	return &notify.SMTPSender{
		Host:     cfg.Host,
		Port:     cfg.Port,
		Username: cfg.Username,
		Password: password,
		From:     cfg.From,
		Mode:     cfg.TLS,
	}
}
