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

type Comment struct {
	ID         uuid.UUID
	ReminderID uuid.UUID
	UserID     uuid.UUID
	Body       string
	CreatedAt  time.Time
}

type commentDoc struct {
	ID         uuid.UUID `bson:"_id"`
	ReminderID uuid.UUID `bson:"reminder_id"`
	UserID     uuid.UUID `bson:"user_id"`
	Body       string    `bson:"body"`
	CreatedAt  time.Time `bson:"created_at"`
}

func commentFromDoc(doc commentDoc) Comment {
	return Comment{
		ID:         doc.ID,
		ReminderID: doc.ReminderID,
		UserID:     doc.UserID,
		Body:       doc.Body,
		CreatedAt:  doc.CreatedAt,
	}
}

func (db *DB) ListComments(ctx context.Context, reminderID uuid.UUID) ([]Comment, error) {
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}})
	cur, err := db.collection("reminder_comments").Find(ctx, bson.M{"reminder_id": reminderID}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []Comment
	for cur.Next(ctx) {
		var doc commentDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, commentFromDoc(doc))
	}
	return out, cur.Err()
}

func (db *DB) CreateComment(ctx context.Context, userID, reminderID uuid.UUID, body string) (Comment, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return Comment{}, fmt.Errorf("body is required")
	}
	now := time.Now().UTC()
	doc := commentDoc{
		ID:         uuid.New(),
		ReminderID: reminderID,
		UserID:     userID,
		Body:       body,
		CreatedAt:  now,
	}
	_, err := db.collection("reminder_comments").InsertOne(ctx, doc)
	if err != nil {
		return Comment{}, err
	}
	return commentFromDoc(doc), nil
}
