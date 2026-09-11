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
)

// SlackPayload is a minimal Slack incoming-webhook body.
type SlackPayload struct {
	Text string `json:"text"`
}

// deliverSlack posts a message to a Slack incoming webhook URL.
func deliverSlack(ctx context.Context, webhookURL, text string) error {
	webhookURL = strings.TrimSpace(webhookURL)
	text = strings.TrimSpace(text)
	if webhookURL == "" {
		return fmt.Errorf("slack webhook URL is required")
	}
	if text == "" {
		return fmt.Errorf("slack message text is required")
	}
	payload := SlackPayload{Text: text}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(b))
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
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode >= 300 {
		return fmt.Errorf("slack: %s", strings.TrimSpace(string(raw)))
	}
	return nil
}

// SlackSender implements Sender for Slack webhooks.
type SlackSender struct{}

func (s SlackSender) Channel() string { return "slack" }

func (s SlackSender) Send(ctx context.Context, msg DeliveryMessage) error {
	text := msg.Text
	if text == "" {
		text = msg.Body
	}
	if msg.Subject != "" {
		text = "*" + msg.Subject + "*\n" + text
	}
	return deliverSlack(ctx, msg.To, text)
}
