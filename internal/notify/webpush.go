package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// deliverWebPush sends a push notification to a browser subscription JSON blob.
func deliverWebPush(ctx context.Context, subscriptionJSON, vapidPublic, vapidPrivate, title, body string) error {
	subscriptionJSON = strings.TrimSpace(subscriptionJSON)
	if subscriptionJSON == "" {
		return fmt.Errorf("web push subscription is required")
	}
	if strings.TrimSpace(vapidPublic) == "" || strings.TrimSpace(vapidPrivate) == "" {
		return fmt.Errorf("VAPID keys are not configured")
	}
	var sub webpush.Subscription
	if err := json.Unmarshal([]byte(subscriptionJSON), &sub); err != nil {
		return fmt.Errorf("invalid web push subscription: %w", err)
	}
	payload, err := json.Marshal(map[string]string{
		"title": title,
		"body":  body,
	})
	if err != nil {
		return err
	}
	resp, err := webpush.SendNotificationWithContext(ctx, payload, &sub, &webpush.Options{
		Subscriber:      "mailto:reminders@dayremind.app",
		VAPIDPublicKey:  strings.TrimSpace(vapidPublic),
		VAPIDPrivateKey: strings.TrimSpace(vapidPrivate),
		TTL:             3600,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("web push: HTTP %d", resp.StatusCode)
	}
	return nil
}

// WebPushSender implements Sender for browser push notifications.
type WebPushSender struct {
	VAPIDPublic  string
	VAPIDPrivate string
}

func (s WebPushSender) Channel() string { return "webpush" }

func (s WebPushSender) Send(ctx context.Context, msg DeliveryMessage) error {
	title := msg.Subject
	if title == "" {
		title = "Day Remind"
	}
	body := msg.Text
	if body == "" {
		body = msg.Body
	}
	return deliverWebPush(ctx, msg.To, s.VAPIDPublic, s.VAPIDPrivate, title, body)
}
