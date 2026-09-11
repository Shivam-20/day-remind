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

type ReminderOffset struct {
	ID              uuid.UUID
	ReminderID      uuid.UUID
	UserID          uuid.UUID
	OffsetMinutes   int
	ChannelOverride string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type OffsetCreateInput struct {
	OffsetMinutes   int
	ChannelOverride string
}

type OffsetUpdateInput struct {
	OffsetMinutes   *int
	ChannelOverride *string
}

type offsetDoc struct {
	ID              uuid.UUID `bson:"_id"`
	ReminderID      uuid.UUID `bson:"reminder_id"`
	UserID          uuid.UUID `bson:"user_id"`
	OffsetMinutes   int       `bson:"offset_minutes"`
	ChannelOverride string    `bson:"channel_override,omitempty"`
	CreatedAt       time.Time `bson:"created_at"`
	UpdatedAt       time.Time `bson:"updated_at"`
}

func offsetFromDoc(doc offsetDoc) ReminderOffset {
	return ReminderOffset{
		ID:              doc.ID,
		ReminderID:      doc.ReminderID,
		UserID:          doc.UserID,
		OffsetMinutes:   doc.OffsetMinutes,
		ChannelOverride: doc.ChannelOverride,
		CreatedAt:       doc.CreatedAt,
		UpdatedAt:       doc.UpdatedAt,
	}
}

func (db *DB) CreateOffset(ctx context.Context, userID, reminderID uuid.UUID, in OffsetCreateInput) (ReminderOffset, error) {
	if _, err := db.GetReminder(ctx, userID, reminderID); err != nil {
		return ReminderOffset{}, err
	}
	if in.OffsetMinutes < 0 {
		return ReminderOffset{}, fmt.Errorf("offset_minutes must be non-negative")
	}
	now := time.Now().UTC()
	doc := offsetDoc{
		ID:              uuid.New(),
		ReminderID:      reminderID,
		UserID:          userID,
		OffsetMinutes:   in.OffsetMinutes,
		ChannelOverride: in.ChannelOverride,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	_, err := db.collection("reminder_offsets").InsertOne(ctx, doc)
	if err != nil {
		return ReminderOffset{}, err
	}
	return offsetFromDoc(doc), nil
}

func (db *DB) GetOffsetByID(ctx context.Context, userID, id uuid.UUID) (ReminderOffset, error) {
	var doc offsetDoc
	err := db.collection("reminder_offsets").FindOne(ctx, bson.M{"_id": id, "user_id": userID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return ReminderOffset{}, ErrNotFound
	}
	if err != nil {
		return ReminderOffset{}, err
	}
	return offsetFromDoc(doc), nil
}

func (db *DB) ListOffsetsForReminder(ctx context.Context, userID, reminderID uuid.UUID) ([]ReminderOffset, error) {
	opts := options.Find().SetSort(bson.D{{Key: "offset_minutes", Value: -1}})
	cur, err := db.collection("reminder_offsets").Find(ctx, bson.M{
		"user_id":     userID,
		"reminder_id": reminderID,
	}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []ReminderOffset
	for cur.Next(ctx) {
		var doc offsetDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, offsetFromDoc(doc))
	}
	return out, cur.Err()
}

func (db *DB) UpdateOffset(ctx context.Context, userID, id uuid.UUID, in OffsetUpdateInput) (ReminderOffset, error) {
	cur, err := db.GetOffsetByID(ctx, userID, id)
	if err != nil {
		return ReminderOffset{}, err
	}
	offsetMinutes := cur.OffsetMinutes
	if in.OffsetMinutes != nil {
		if *in.OffsetMinutes < 0 {
			return ReminderOffset{}, fmt.Errorf("offset_minutes must be non-negative")
		}
		offsetMinutes = *in.OffsetMinutes
	}
	channelOverride := cur.ChannelOverride
	if in.ChannelOverride != nil {
		channelOverride = *in.ChannelOverride
	}
	set := bson.M{
		"offset_minutes":   offsetMinutes,
		"channel_override": channelOverride,
		"updated_at":       time.Now().UTC(),
	}
	res, err := db.collection("reminder_offsets").UpdateOne(ctx, bson.M{"_id": id, "user_id": userID}, bson.M{"$set": set})
	if err != nil {
		return ReminderOffset{}, err
	}
	if res.MatchedCount == 0 {
		return ReminderOffset{}, ErrNotFound
	}
	return db.GetOffsetByID(ctx, userID, id)
}

func (db *DB) DeleteOffset(ctx context.Context, userID, id uuid.UUID) error {
	res, err := db.collection("reminder_offsets").DeleteOne(ctx, bson.M{"_id": id, "user_id": userID})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}
