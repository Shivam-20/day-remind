// Package webhook delivers signed reminder events to user-configured HTTP
// endpoints. Events are written to the webhook_deliveries outbox first, sent
// right away when a worker is free, and retried with backoff by Run.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/reminder"
	"github.com/shivam-20/day-remind/internal/storage"
)

const (
	EventReminderFired = "reminder.fired"
	EventReminderDone  = "reminder.done"
	EventPing          = "ping"
)

// Events lists the events a user can subscribe to.
var Events = []string{EventReminderFired, EventReminderDone}

const (
	SignatureHeader = "X-DayRemind-Signature"
	EventHeader     = "X-DayRemind-Event"
	DeliveryHeader  = "X-DayRemind-Delivery"

	maxAttempts  = 5
	sendTimeout  = 10 * time.Second
	claimLease   = 2 * time.Minute
	pollInterval = 15 * time.Second
	workers      = 4
)

// retryBackoff is the wait after failed attempt n (1-based).
var retryBackoff = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour}

type Dispatcher struct {
	db      *storage.DB
	baseURL string
	client  *http.Client
	sem     chan struct{}
}

// New returns a dispatcher. baseURL is the public app URL used for deep
// links in payloads (WEBHOOK_BASE_URL); it may be empty.
func New(db *storage.DB, baseURL string) *Dispatcher {
	return &Dispatcher{
		db:      db,
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		client:  newSafeClient(sendTimeout, false),
		sem:     make(chan struct{}, workers),
	}
}

// ValidateEvents normalizes and checks a subscription list.
func ValidateEvents(in []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, e := range in {
		e = strings.ToLower(strings.TrimSpace(e))
		valid := false
		for _, known := range Events {
			valid = valid || e == known
		}
		if !valid {
			return nil, fmt.Errorf("unknown event %q (supported: %s)", e, strings.Join(Events, ", "))
		}
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("at least one event is required")
	}
	return out, nil
}

// GenerateSecret returns a new random signing secret.
func GenerateSecret() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "whsec_" + hex.EncodeToString(b), nil
}

