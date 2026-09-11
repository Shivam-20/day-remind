package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DatabaseURL        string
	WebPort            int
	SessionKey         string
	AdminEmails        []string
	ResendAPIKey       string
	EmailFrom          string
	TelegramBotToken   string
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string
	VAPIDPublic        string
	VAPIDPrivate       string
	TwilioAccountSID   string
	TwilioAuthToken    string
	TwilioFrom         string
	GeminiAPIKey       string
	WebhookBaseURL     string
	MaxFreeReminders   int
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
	maxFree := 50
	if raw := strings.TrimSpace(os.Getenv("MAX_FREE_REMINDERS")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return Config{}, fmt.Errorf("invalid MAX_FREE_REMINDERS: %q", raw)
		}
		maxFree = n
	}
	return Config{
		DatabaseURL:        dbURL,
		WebPort:            port,
		SessionKey:         sessionKey,
		AdminEmails:        parseList(os.Getenv("ADMIN_EMAILS")),
		ResendAPIKey:       strings.TrimSpace(os.Getenv("RESEND_API_KEY")),
		EmailFrom:          strings.TrimSpace(os.Getenv("EMAIL_FROM")),
		TelegramBotToken:   strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		GoogleClientID:     strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID")),
		GoogleClientSecret: strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_SECRET")),
		GoogleRedirectURL:  strings.TrimSpace(os.Getenv("GOOGLE_REDIRECT_URL")),
		VAPIDPublic:        strings.TrimSpace(os.Getenv("VAPID_PUBLIC_KEY")),
		VAPIDPrivate:       strings.TrimSpace(os.Getenv("VAPID_PRIVATE_KEY")),
		TwilioAccountSID:   strings.TrimSpace(os.Getenv("TWILIO_ACCOUNT_SID")),
		TwilioAuthToken:    strings.TrimSpace(os.Getenv("TWILIO_AUTH_TOKEN")),
		TwilioFrom:         strings.TrimSpace(os.Getenv("TWILIO_FROM")),
		GeminiAPIKey:       strings.TrimSpace(os.Getenv("GEMINI_API_KEY")),
		WebhookBaseURL:     strings.TrimSpace(os.Getenv("WEBHOOK_BASE_URL")),
		MaxFreeReminders:   maxFree,
	}, nil
}

func parseList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
