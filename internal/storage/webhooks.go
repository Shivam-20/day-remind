package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MaxWebhooksPerUser caps outbound endpoints per account.
const MaxWebhooksPerUser = 10

// webhookAutoDisableAfter disables an endpoint after this many deliveries in
// a row exhausted all their retries.
const webhookAutoDisableAfter = 5

const (
	WebhookDeliveryPending = "pending"
	WebhookDeliverySending = "sending"
	WebhookDeliveryRetry   = "retry"
	WebhookDeliverySent    = "sent"
	WebhookDeliveryFailed  = "failed"
)

type Webhook struct {
	ID                  uuid.UUID
	UserID              uuid.UUID
	URL                 string
	Secret              string
	Events              []string
	Enabled             bool
	DisabledReason      string
	ConsecutiveFailures int
	LastStatusCode      int
	LastError           string
	LastDeliveryAt      *time.Time
	CreatedAt           time.Time
}

type webhookDoc struct {
	ID                  uuid.UUID  `bson:"_id"`
	UserID              uuid.UUID  `bson:"user_id"`
	URL                 string     `bson:"url"`
	Secret              string     `bson:"secret"`
	Events              []string   `bson:"events"`
	Enabled             bool       `bson:"enabled"`
	DisabledReason      string     `bson:"disabled_reason,omitempty"`
	ConsecutiveFailures int        `bson:"consecutive_failures"`
	LastStatusCode      int        `bson:"last_status_code,omitempty"`
	LastError           string     `bson:"last_error,omitempty"`
	LastDeliveryAt      *time.Time `bson:"last_delivery_at,omitempty"`
	CreatedAt           time.Time  `bson:"created_at"`
	UpdatedAt           time.Time  `bson:"updated_at"`
}

type WebhookUpdate struct {
	URL     *string
	Secret  *string
	Events  []string
	Enabled *bool
}

func webhookFromDoc(doc webhookDoc) Webhook {
	return Webhook{
		ID:                  doc.ID,
		UserID:              doc.UserID,
		URL:                 doc.URL,
		Secret:              doc.Secret,
		Events:              doc.Events,
		Enabled:             doc.Enabled,
		DisabledReason:      doc.DisabledReason,
		ConsecutiveFailures: doc.ConsecutiveFailures,
		LastStatusCode:      doc.LastStatusCode,
		LastError:           doc.LastError,
		LastDeliveryAt:      doc.LastDeliveryAt,
		CreatedAt:           doc.CreatedAt,
	}
}

func (db *DB) CreateWebhook(ctx context.Context, userID uuid.UUID, url, secret string, events []string) (Webhook, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return Webhook{}, fmt.Errorf("url is required")
	}
	if len(events) == 0 {
		return Webhook{}, fmt.Errorf("at least one event is required")
	}
	n, err := db.collection("webhooks").CountDocuments(ctx, bson.M{"user_id": userID})
	if err != nil {
		return Webhook{}, err
	}
	if n >= MaxWebhooksPerUser {
		return Webhook{}, fmt.Errorf("webhook limit reached (%d)", MaxWebhooksPerUser)
	}
	now := time.Now().UTC()
	doc := webhookDoc{
		ID:        uuid.New(),
		UserID:    userID,
		URL:       url,
		Secret:    secret,
		Events:    events,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := db.collection("webhooks").InsertOne(ctx, doc); err != nil {
		return Webhook{}, err
	}
	return webhookFromDoc(doc), nil
}

func (db *DB) GetWebhook(ctx context.Context, userID, id uuid.UUID) (Webhook, error) {
	var doc webhookDoc
	err := db.collection("webhooks").FindOne(ctx, bson.M{"_id": id, "user_id": userID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return Webhook{}, ErrNotFound
	}
	if err != nil {
		return Webhook{}, err
	}
	return webhookFromDoc(doc), nil
}

func (db *DB) ListWebhooks(ctx context.Context, userID uuid.UUID) ([]Webhook, error) {
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
	return db.findWebhooks(ctx, bson.M{"user_id": userID}, opts)
}

// ListWebhooksForEvent returns the user's enabled endpoints subscribed to event.
func (db *DB) ListWebhooksForEvent(ctx context.Context, userID uuid.UUID, event string) ([]Webhook, error) {
	return db.findWebhooks(ctx, bson.M{"user_id": userID, "enabled": true, "events": event})
}

func (db *DB) findWebhooks(ctx context.Context, filter bson.M, opts ...*options.FindOptions) ([]Webhook, error) {
	cur, err := db.collection("webhooks").Find(ctx, filter, opts...)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []Webhook
	for cur.Next(ctx) {
		var doc webhookDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, webhookFromDoc(doc))
	}
	return out, cur.Err()
}

