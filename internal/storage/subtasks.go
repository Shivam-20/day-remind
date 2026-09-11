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

type Subtask struct {
	ID         uuid.UUID
	ReminderID uuid.UUID
	UserID     uuid.UUID
	Title      string
	Done       bool
	SortOrder  int
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type SubtaskCreateInput struct {
	Title     string
	SortOrder int
}

type SubtaskUpdateInput struct {
	Title     *string
	Done      *bool
	SortOrder *int
}

type subtaskDoc struct {
	ID         uuid.UUID `bson:"_id"`
	ReminderID uuid.UUID `bson:"reminder_id"`
	UserID     uuid.UUID `bson:"user_id"`
	Title      string    `bson:"title"`
	Done       bool      `bson:"done"`
	SortOrder  int       `bson:"sort_order"`
	CreatedAt  time.Time `bson:"created_at"`
	UpdatedAt  time.Time `bson:"updated_at"`
}

func subtaskFromDoc(doc subtaskDoc) Subtask {
	return Subtask{
		ID:         doc.ID,
		ReminderID: doc.ReminderID,
		UserID:     doc.UserID,
		Title:      doc.Title,
		Done:       doc.Done,
		SortOrder:  doc.SortOrder,
		CreatedAt:  doc.CreatedAt,
		UpdatedAt:  doc.UpdatedAt,
	}
}

func (db *DB) CreateSubtask(ctx context.Context, userID, reminderID uuid.UUID, in SubtaskCreateInput) (Subtask, error) {
	if _, err := db.GetReminder(ctx, userID, reminderID); err != nil {
		return Subtask{}, err
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return Subtask{}, fmt.Errorf("title is required")
	}
	now := time.Now().UTC()
	doc := subtaskDoc{
		ID:         uuid.New(),
		ReminderID: reminderID,
		UserID:     userID,
		Title:      title,
		SortOrder:  in.SortOrder,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	_, err := db.collection("subtasks").InsertOne(ctx, doc)
	if err != nil {
		return Subtask{}, err
	}
	return subtaskFromDoc(doc), nil
}

func (db *DB) GetSubtaskByID(ctx context.Context, userID, id uuid.UUID) (Subtask, error) {
	var doc subtaskDoc
	err := db.collection("subtasks").FindOne(ctx, bson.M{"_id": id, "user_id": userID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return Subtask{}, ErrNotFound
	}
	if err != nil {
		return Subtask{}, err
	}
	return subtaskFromDoc(doc), nil
}

func (db *DB) ListSubtasks(ctx context.Context, userID, reminderID uuid.UUID) ([]Subtask, error) {
	opts := options.Find().SetSort(bson.D{{Key: "sort_order", Value: 1}, {Key: "created_at", Value: 1}})
	cur, err := db.collection("subtasks").Find(ctx, bson.M{
		"user_id":     userID,
		"reminder_id": reminderID,
	}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []Subtask
	for cur.Next(ctx) {
		var doc subtaskDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, subtaskFromDoc(doc))
	}
	return out, cur.Err()
}

func (db *DB) UpdateSubtask(ctx context.Context, userID, id uuid.UUID, in SubtaskUpdateInput) (Subtask, error) {
	cur, err := db.GetSubtaskByID(ctx, userID, id)
	if err != nil {
		return Subtask{}, err
	}
	title := cur.Title
	if in.Title != nil {
		title = strings.TrimSpace(*in.Title)
		if title == "" {
			return Subtask{}, fmt.Errorf("title is required")
		}
	}
	done := cur.Done
	if in.Done != nil {
		done = *in.Done
	}
	sortOrder := cur.SortOrder
	if in.SortOrder != nil {
		sortOrder = *in.SortOrder
	}
	set := bson.M{
		"title":      title,
		"done":       done,
		"sort_order": sortOrder,
		"updated_at": time.Now().UTC(),
	}
	res, err := db.collection("subtasks").UpdateOne(ctx, bson.M{"_id": id, "user_id": userID}, bson.M{"$set": set})
	if err != nil {
		return Subtask{}, err
	}
	if res.MatchedCount == 0 {
		return Subtask{}, ErrNotFound
	}
	return db.GetSubtaskByID(ctx, userID, id)
}

func (db *DB) DeleteSubtask(ctx context.Context, userID, id uuid.UUID) error {
	res, err := db.collection("subtasks").DeleteOne(ctx, bson.M{"_id": id, "user_id": userID})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (db *DB) ReorderSubtasks(ctx context.Context, userID, reminderID uuid.UUID, orderedIDs []uuid.UUID) error {
	if _, err := db.GetReminder(ctx, userID, reminderID); err != nil {
		return err
	}
	now := time.Now().UTC()
	for i, id := range orderedIDs {
		res, err := db.collection("subtasks").UpdateOne(ctx, bson.M{
			"_id":         id,
			"user_id":     userID,
			"reminder_id": reminderID,
		}, bson.M{"$set": bson.M{
			"sort_order": i,
			"updated_at": now,
		}})
		if err != nil {
			return err
		}
		if res.MatchedCount == 0 {
			return ErrNotFound
		}
	}
	return nil
}
