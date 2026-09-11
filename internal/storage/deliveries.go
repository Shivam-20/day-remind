package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	DeliveryStatusPending = "pending"
	DeliveryStatusSent    = "sent"
	DeliveryStatusFailed  = "failed"
	DeliveryStatusSkipped = "skipped"
)

const maxDeliveryAttempts = 3

type Delivery struct {
	ID             uuid.UUID
	ReminderID     uuid.UUID
	UserID         uuid.UUID
	Channel        string
	Status         string
	AttemptCount   int
	LastError      string
	SentAt         *time.Time
	IdempotencyKey string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type deliveryDoc struct {
	ID             uuid.UUID  `bson:"_id"`
	ReminderID     uuid.UUID  `bson:"reminder_id"`
	UserID         uuid.UUID  `bson:"user_id"`
	Channel        string     `bson:"channel"`
	Status         string     `bson:"status"`
	AttemptCount   int        `bson:"attempt_count"`
	LastError      string     `bson:"last_error"`
	SentAt         *time.Time `bson:"sent_at,omitempty"`
	IdempotencyKey string     `bson:"idempotency_key"`
	CreatedAt      time.Time  `bson:"created_at"`
	UpdatedAt      time.Time  `bson:"updated_at"`
}

func deliveryRecordFromDoc(doc deliveryDoc) Delivery {
	return Delivery{
		ID:             doc.ID,
		ReminderID:     doc.ReminderID,
		UserID:         doc.UserID,
		Channel:        doc.Channel,
		Status:         doc.Status,
		AttemptCount:   doc.AttemptCount,
		LastError:      doc.LastError,
		SentAt:         doc.SentAt,
		IdempotencyKey: doc.IdempotencyKey,
		CreatedAt:      doc.CreatedAt,
		UpdatedAt:      doc.UpdatedAt,
	}
}

func (db *DB) CreateDelivery(ctx context.Context, d Delivery) (Delivery, error) {
	now := time.Now().UTC()
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	doc := deliveryDoc{
		ID:             d.ID,
		ReminderID:     d.ReminderID,
		UserID:         d.UserID,
		Channel:        d.Channel,
		Status:         d.Status,
		AttemptCount:   d.AttemptCount,
		LastError:      d.LastError,
		SentAt:         d.SentAt,
		IdempotencyKey: d.IdempotencyKey,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	_, err := db.collection("reminder_deliveries").InsertOne(ctx, doc)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return Delivery{}, ErrConflict
		}
		return Delivery{}, err
	}
	return deliveryRecordFromDoc(doc), nil
}

func (db *DB) MarkDeliverySent(ctx context.Context, id uuid.UUID) error {
	now := time.Now().UTC()
	res, err := db.collection("reminder_deliveries").UpdateOne(ctx, bson.M{"_id": id}, bson.M{
		"$set": bson.M{
			"status":     DeliveryStatusSent,
			"sent_at":    now,
			"updated_at": now,
		},
	})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (db *DB) MarkDeliveryFailed(ctx context.Context, id uuid.UUID, errMsg string) error {
	now := time.Now().UTC()
	res, err := db.collection("reminder_deliveries").UpdateOne(ctx, bson.M{"_id": id}, bson.M{
		"$set": bson.M{
			"status":        DeliveryStatusFailed,
			"last_error":    errMsg,
			"updated_at":    now,
		},
		"$inc": bson.M{"attempt_count": 1},
	})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (db *DB) MarkDeliverySkipped(ctx context.Context, id uuid.UUID, reason string) error {
	now := time.Now().UTC()
	_, err := db.collection("reminder_deliveries").UpdateOne(ctx, bson.M{"_id": id}, bson.M{
		"$set": bson.M{
			"status":     DeliveryStatusSkipped,
			"last_error": reason,
			"updated_at": now,
		},
	})
	return err
}

func (db *DB) ListDeliveriesForReminder(ctx context.Context, userID, reminderID uuid.UUID, limit int) ([]Delivery, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	cur, err := db.collection("reminder_deliveries").Find(ctx, bson.M{
		"reminder_id": reminderID,
		"user_id":     userID,
	}, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	return decodeDeliveries(cur, ctx)
}

func (db *DB) ListRecentDeliveriesForUser(ctx context.Context, userID uuid.UUID, limit int) ([]Delivery, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	cur, err := db.collection("reminder_deliveries").Find(ctx, bson.M{"user_id": userID},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	return decodeDeliveries(cur, ctx)
}

func (db *DB) ListRetryableDeliveries(ctx context.Context, limit int) ([]Delivery, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	cutoff := time.Now().UTC().Add(-2 * time.Minute)
	cur, err := db.collection("reminder_deliveries").Find(ctx, bson.M{
		"status":        DeliveryStatusFailed,
		"attempt_count": bson.M{"$lt": maxDeliveryAttempts},
		"updated_at":    bson.M{"$lte": cutoff},
	}, options.Find().SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	return decodeDeliveries(cur, ctx)
}

func decodeDeliveries(cur *mongo.Cursor, ctx context.Context) ([]Delivery, error) {
	var out []Delivery
	for cur.Next(ctx) {
		var doc deliveryDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, deliveryRecordFromDoc(doc))
	}
	return out, cur.Err()
}

func DeliveryIdempotencyKey(reminderID uuid.UUID, channel string, dueAt time.Time) string {
	return fmt.Sprintf("%s:%s:%d", reminderID.String(), channel, dueAt.UTC().Unix())
}
