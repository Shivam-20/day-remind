package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/reminder"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Tag struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type TagCreateInput struct {
	Name string
}

type TagUpdateInput struct {
	Name *string
}

type tagDoc struct {
	ID        uuid.UUID `bson:"_id"`
	UserID    uuid.UUID `bson:"user_id"`
	Name      string    `bson:"name"`
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
}

func tagFromDoc(doc tagDoc) Tag {
	return Tag{
		ID:        doc.ID,
		UserID:    doc.UserID,
		Name:      doc.Name,
		CreatedAt: doc.CreatedAt,
		UpdatedAt: doc.UpdatedAt,
	}
}

func (db *DB) CreateTag(ctx context.Context, userID uuid.UUID, in TagCreateInput) (Tag, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return Tag{}, fmt.Errorf("name is required")
	}
	now := time.Now().UTC()
	doc := tagDoc{
		ID:        uuid.New(),
		UserID:    userID,
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err := db.collection("tags").InsertOne(ctx, doc)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return Tag{}, ErrConflict
		}
		return Tag{}, err
	}
	return tagFromDoc(doc), nil
}

func (db *DB) GetTagByID(ctx context.Context, userID, id uuid.UUID) (Tag, error) {
	var doc tagDoc
	err := db.collection("tags").FindOne(ctx, bson.M{"_id": id, "user_id": userID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return Tag{}, ErrNotFound
	}
	if err != nil {
		return Tag{}, err
	}
	return tagFromDoc(doc), nil
}

func (db *DB) ListTags(ctx context.Context, userID uuid.UUID) ([]Tag, error) {
	opts := options.Find().SetSort(bson.D{{Key: "name", Value: 1}})
	cur, err := db.collection("tags").Find(ctx, bson.M{"user_id": userID}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []Tag
	for cur.Next(ctx) {
		var doc tagDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, tagFromDoc(doc))
	}
	return out, cur.Err()
}

func (db *DB) UpdateTag(ctx context.Context, userID, id uuid.UUID, in TagUpdateInput) (Tag, error) {
	cur, err := db.GetTagByID(ctx, userID, id)
	if err != nil {
		return Tag{}, err
	}
	name := cur.Name
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
		if name == "" {
			return Tag{}, fmt.Errorf("name is required")
		}
	}
	set := bson.M{
		"name":       name,
		"updated_at": time.Now().UTC(),
	}
	res, err := db.collection("tags").UpdateOne(ctx, bson.M{"_id": id, "user_id": userID}, bson.M{"$set": set})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return Tag{}, ErrConflict
		}
		return Tag{}, err
	}
	if res.MatchedCount == 0 {
		return Tag{}, ErrNotFound
	}
	return db.GetTagByID(ctx, userID, id)
}

func (db *DB) DeleteTag(ctx context.Context, userID, id uuid.UUID) error {
	res, err := db.collection("tags").DeleteOne(ctx, bson.M{"_id": id, "user_id": userID})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	_, _ = db.collection("reminders").UpdateMany(ctx,
		bson.M{"user_id": userID, "tag_ids": id},
		bson.M{"$pull": bson.M{"tag_ids": id}},
	)
	return nil
}

func (db *DB) SetReminderTags(ctx context.Context, userID, reminderID uuid.UUID, tagIDs []uuid.UUID) (reminder.Reminder, error) {
	if tagIDs == nil {
		tagIDs = []uuid.UUID{}
	}
	for _, tagID := range tagIDs {
		if _, err := db.GetTagByID(ctx, userID, tagID); err != nil {
			return reminder.Reminder{}, err
		}
	}
	res, err := db.collection("reminders").UpdateOne(ctx,
		bson.M{"_id": reminderID, "user_id": userID},
		bson.M{"$set": bson.M{
			"tag_ids":    tagIDs,
			"updated_at": time.Now().UTC(),
		}},
	)
	if err != nil {
		return reminder.Reminder{}, err
	}
	if res.MatchedCount == 0 {
		return reminder.Reminder{}, ErrNotFound
	}
	return db.GetReminder(ctx, userID, reminderID)
}
