package notify

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SMSConfig holds Twilio credentials for SMS delivery.
type SMSConfig struct {
	AccountSID string
	AuthToken  string
	From       string
}

// deliverSMS sends a text message via Twilio when credentials are configured.
func deliverSMS(ctx context.Context, cfg SMSConfig, to, body string) error {
	accountSID := strings.TrimSpace(cfg.AccountSID)
	authToken := strings.TrimSpace(cfg.AuthToken)
	from := strings.TrimSpace(cfg.From)
	to = strings.TrimSpace(to)
	body = strings.TrimSpace(body)
	if accountSID == "" || authToken == "" || from == "" {
		return fmt.Errorf("twilio SMS is not configured")
	}
	if to == "" {
		return fmt.Errorf("sms recipient is required")
	}
	if body == "" {
		return fmt.Errorf("sms body is required")
	}

	endpoint := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", accountSID)
	form := url.Values{}
	form.Set("To", to)
	form.Set("From", from)
	form.Set("Body", body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(accountSID, authToken)

	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode >= 300 {
		return fmt.Errorf("twilio: %s", strings.TrimSpace(string(raw)))
	}
	return nil
}

// SMSSender implements Sender for Twilio SMS.
type SMSSender struct {
	Config SMSConfig
}

func (s SMSSender) Channel() string { return "sms" }

func (s SMSSender) Send(ctx context.Context, msg DeliveryMessage) error {
	body := msg.Text
	if body == "" {
		body = msg.Body
	}
	return deliverSMS(ctx, s.Config, msg.To, body)
}
