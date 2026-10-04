package storage

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestWebhookOutbox(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()
	userID := uuid.New()

	hook, err := db.CreateWebhook(ctx, userID, "https://example.com/h", "whsec_x", []string{"reminder.done"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.DeleteWebhook(ctx, userID, hook.ID) }()

	if got, _ := db.ListWebhooksForEvent(ctx, userID, "reminder.done"); len(got) != 1 {
		t.Fatalf("subscribed endpoints = %d, want 1", len(got))
	}
	if got, _ := db.ListWebhooksForEvent(ctx, userID, "reminder.fired"); len(got) != 0 {
		t.Fatalf("unsubscribed event matched %d endpoints", len(got))
	}

	key := hook.ID.String() + ":reminder.done:" + uuid.NewString()
	row, err := db.CreateWebhookDelivery(ctx, WebhookDelivery{WebhookID: hook.ID, UserID: userID, Event: "reminder.done", Payload: "{}", IdempotencyKey: key})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateWebhookDelivery(ctx, WebhookDelivery{WebhookID: hook.ID, UserID: userID, Event: "reminder.done", IdempotencyKey: key}); err != ErrConflict {
		t.Fatalf("duplicate enqueue err = %v, want ErrConflict", err)
	}

	now := time.Now().UTC()
	claimed, err := db.ClaimWebhookDelivery(ctx, &row.ID, now, time.Minute, 5)
	if err != nil || claimed.AttemptCount != 1 || claimed.Status != WebhookDeliverySending {
		t.Fatalf("claim: %+v %v", claimed, err)
	}
	// Leased: a second worker must not get it until the lease expires.
	if _, err := db.ClaimWebhookDelivery(ctx, &row.ID, now, time.Minute, 5); err != ErrNotFound {
		t.Fatalf("double claim err = %v, want ErrNotFound", err)
	}
	if _, err := db.ClaimWebhookDelivery(ctx, &row.ID, now.Add(2*time.Minute), time.Minute, 5); err != nil {
		t.Fatalf("claim after lease expiry: %v", err)
	}

	retryAt := now.Add(time.Hour)
	if err := db.FinishWebhookDelivery(ctx, row.ID, false, 500, "HTTP 500", &retryAt); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ClaimWebhookDelivery(ctx, &row.ID, now.Add(30*time.Minute), time.Minute, 5); err != ErrNotFound {
		t.Fatalf("claimed before retry time: %v", err)
	}
	if err := db.FinishWebhookDelivery(ctx, row.ID, true, 200, "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ClaimWebhookDelivery(ctx, &row.ID, now.Add(3*time.Hour), time.Minute, 5); err != ErrNotFound {
		t.Fatalf("sent delivery was claimable: %v", err)
	}

	// Enough final failures in a row auto-disable the endpoint.
	for i := 0; i < webhookAutoDisableAfter; i++ {
		if err := db.RecordWebhookAttempt(ctx, hook.ID, false, true, 500, "HTTP 500"); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := db.GetWebhook(ctx, userID, hook.ID)
	if got.Enabled || got.DisabledReason == "" {
		t.Fatalf("endpoint not auto-disabled: %+v", got)
	}
	enabled := true
	got, err = db.UpdateWebhook(ctx, userID, hook.ID, WebhookUpdate{Enabled: &enabled})
	if err != nil || !got.Enabled || got.ConsecutiveFailures != 0 || got.DisabledReason != "" {
		t.Fatalf("re-enable did not reset: %+v %v", got, err)
	}

	if err := db.DeleteWebhook(ctx, userID, hook.ID); err != nil {
		t.Fatal(err)
	}
	if items, _ := db.ListWebhookDeliveries(ctx, userID, hook.ID, 10); len(items) != 0 {
		t.Fatalf("deliveries survived endpoint delete: %d", len(items))
	}
}
