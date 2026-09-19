package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shivam-20/day-remind/internal/config"
	"github.com/shivam-20/day-remind/internal/storage"
)

type DeliveryConfig struct {
	EmailEnabled     bool
	EmailFrom        string
	ResendAPIKey     string
	TelegramEnabled  bool
	TelegramBotToken string
	WebPushEnabled   bool
	VAPIDPublic      string
	VAPIDPrivate     string
	SMSEnabled       bool
	TwilioAccountSID string
	TwilioAuthToken  string
	TwilioFrom       string
	SlackEnabled     bool
}

func EffectiveDelivery(cfg config.Config, stored storage.DeliverySettings) DeliveryConfig {
	emailFrom := firstNonEmpty(stored.EmailFrom, cfg.EmailFrom)
	resendKey := firstNonEmpty(stored.ResendAPIKey, cfg.ResendAPIKey)
	tgToken := firstNonEmpty(stored.TelegramBotToken, cfg.TelegramBotToken)
	vapidPublic := strings.TrimSpace(cfg.VAPIDPublic)
	vapidPrivate := strings.TrimSpace(cfg.VAPIDPrivate)
	twilioSID := strings.TrimSpace(cfg.TwilioAccountSID)
	twilioToken := strings.TrimSpace(cfg.TwilioAuthToken)
	twilioFrom := strings.TrimSpace(cfg.TwilioFrom)

	hasStored := !stored.UpdatedAt.IsZero()
	emailEnabled := stored.EmailEnabled
	if !hasStored && resendKey != "" && emailFrom != "" {
		emailEnabled = true
	}
	tgEnabled := stored.TelegramEnabled
	if !hasStored && tgToken != "" {
		tgEnabled = true
	}
	webPushEnabled := vapidPublic != "" && vapidPrivate != ""
	smsEnabled := twilioSID != "" && twilioToken != "" && twilioFrom != ""

	return DeliveryConfig{
		EmailEnabled:     emailEnabled,
		EmailFrom:        emailFrom,
		ResendAPIKey:     resendKey,
		TelegramEnabled:  tgEnabled,
		TelegramBotToken: tgToken,
		WebPushEnabled:   webPushEnabled,
		VAPIDPublic:      vapidPublic,
		VAPIDPrivate:     vapidPrivate,
		SMSEnabled:       smsEnabled,
		TwilioAccountSID: twilioSID,
		TwilioAuthToken:  twilioToken,
		TwilioFrom:       twilioFrom,
		SlackEnabled:     true,
	}
}

func ChannelAvailability(d DeliveryConfig) map[string]bool {
	emailReady := d.ResendAPIKey != "" && d.EmailFrom != ""
	tgReady := d.TelegramBotToken != ""
	webPushReady := d.VAPIDPublic != "" && d.VAPIDPrivate != ""
	smsReady := d.TwilioAccountSID != "" && d.TwilioAuthToken != "" && d.TwilioFrom != ""
	return map[string]bool{
		storage.ChannelEmail:    emailReady && d.EmailEnabled,
		storage.ChannelTelegram: tgReady && d.TelegramEnabled,
		storage.ChannelWebPush:  webPushReady && d.WebPushEnabled,
		storage.ChannelSMS:      smsReady && d.SMSEnabled,
		storage.ChannelSlack:    d.SlackEnabled,
	}
}

func SendEmail(ctx context.Context, d DeliveryConfig, to, subject, body string) error {
	if !d.EmailEnabled || d.ResendAPIKey == "" || d.EmailFrom == "" {
		return fmt.Errorf("email delivery is not configured")
	}
	to = strings.TrimSpace(strings.ToLower(to))
	if to == "" {
		return fmt.Errorf("recipient email is required")
	}
	payload := map[string]any{
		"from":    d.EmailFrom,
		"to":      []string{to},
		"subject": subject,
		"html":    body,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.ResendAPIKey)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return fmt.Errorf("resend: %s", strings.TrimSpace(string(msg)))
	}
	return nil
}

// SendWebPush delivers a browser push notification using VAPID keys from config.
func SendWebPush(ctx context.Context, d DeliveryConfig, subscriptionJSON, title, body string) error {
	if !d.WebPushEnabled {
		return fmt.Errorf("web push delivery is not configured")
	}
	return deliverWebPush(ctx, subscriptionJSON, d.VAPIDPublic, d.VAPIDPrivate, title, body)
}

// SendSMS sends an SMS via Twilio when TWILIO_* credentials are set.
func SendSMS(ctx context.Context, cfg SMSConfig, to, body string) error {
	if strings.TrimSpace(cfg.AccountSID) == "" || strings.TrimSpace(cfg.AuthToken) == "" || strings.TrimSpace(cfg.From) == "" {
		return fmt.Errorf("sms delivery is not configured")
	}
	return deliverSMS(ctx, cfg, to, body)
}

// SendSlack posts a message to a Slack incoming webhook.
func SendSlack(ctx context.Context, webhookURL, text string) error {
	return deliverSlack(ctx, webhookURL, text)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func MaskSecret(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return "****"
	}
	return "..." + s[len(s)-4:]
}