// Sign computes the v1 signature: hex(HMAC-SHA256(secret, "<unix ts>.<body>")).
// Receivers should recompute it, compare in constant time, and reject stale
// timestamps to prevent replay.
func Sign(secret string, ts int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// EmitFired queues reminder.fired for the reminder's due time. Safe to call on
// every scheduler tick: the idempotency key makes repeats no-ops.
func (d *Dispatcher) EmitFired(ctx context.Context, item reminder.Reminder) {
	d.emit(ctx, item, EventReminderFired)
}

// EmitDone queues reminder.done. Pass the reminder as returned by the update,
// before any recurrence moves its due date, so each occurrence fires once.
func (d *Dispatcher) EmitDone(ctx context.Context, item reminder.Reminder) {
	d.emit(ctx, item, EventReminderDone)
}

func (d *Dispatcher) emit(ctx context.Context, item reminder.Reminder, event string) {
	if d == nil {
		return
	}
	hooks, err := d.db.ListWebhooksForEvent(ctx, item.UserID, event)
	if err != nil {
		log.Printf("webhook: list endpoints: %v", err)
		return
	}
	if len(hooks) == 0 {
		return
	}
	body, err := json.Marshal(d.payload(uuid.New(), event, item))
	if err != nil {
		return
	}
	for _, h := range hooks {
		// The scheduler re-emits for every overdue reminder each tick; don't
		// flood a new endpoint with reminders that fired before it existed.
		if event == EventReminderFired && item.DueAt.Before(h.CreatedAt) {
			continue
		}
		key := strings.Join([]string{h.ID.String(), event, item.ID.String(), item.DueAt.UTC().Format(time.RFC3339)}, ":")
		row, err := d.db.CreateWebhookDelivery(ctx, storage.WebhookDelivery{
			WebhookID:      h.ID,
			UserID:         item.UserID,
			Event:          event,
			Payload:        string(body),
			IdempotencyKey: key,
		})
		if err == storage.ErrConflict {
			continue
		}
		if err != nil {
			log.Printf("webhook: enqueue: %v", err)
			continue
		}
		d.kick(row.ID)
	}
}

// kick tries to send a freshly queued row now; if all workers are busy the
// Run loop picks it up on its next pass.
func (d *Dispatcher) kick(id uuid.UUID) {
	select {
	case d.sem <- struct{}{}:
		go func() {
			defer func() { <-d.sem }()
			ctx, cancel := context.WithTimeout(context.Background(), sendTimeout+5*time.Second)
			defer cancel()
			row, err := d.db.ClaimWebhookDelivery(ctx, &id, time.Now().UTC(), claimLease, maxAttempts)
			if err != nil {
				return
			}
			d.process(ctx, row)
		}()
	default:
	}
}

// Run drains due deliveries (new rows nobody kicked, retries, expired leases)
// until ctx is cancelled.
func (d *Dispatcher) Run(ctx context.Context) {
	if d == nil {
		return
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		d.drain(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (d *Dispatcher) drain(ctx context.Context) {
	for {
		select {
		case d.sem <- struct{}{}:
		case <-ctx.Done():
			return
		}
		row, err := d.db.ClaimWebhookDelivery(ctx, nil, time.Now().UTC(), claimLease, maxAttempts)
		if err != nil {
			<-d.sem
			if err != storage.ErrNotFound {
				log.Printf("webhook: claim: %v", err)
			}
			return
		}
		go func() {
			defer func() { <-d.sem }()
			sendCtx, cancel := context.WithTimeout(context.Background(), sendTimeout+5*time.Second)
			defer cancel()
			d.process(sendCtx, row)
		}()
	}
}

func (d *Dispatcher) process(ctx context.Context, row storage.WebhookDelivery) {
	hook, err := d.db.GetWebhookByID(ctx, row.WebhookID)
	if err != nil || !hook.Enabled {
		_ = d.db.FailWebhookDelivery(ctx, row.ID, "endpoint disabled or deleted")
		return
	}
	code, sendErr := d.post(ctx, hook, row.ID, row.Event, []byte(row.Payload))
	ok := sendErr == nil
	msg := ""
	if !ok {
		msg = sendErr.Error()
	}
	// A blocked address will not fix itself between retries.
	final := ok || row.AttemptCount >= maxAttempts || errors.Is(sendErr, ErrBlockedAddress)
	var retryAt *time.Time
	if !final {
		t := time.Now().UTC().Add(retryBackoff[min(row.AttemptCount, len(retryBackoff))-1])
		retryAt = &t
	}
	if err := d.db.FinishWebhookDelivery(ctx, row.ID, ok, code, msg, retryAt); err != nil {
		log.Printf("webhook: finish delivery: %v", err)
	}
	_ = d.db.RecordWebhookAttempt(ctx, hook.ID, ok, !ok && final, code, msg)
}

// Ping sends a synchronous test event so the user can check their endpoint.
func (d *Dispatcher) Ping(ctx context.Context, hook storage.Webhook) (int, error) {
	body, err := json.Marshal(map[string]any{
		"id":         uuid.New().String(),
		"event":      EventPing,
		"created_at": time.Now().UTC().Format(time.RFC3339),
		"data":       map[string]any{"webhook_id": hook.ID.String()},
	})
	if err != nil {
		return 0, err
	}
	code, sendErr := d.post(ctx, hook, uuid.New(), EventPing, body)
	msg := ""
	if sendErr != nil {
		msg = sendErr.Error()
	}
	_ = d.db.RecordWebhookAttempt(ctx, hook.ID, sendErr == nil, false, code, msg)
	return code, sendErr
}

// post sends one signed request. A non-2xx status is an error.
func (d *Dispatcher) post(ctx context.Context, hook storage.Webhook, deliveryID uuid.UUID, event string, body []byte) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hook.URL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	ts := time.Now().Unix()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "DayRemind-Webhooks/1.0")
	req.Header.Set(EventHeader, event)
	req.Header.Set(DeliveryHeader, deliveryID.String())
	req.Header.Set(SignatureHeader, fmt.Sprintf("t=%d,v1=%s", ts, Sign(hook.Secret, ts, body)))
	res, err := d.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return res.StatusCode, fmt.Errorf("endpoint returned HTTP %d", res.StatusCode)
	}
	return res.StatusCode, nil
}

func (d *Dispatcher) payload(eventID uuid.UUID, event string, item reminder.Reminder) map[string]any {
	r := map[string]any{
		"id":          item.ID.String(),
		"title":       item.Title,
		"description": item.Description,
		"status":      item.Status,
		"priority":    item.Priority,
		"due_at":      item.DueAt.UTC().Format(time.RFC3339),
		"timezone":    item.Timezone,
	}
	if item.CompletedAt != nil {
		r["completed_at"] = item.CompletedAt.UTC().Format(time.RFC3339)
	}
	if item.RecurrenceRule != "" {
		r["recurrence_rule"] = item.RecurrenceRule
	}
	if item.ListID != nil {
		r["list_id"] = item.ListID.String()
	}
	if d.baseURL != "" {
		r["url"] = d.baseURL + "/#/reminders?open=" + item.ID.String()
	}
	return map[string]any{
		"id":         eventID.String(),
		"event":      event,
		"created_at": time.Now().UTC().Format(time.RFC3339),
		"data":       map[string]any{"reminder": r},
	}
}
