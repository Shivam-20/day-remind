package storage

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	OccurrenceKindMain   = "main"
	OccurrenceKindOffset = "offset"
)

type ReminderOccurrence struct {
	ID            uuid.UUID
	ReminderID    uuid.UUID
	UserID        uuid.UUID
	FireAt        time.Time
	Kind          string
	OffsetMinutes int
	Fired         bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type OccurrenceCreateInput struct {
	ReminderID    uuid.UUID
	FireAt        time.Time
	Kind          string
	OffsetMinutes int
}

type occurrenceDoc struct {
	ID            uuid.UUID `bson:"_id"`
	ReminderID    uuid.UUID `bson:"reminder_id"`
	UserID        uuid.UUID `bson:"user_id"`
	FireAt        time.Time `bson:"fire_at"`
	Kind          string    `bson:"kind"`
	OffsetMinutes int       `bson:"offset_minutes"`
	Fired         bool      `bson:"fired"`
	CreatedAt     time.Time `bson:"created_at"`
	UpdatedAt     time.Time `bson:"updated_at"`
}

func occurrenceFromDoc(doc occurrenceDoc) ReminderOccurrence {
	return ReminderOccurrence{
		ID:            doc.ID,
		ReminderID:    doc.ReminderID,
		UserID:        doc.UserID,
		FireAt:        doc.FireAt,
		Kind:          doc.Kind,
		OffsetMinutes: doc.OffsetMinutes,
		Fired:         doc.Fired,
		CreatedAt:     doc.CreatedAt,
		UpdatedAt:     doc.UpdatedAt,
	}
}

func (db *DB) CreateOccurrence(ctx context.Context, userID uuid.UUID, in OccurrenceCreateInput) (ReminderOccurrence, error) {
	if _, err := db.GetReminder(ctx, userID, in.ReminderID); err != nil {
		return ReminderOccurrence{}, err
	}
	now := time.Now().UTC()
	doc := occurrenceDoc{
		ID:            uuid.New(),
		ReminderID:    in.ReminderID,
		UserID:        userID,
		FireAt:        in.FireAt.UTC(),
		Kind:          in.Kind,
		OffsetMinutes: in.OffsetMinutes,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	_, err := db.collection("reminder_occurrences").InsertOne(ctx, doc)
	if err != nil {
		return ReminderOccurrence{}, err
	}
	return occurrenceFromDoc(doc), nil
}

func (db *DB) ListDueOccurrences(ctx context.Context, before time.Time, limit int) ([]ReminderOccurrence, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	filter := bson.M{
		"fired":   false,
		"fire_at": bson.M{"$lte": before.UTC()},
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "fire_at", Value: 1}}).
		SetLimit(int64(limit))
	cur, err := db.collection("reminder_occurrences").Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []ReminderOccurrence
	for cur.Next(ctx) {
		var doc occurrenceDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, occurrenceFromDoc(doc))
	}
	return out, cur.Err()
}

func (db *DB) MarkOccurrenceFired(ctx context.Context, id uuid.UUID) error {
	now := time.Now().UTC()
	res, err := db.collection("reminder_occurrences").UpdateOne(ctx, bson.M{"_id": id}, bson.M{
		"$set": bson.M{
			"fired":      true,
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

func (db *DB) ListOccurrencesForReminder(ctx context.Context, userID, reminderID uuid.UUID) ([]ReminderOccurrence, error) {
	opts := options.Find().SetSort(bson.D{{Key: "fire_at", Value: 1}})
	cur, err := db.collection("reminder_occurrences").Find(ctx, bson.M{
		"user_id":     userID,
		"reminder_id": reminderID,
	}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []ReminderOccurrence
	for cur.Next(ctx) {
		var doc occurrenceDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, occurrenceFromDoc(doc))
	}
	return out, cur.Err()
}
