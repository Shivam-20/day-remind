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

type Template struct {
	ID          uuid.UUID
	UserID      *uuid.UUID
	Name        string
	Title       string
	Description string
	Priority    string
	Offsets     []int
	IsSystem    bool
}

type templateDoc struct {
	ID          uuid.UUID  `bson:"_id"`
	UserID      *uuid.UUID `bson:"user_id,omitempty"`
	Name        string     `bson:"name"`
	Title       string     `bson:"title"`
	Description string     `bson:"description"`
	Priority    string     `bson:"priority"`
	Offsets     []int      `bson:"offsets"`
	IsSystem    bool       `bson:"is_system"`
	CreatedAt   time.Time  `bson:"created_at"`
	UpdatedAt   time.Time  `bson:"updated_at"`
}

func templateFromDoc(doc templateDoc) Template {
	return Template{
		ID:          doc.ID,
		UserID:      doc.UserID,
		Name:        doc.Name,
		Title:       doc.Title,
		Description: doc.Description,
		Priority:    doc.Priority,
		Offsets:     doc.Offsets,
		IsSystem:    doc.IsSystem,
	}
}

var systemTemplates = []templateDoc{
	{
		Name:        "Birthday",
		Title:       "Birthday: {{name}}",
		Description: "Don't forget to wish them a happy birthday!",
		Priority:    reminder.PriorityNormal,
		Offsets:     []int{10080, 1440, 60},
		IsSystem:    true,
	},
	{
		Name:        "Bill payment",
		Title:       "Pay {{bill}}",
		Description: "Bill payment due",
		Priority:    reminder.PriorityHigh,
		Offsets:     []int{4320, 1440, 0},
		IsSystem:    true,
	},
	{
		Name:        "Medicine",
		Title:       "Take medicine",
		Description: "Time for your medication",
		Priority:    reminder.PriorityHigh,
		Offsets:     []int{0},
		IsSystem:    true,
	},
	{
		Name:        "Meeting follow-up",
		Title:       "Follow up: {{meeting}}",
		Description: "Send follow-up notes from the meeting",
		Priority:    reminder.PriorityNormal,
		Offsets:     []int{1440, 60},
		IsSystem:    true,
	},
}

func (db *DB) seedSystemTemplates(ctx context.Context) error {
	count, err := db.collection("templates").CountDocuments(ctx, bson.M{"is_system": true})
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	now := time.Now().UTC()
	docs := make([]interface{}, 0, len(systemTemplates))
	for _, t := range systemTemplates {
		t.ID = uuid.New()
		t.CreatedAt = now
		t.UpdatedAt = now
		docs = append(docs, t)
	}
	_, err = db.collection("templates").InsertMany(ctx, docs)
	return err
}

func (db *DB) ListTemplates(ctx context.Context, userID uuid.UUID) ([]Template, error) {
	filter := bson.M{
		"$or": []bson.M{
			{"is_system": true},
			{"user_id": userID},
		},
	}
	opts := options.Find().SetSort(bson.D{{Key: "is_system", Value: -1}, {Key: "name", Value: 1}})
	cur, err := db.collection("templates").Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []Template
	for cur.Next(ctx) {
		var doc templateDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, templateFromDoc(doc))
	}
	return out, cur.Err()
}

func (db *DB) CreateTemplate(ctx context.Context, userID uuid.UUID, name, title, description, priority string, offsets []int) (Template, error) {
	name = strings.TrimSpace(name)
	title = strings.TrimSpace(title)
	if name == "" {
		return Template{}, fmt.Errorf("name is required")
	}
	if title == "" {
		return Template{}, fmt.Errorf("title is required")
	}
	if priority == "" {
		priority = reminder.PriorityNormal
	}
	now := time.Now().UTC()
	doc := templateDoc{
		ID:          uuid.New(),
		UserID:      &userID,
		Name:        name,
		Title:       title,
		Description: strings.TrimSpace(description),
		Priority:    priority,
		Offsets:     offsets,
		IsSystem:    false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, err := db.collection("templates").InsertOne(ctx, doc)
	if err != nil {
		return Template{}, err
	}
	return templateFromDoc(doc), nil
}

func (db *DB) GetTemplate(ctx context.Context, userID, id uuid.UUID) (Template, error) {
	var doc templateDoc
	err := db.collection("templates").FindOne(ctx, bson.M{
		"_id": id,
		"$or": []bson.M{
			{"is_system": true},
			{"user_id": userID},
		},
	}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return Template{}, ErrNotFound
	}
	if err != nil {
		return Template{}, err
	}
	return templateFromDoc(doc), nil
}