func (db *DB) UpdateWebhook(ctx context.Context, userID, id uuid.UUID, in WebhookUpdate) (Webhook, error) {
	cur, err := db.GetWebhook(ctx, userID, id)
	if err != nil {
		return Webhook{}, err
	}
	if in.URL != nil {
		url := strings.TrimSpace(*in.URL)
		if url == "" {
			return Webhook{}, fmt.Errorf("url is required")
		}
		cur.URL = url
	}
	if in.Secret != nil {
		cur.Secret = *in.Secret
	}
	if in.Events != nil {
		if len(in.Events) == 0 {
			return Webhook{}, fmt.Errorf("at least one event is required")
		}
		cur.Events = in.Events
	}
	set := bson.M{
		"url":        cur.URL,
		"secret":     cur.Secret,
		"events":     cur.Events,
		"updated_at": time.Now().UTC(),
	}
	if in.Enabled != nil {
		cur.Enabled = *in.Enabled
		set["enabled"] = cur.Enabled
		if cur.Enabled {
			// Re-enabling gives an auto-disabled endpoint a clean slate.
			cur.ConsecutiveFailures = 0
			cur.DisabledReason = ""
			set["consecutive_failures"] = 0
			set["disabled_reason"] = ""
		}
	}
	res, err := db.collection("webhooks").UpdateOne(ctx, bson.M{"_id": id, "user_id": userID}, bson.M{"$set": set})
	if err != nil {
		return Webhook{}, err
	}
	if res.MatchedCount == 0 {
		return Webhook{}, ErrNotFound
	}
	return cur, nil
}

func (db *DB) DeleteWebhook(ctx context.Context, userID, id uuid.UUID) error {
	res, err := db.collection("webhooks").DeleteOne(ctx, bson.M{"_id": id, "user_id": userID})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	_, _ = db.collection("webhook_deliveries").DeleteMany(ctx, bson.M{"webhook_id": id})
	return nil
}

// RecordWebhookAttempt stores the outcome of one HTTP attempt on the endpoint.
// final marks a delivery that has exhausted its retries; enough of those in a
// row auto-disable the endpoint. A success resets the streak.
func (db *DB) RecordWebhookAttempt(ctx context.Context, id uuid.UUID, ok, final bool, statusCode int, errMsg string) error {
	now := time.Now().UTC()
	set := bson.M{"last_status_code": statusCode, "last_error": errMsg, "last_delivery_at": now}
	update := bson.M{"$set": set}
	if ok {
		set["consecutive_failures"] = 0
	} else if final {
		update["$inc"] = bson.M{"consecutive_failures": 1}
	}
	if _, err := db.collection("webhooks").UpdateOne(ctx, bson.M{"_id": id}, update); err != nil {
		return err
	}
	if !ok && final {
		_, err := db.collection("webhooks").UpdateOne(ctx, bson.M{
			"_id":                  id,
			"enabled":              true,
			"consecutive_failures": bson.M{"$gte": webhookAutoDisableAfter},
		}, bson.M{"$set": bson.M{
			"enabled":         false,
			"disabled_reason": fmt.Sprintf("disabled after %d failed deliveries in a row", webhookAutoDisableAfter),
			"updated_at":      now,
		}})
		return err
	}
	return nil
}

// WebhookDelivery is one queued event for one endpoint (outbox row).
type WebhookDelivery struct {
	ID             uuid.UUID
	WebhookID      uuid.UUID
	UserID         uuid.UUID
	Event          string
	Payload        string
	Status         string
	AttemptCount   int
	ResponseCode   int
	LastError      string
	NextAttemptAt  time.Time
	IdempotencyKey string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type webhookDeliveryDoc struct {
	ID             uuid.UUID `bson:"_id"`
	WebhookID      uuid.UUID `bson:"webhook_id"`
	UserID         uuid.UUID `bson:"user_id"`
	Event          string    `bson:"event"`
	Payload        string    `bson:"payload"`
	Status         string    `bson:"status"`
	AttemptCount   int       `bson:"attempt_count"`
	ResponseCode   int       `bson:"response_code,omitempty"`
	LastError      string    `bson:"last_error,omitempty"`
	NextAttemptAt  time.Time `bson:"next_attempt_at"`
	IdempotencyKey string    `bson:"idempotency_key"`
	CreatedAt      time.Time `bson:"created_at"`
	UpdatedAt      time.Time `bson:"updated_at"`
}

func webhookDeliveryFromDoc(doc webhookDeliveryDoc) WebhookDelivery {
	return WebhookDelivery(doc)
}

// CreateWebhookDelivery enqueues a delivery. A duplicate idempotency key
// (same event already queued for this endpoint) returns ErrConflict.
func (db *DB) CreateWebhookDelivery(ctx context.Context, d WebhookDelivery) (WebhookDelivery, error) {
	now := time.Now().UTC()
	doc := webhookDeliveryDoc{
		ID:             uuid.New(),
		WebhookID:      d.WebhookID,
		UserID:         d.UserID,
		Event:          d.Event,
		Payload:        d.Payload,
		Status:         WebhookDeliveryPending,
		NextAttemptAt:  now,
		IdempotencyKey: d.IdempotencyKey,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if _, err := db.collection("webhook_deliveries").InsertOne(ctx, doc); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return WebhookDelivery{}, ErrConflict
		}
		return WebhookDelivery{}, err
	}
	return webhookDeliveryFromDoc(doc), nil
}

