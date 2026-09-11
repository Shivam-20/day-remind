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
}

func EffectiveDelivery(cfg config.Config, stored storage.DeliverySettings) DeliveryConfig {
	emailFrom := firstNonEmpty(stored.EmailFrom, cfg.EmailFrom)
	resendKey := firstNonEmpty(stored.ResendAPIKey, cfg.ResendAPIKey)
	tgToken := firstNonEmpty(stored.TelegramBotToken, cfg.TelegramBotToken)

	hasStored := !stored.UpdatedAt.IsZero()
	emailEnabled := stored.EmailEnabled
	if !hasStored && resendKey != "" && emailFrom != "" {
		emailEnabled = true
	}
	tgEnabled := stored.TelegramEnabled
	if !hasStored && tgToken != "" {
		tgEnabled = true
	}

	return DeliveryConfig{
		EmailEnabled:     emailEnabled,
		EmailFrom:        emailFrom,
		ResendAPIKey:     resendKey,
		TelegramEnabled:  tgEnabled,
		TelegramBotToken: tgToken,
	}
}

func ChannelAvailability(d DeliveryConfig) map[string]bool {
	emailReady := d.ResendAPIKey != "" && d.EmailFrom != ""
	tgReady := d.TelegramBotToken != ""
	return map[string]bool{
		storage.ChannelEmail:    emailReady && d.EmailEnabled,
		storage.ChannelTelegram: tgReady && d.TelegramEnabled,
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

func SendTelegram(ctx context.Context, d DeliveryConfig, chatID, text string) error {
	if !d.TelegramEnabled || d.TelegramBotToken == "" {
		return fmt.Errorf("telegram delivery is not configured")
	}
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return fmt.Errorf("telegram chat id is required")
	}
	payload := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", d.TelegramBotToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode >= 300 {
		return fmt.Errorf("telegram: %s", strings.TrimSpace(string(body)))
	}
	var out struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &out); err != nil || !out.OK {
		if out.Description != "" {
			return fmt.Errorf("telegram: %s", out.Description)
		}
		return fmt.Errorf("telegram send failed")
	}
	return nil
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
