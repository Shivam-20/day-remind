package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type PushSubscription struct {
	UserID   uuid.UUID
	Endpoint string
	Keys     string
}

type pushSubscriptionDoc struct {
	Endpoint  string    `bson:"_id"`
	UserID    uuid.UUID `bson:"user_id"`
	Keys      string    `bson:"keys"`
	UpdatedAt time.Time `bson:"updated_at"`
}

func pushFromDoc(doc pushSubscriptionDoc) PushSubscription {
	return PushSubscription{
		UserID:   doc.UserID,
		Endpoint: doc.Endpoint,
		Keys:     doc.Keys,
	}
}

func (db *DB) SavePushSubscription(ctx context.Context, userID uuid.UUID, endpoint, keys string) (PushSubscription, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return PushSubscription{}, fmt.Errorf("endpoint is required")
	}
	if strings.TrimSpace(keys) == "" {
		return PushSubscription{}, fmt.Errorf("keys are required")
	}
	now := time.Now().UTC()
	doc := pushSubscriptionDoc{
		Endpoint:  endpoint,
		UserID:    userID,
		Keys:      keys,
		UpdatedAt: now,
	}
	opts := options.Update().SetUpsert(true)
	_, err := db.collection("push_subscriptions").UpdateOne(ctx, bson.M{"_id": endpoint},
		bson.M{"$set": doc}, opts)
	if err != nil {
		return PushSubscription{}, err
	}
	return pushFromDoc(doc), nil
}

func (db *DB) ListPushSubscriptions(ctx context.Context, userID uuid.UUID) ([]PushSubscription, error) {
	cur, err := db.collection("push_subscriptions").Find(ctx, bson.M{"user_id": userID})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []PushSubscription
	for cur.Next(ctx) {
		var doc pushSubscriptionDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, pushFromDoc(doc))
	}
	return out, cur.Err()
}
