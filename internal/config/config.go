package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL string
	WebPort     int
	SessionKey  string
	// SMTP holds outbound email settings. Email delivery is enabled only when
	// Host and From are both set.
	SMTP SMTPConfig
	// TelegramBotToken enables the Telegram channel and the command bot.
	TelegramBotToken string
	// TelegramAPIBase is overridable so tests can point at a local stub
	// instead of the real Telegram API.
	TelegramAPIBase string
	// SchedulerInterval is how often due reminders are swept.
	SchedulerInterval time.Duration
	// AllowPrivateSMTP permits a user-supplied mail server on a loopback or
	// private address. Off by default, because it otherwise lets any user make
	// this process connect to an internal service. Development against the
	// MailHog container needs it on.
	AllowPrivateSMTP bool

	// AdminEmails is the bootstrap allowlist. A user whose email appears here
	// is promoted to admin on their next signup or login, which keeps the role
	// grantable only by whoever controls the server environment.
	AdminEmails map[string]bool
}

// IsAdminEmail reports whether the address is pinned to admin by configuration.
// A pinned admin cannot be demoted from the UI, since the next login would
// simply restore the role.
func (c Config) IsAdminEmail(email string) bool {
	return c.AdminEmails[strings.TrimSpace(strings.ToLower(email))]
}

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	// TLS selects the transport: "starttls" (default, port 587),
	// "implicit" (SMTPS, port 465) or "none" for a local catcher like MailHog.
	TLS string
}

// Enabled reports whether enough is configured to attempt email delivery.
func (c SMTPConfig) Enabled() bool {
	return c.Host != "" && c.From != ""
}

func (c Config) TelegramEnabled() bool {
	return c.TelegramBotToken != ""
}

func loadSMTP() (SMTPConfig, error) {
	c := SMTPConfig{
		Host:     strings.TrimSpace(os.Getenv("SMTP_HOST")),
		Username: os.Getenv("SMTP_USERNAME"),
		Password: os.Getenv("SMTP_PASSWORD"),
		From:     strings.TrimSpace(os.Getenv("SMTP_FROM")),
		TLS:      strings.ToLower(strings.TrimSpace(os.Getenv("SMTP_TLS"))),
		Port:     587,
	}
	if raw := strings.TrimSpace(os.Getenv("SMTP_PORT")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 65535 {
			return SMTPConfig{}, fmt.Errorf("invalid SMTP_PORT: %q", raw)
		}
		c.Port = n
	}
	if c.TLS == "" {
		c.TLS = "starttls"
	}
	switch c.TLS {
	case "starttls", "implicit", "none":
	default:
		return SMTPConfig{}, fmt.Errorf("invalid SMTP_TLS: %q (want starttls, implicit or none)", c.TLS)
	}
	if c.Host != "" && c.From == "" {
		return SMTPConfig{}, fmt.Errorf("SMTP_FROM is required when SMTP_HOST is set")
	}
	return c, nil
}

func truthy(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func parseAdminEmails(raw string) map[string]bool {
	out := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		if e := strings.TrimSpace(strings.ToLower(part)); e != "" {
			out[e] = true
		}
	}
	return out
}

func Load() (Config, error) {
	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dbURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	sessionKey := strings.TrimSpace(os.Getenv("SESSION_KEY"))
	if len(sessionKey) < 32 {
		return Config{}, fmt.Errorf("SESSION_KEY must be at least 32 characters")
	}
	port := 8090
	if raw := strings.TrimSpace(os.Getenv("WEB_PORT")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return Config{}, fmt.Errorf("invalid WEB_PORT: %q", raw)
		}
		port = n
	}
	smtpCfg, err := loadSMTP()
	if err != nil {
		return Config{}, err
	}
	interval := 30 * time.Second
	if raw := strings.TrimSpace(os.Getenv("SCHEDULER_INTERVAL")); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d < time.Second {
			return Config{}, fmt.Errorf("invalid SCHEDULER_INTERVAL: %q (e.g. 30s)", raw)
		}
		interval = d
	}
	apiBase := strings.TrimRight(strings.TrimSpace(os.Getenv("TELEGRAM_API_BASE")), "/")
	if apiBase == "" {
		apiBase = "https://api.telegram.org"
	}
	return Config{
		DatabaseURL:       dbURL,
		WebPort:           port,
		SessionKey:        sessionKey,
		SMTP:              smtpCfg,
		TelegramBotToken:  strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		TelegramAPIBase:   apiBase,
		SchedulerInterval: interval,
		AllowPrivateSMTP:  truthy(os.Getenv("ALLOW_PRIVATE_SMTP")),
		AdminEmails:       parseAdminEmails(os.Getenv("ADMIN_EMAILS")),
	}, nil
}
