package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DatabaseURL                string
	WebPort                    int
	SessionKey                 string
	AdminEmails                []string
	ResendAPIKey               string
	EmailFrom                  string
	SMTPHost                   string
	SMTPPort                   int
	SMTPUsername               string
	SMTPPassword               string
	SMTPTLSMode                string
	TelegramBotToken           string
	TelegramUpdateMode         string
	TelegramWebhookSecret      string
	TelegramGatewayURL         string
	TelegramGatewayAPIToken    string
	TelegramGatewayInboundMode string
	GoogleClientID             string
	GoogleClientSecret         string
	GoogleRedirectURL          string
	VAPIDPublic                string
	VAPIDPrivate               string
	TwilioAccountSID           string
	TwilioAuthToken            string
	TwilioFrom                 string
	AIProvider                 string
	GeminiAPIKey               string
	GeminiModel                string
	OpenRouterAPIKey           string
	OpenRouterModel            string
	WebhookBaseURL             string
	MaxFreeReminders           int
	BasePath                   string
	AuthJWTSecret              string
	GatewayCookie              string
	JWTIssuer                  string
	TrustAuthHeaders           bool
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
	smtpPort := 587
	if raw := strings.TrimSpace(os.Getenv("SMTP_PORT")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 65535 {
			return Config{}, fmt.Errorf("invalid SMTP_PORT: %q", raw)
		}
		smtpPort = n
	}
	tgMode := strings.ToLower(strings.TrimSpace(os.Getenv("TELEGRAM_UPDATE_MODE")))
	switch tgMode {
	case "", "polling":
		tgMode = "polling"
	case "webhook", "off":
	default:
		return Config{}, fmt.Errorf("invalid TELEGRAM_UPDATE_MODE: %q", tgMode)
	}
	gatewayCookie := strings.TrimSpace(os.Getenv("GATEWAY_COOKIE"))
	if gatewayCookie == "" {
		gatewayCookie = "auth_token"
	}
	jwtIssuer := strings.TrimSpace(os.Getenv("JWT_ISSUER"))
	if jwtIssuer == "" {
		jwtIssuer = "home"
	}
	return Config{
		DatabaseURL:                dbURL,
		WebPort:                    port,
		SessionKey:                 sessionKey,
		AdminEmails:                parseList(os.Getenv("ADMIN_EMAILS")),
		ResendAPIKey:               strings.TrimSpace(os.Getenv("RESEND_API_KEY")),
		EmailFrom:                  strings.TrimSpace(os.Getenv("EMAIL_FROM")),
		SMTPHost:                   strings.TrimSpace(os.Getenv("SMTP_HOST")),
		SMTPPort:                   smtpPort,
		SMTPUsername:               strings.TrimSpace(os.Getenv("SMTP_USERNAME")),
		SMTPPassword:               strings.TrimSpace(os.Getenv("SMTP_PASSWORD")),
		SMTPTLSMode:                strings.TrimSpace(os.Getenv("SMTP_TLS")),
		TelegramBotToken:           strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		TelegramUpdateMode:         tgMode,
		TelegramWebhookSecret:      strings.TrimSpace(os.Getenv("TELEGRAM_WEBHOOK_SECRET")),
		TelegramGatewayURL:         strings.TrimSpace(os.Getenv("TELEGRAM_GATEWAY_URL")),
		TelegramGatewayAPIToken:    strings.TrimSpace(os.Getenv("TELEGRAM_GATEWAY_API_TOKEN")),
		TelegramGatewayInboundMode: normalizeGatewayInboundMode(strings.TrimSpace(os.Getenv("TELEGRAM_GATEWAY_INBOUND_MODE"))),
		GoogleClientID:             strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID")),
		GoogleClientSecret:         strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_SECRET")),
		GoogleRedirectURL:          strings.TrimSpace(os.Getenv("GOOGLE_REDIRECT_URL")),
		VAPIDPublic:                strings.TrimSpace(os.Getenv("VAPID_PUBLIC_KEY")),
		VAPIDPrivate:               strings.TrimSpace(os.Getenv("VAPID_PRIVATE_KEY")),
		TwilioAccountSID:           strings.TrimSpace(os.Getenv("TWILIO_ACCOUNT_SID")),
		TwilioAuthToken:            strings.TrimSpace(os.Getenv("TWILIO_AUTH_TOKEN")),
		TwilioFrom:                 strings.TrimSpace(os.Getenv("TWILIO_FROM")),
		AIProvider:                 strings.ToLower(strings.TrimSpace(os.Getenv("AI_PROVIDER"))),
		GeminiAPIKey:               strings.TrimSpace(os.Getenv("GEMINI_API_KEY")),
		GeminiModel:                firstNonEmptyEnv("GEMINI_MODEL", "gemini-2.5-flash"),
		OpenRouterAPIKey:           strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")),
		OpenRouterModel:            firstNonEmptyEnv("OPENROUTER_MODEL", "google/gemini-2.5-flash"),
		WebhookBaseURL:             strings.TrimSpace(os.Getenv("WEBHOOK_BASE_URL")),
		MaxFreeReminders:           maxFree,
		BasePath:                   strings.Trim(os.Getenv("APP_BASE_PATH"), "/"),
		AuthJWTSecret:              strings.TrimSpace(os.Getenv("AUTH_JWT_SECRET")),
		GatewayCookie:              gatewayCookie,
		JWTIssuer:                  jwtIssuer,
		TrustAuthHeaders:           strings.EqualFold(strings.TrimSpace(os.Getenv("TRUST_AUTH_HEADERS")), "true"),
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

// firstNonEmptyEnv returns the trimmed env value, or fallback when unset/empty.
func firstNonEmptyEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// normalizeGatewayInboundMode validates the TELEGRAM_GATEWAY_INBOUND_MODE env value.
// Accepted values: "poll" (default), "webhook", "both".
func normalizeGatewayInboundMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "webhook", "both":
		return strings.ToLower(strings.TrimSpace(mode))
	default:
		return "poll"
	}
}

// HasTelegramGateway reports whether the Telegram gateway is configured via env.
func (c Config) HasTelegramGateway() bool {
	return strings.TrimSpace(c.TelegramGatewayURL) != "" && strings.TrimSpace(c.TelegramGatewayAPIToken) != ""
}