// ClaimWebhookDelivery atomically leases one due delivery for sending. With a
// non-nil id it claims only that row. The lease (next_attempt_at pushed into
// the future) keeps concurrent workers off it, and lets a crashed attempt be
// picked up again once the lease expires. Returns ErrNotFound when nothing is due.
func (db *DB) ClaimWebhookDelivery(ctx context.Context, id *uuid.UUID, now time.Time, lease time.Duration, maxAttempts int) (WebhookDelivery, error) {
	filter := bson.M{
		"status":          bson.M{"$in": bson.A{WebhookDeliveryPending, WebhookDeliveryRetry, WebhookDeliverySending}},
		"next_attempt_at": bson.M{"$lte": now.UTC()},
		"attempt_count":   bson.M{"$lt": maxAttempts},
	}
	if id != nil {
		filter["_id"] = *id
	}
	opts := options.FindOneAndUpdate().
		SetSort(bson.D{{Key: "next_attempt_at", Value: 1}}).
		SetReturnDocument(options.After)
	var doc webhookDeliveryDoc
	err := db.collection("webhook_deliveries").FindOneAndUpdate(ctx, filter, bson.M{
		"$set": bson.M{
			"status":          WebhookDeliverySending,
			"next_attempt_at": now.UTC().Add(lease),
			"updated_at":      now.UTC(),
		},
		"$inc": bson.M{"attempt_count": 1},
	}, opts).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return WebhookDelivery{}, ErrNotFound
	}
	if err != nil {
		return WebhookDelivery{}, err
	}
	return webhookDeliveryFromDoc(doc), nil
}

// FinishWebhookDelivery records an attempt. retryAt nil means no further
// attempts: status becomes sent (ok) or failed.
func (db *DB) FinishWebhookDelivery(ctx context.Context, id uuid.UUID, ok bool, statusCode int, errMsg string, retryAt *time.Time) error {
	now := time.Now().UTC()
	set := bson.M{"response_code": statusCode, "last_error": errMsg, "updated_at": now}
	switch {
	case ok:
		set["status"] = WebhookDeliverySent
	case retryAt != nil:
		set["status"] = WebhookDeliveryRetry
		set["next_attempt_at"] = retryAt.UTC()
	default:
		set["status"] = WebhookDeliveryFailed
	}
	_, err := db.collection("webhook_deliveries").UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": set})
	return err
}

// FailWebhookDelivery closes a row without sending (endpoint disabled or
// deleted), so it stops being claimed.
func (db *DB) FailWebhookDelivery(ctx context.Context, id uuid.UUID, reason string) error {
	return db.FinishWebhookDelivery(ctx, id, false, 0, reason, nil)
}

// GetWebhookByID loads an endpoint without user scoping (dispatcher use).
func (db *DB) GetWebhookByID(ctx context.Context, id uuid.UUID) (Webhook, error) {
	var doc webhookDoc
	err := db.collection("webhooks").FindOne(ctx, bson.M{"_id": id}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return Webhook{}, ErrNotFound
	}
	if err != nil {
		return Webhook{}, err
	}
	return webhookFromDoc(doc), nil
}

func (db *DB) ListWebhookDeliveries(ctx context.Context, userID, webhookID uuid.UUID, limit int) ([]WebhookDelivery, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(int64(limit))
	cur, err := db.collection("webhook_deliveries").Find(ctx, bson.M{"user_id": userID, "webhook_id": webhookID}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []WebhookDelivery
	for cur.Next(ctx) {
		var doc webhookDeliveryDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, webhookDeliveryFromDoc(doc))
	}
	return out, cur.Err()
}
