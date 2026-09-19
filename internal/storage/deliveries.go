package storage

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Delivery is one attempt to send one notification over one channel. The log is
// append-only and exists so failures are visible rather than silent.
type Delivery struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	ReminderID uuid.UUID
	Channel    string
	Kind       string
	Status     string
	Detail     string
	CreatedAt  time.Time
}

const (
	DeliverySent   = "sent"
	DeliveryFailed = "failed"
)

type deliveryDoc struct {
	ID         uuid.UUID `bson:"_id"`
	UserID     uuid.UUID `bson:"user_id"`
	ReminderID uuid.UUID `bson:"reminder_id"`
	Channel    string    `bson:"channel"`
	Kind       string    `bson:"kind"`
	Status     string    `bson:"status"`
	Detail     string    `bson:"detail,omitempty"`
	CreatedAt  time.Time `bson:"created_at"`
}

// LogDelivery records an attempt. It never returns an error to the caller's
// critical path decision — sending already happened or already failed — but the
// error is surfaced so the scheduler can log it.
func (db *DB) LogDelivery(ctx context.Context, d Delivery) error {
	// Keep the stored detail bounded; SMTP and API errors can be verbose.
	const maxDetail = 500
	if len(d.Detail) > maxDetail {
		d.Detail = d.Detail[:maxDetail]
	}
	_, err := db.collection("deliveries").InsertOne(ctx, deliveryDoc{
		ID:         uuid.New(),
		UserID:     d.UserID,
		ReminderID: d.ReminderID,
		Channel:    d.Channel,
		Kind:       d.Kind,
		Status:     d.Status,
		Detail:     d.Detail,
		CreatedAt:  time.Now().UTC(),
	})
	return err
}

// RecentDeliveries powers the delivery log in the admin panel.
func (db *DB) RecentDeliveries(ctx context.Context, limit int64) ([]Delivery, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	cur, err := db.collection("deliveries").Find(ctx, bson.M{},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []Delivery
	for cur.Next(ctx) {
		var doc deliveryDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, Delivery{
			ID:         doc.ID,
			UserID:     doc.UserID,
			ReminderID: doc.ReminderID,
			Channel:    doc.Channel,
			Kind:       doc.Kind,
			Status:     doc.Status,
			Detail:     doc.Detail,
			CreatedAt:  doc.CreatedAt,
		})
	}
	return out, cur.Err()
}

// CountDeliveries reports sent/failed totals for the admin stats row.
func (db *DB) CountDeliveries(ctx context.Context, status string) (int64, error) {
	return db.collection("deliveries").CountDocuments(ctx, bson.M{"status": status})
}
