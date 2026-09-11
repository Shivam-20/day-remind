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

type Webhook struct {
	ID      uuid.UUID
	UserID  uuid.UUID
	URL     string
	Secret  string
	Events  []string
	Enabled bool
}

type webhookDoc struct {
	ID        uuid.UUID `bson:"_id"`
	UserID    uuid.UUID `bson:"user_id"`
	URL       string    `bson:"url"`
	Secret    string    `bson:"secret"`
	Events    []string  `bson:"events"`
	Enabled   bool      `bson:"enabled"`
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
}

type WebhookUpdate struct {
	URL     *string
	Secret  *string
	Events  []string
	Enabled *bool
}

func webhookFromDoc(doc webhookDoc) Webhook {
	return Webhook{
		ID:      doc.ID,
		UserID:  doc.UserID,
		URL:     doc.URL,
		Secret:  doc.Secret,
		Events:  doc.Events,
		Enabled: doc.Enabled,
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
	_, err := db.collection("webhooks").InsertOne(ctx, doc)
	if err != nil {
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
	cur, err := db.collection("webhooks").Find(ctx, bson.M{"user_id": userID}, opts)
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
	if in.Enabled != nil {
		cur.Enabled = *in.Enabled
	}
	res, err := db.collection("webhooks").UpdateOne(ctx, bson.M{"_id": id, "user_id": userID}, bson.M{
		"$set": bson.M{
			"url":        cur.URL,
			"secret":     cur.Secret,
			"events":     cur.Events,
			"enabled":    cur.Enabled,
			"updated_at": time.Now().UTC(),
		},
	})
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
	return nil
}

func (db *DB) ListWebhooksForEvent(ctx context.Context, event string) ([]Webhook, error) {
	cur, err := db.collection("webhooks").Find(ctx, bson.M{
		"enabled": true,
		"events":  event,
	})
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
