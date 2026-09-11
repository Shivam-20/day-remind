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

const (
	SharePermissionView = "view"
	SharePermissionEdit = "edit"
)

type ReminderShare struct {
	ID              uuid.UUID
	ReminderID      uuid.UUID
	OwnerID         uuid.UUID
	SharedWithEmail string
	Permission      string
	Accepted        bool
}

type reminderShareDoc struct {
	ID              uuid.UUID `bson:"_id"`
	ReminderID      uuid.UUID `bson:"reminder_id"`
	OwnerID         uuid.UUID `bson:"owner_id"`
	SharedWithEmail string    `bson:"shared_with_email"`
	Permission      string    `bson:"permission"`
	Accepted        bool      `bson:"accepted"`
	CreatedAt       time.Time `bson:"created_at"`
	UpdatedAt       time.Time `bson:"updated_at"`
}

func shareFromDoc(doc reminderShareDoc) ReminderShare {
	return ReminderShare{
		ID:              doc.ID,
		ReminderID:      doc.ReminderID,
		OwnerID:         doc.OwnerID,
		SharedWithEmail: doc.SharedWithEmail,
		Permission:      doc.Permission,
		Accepted:        doc.Accepted,
	}
}

func (db *DB) CreateShare(ctx context.Context, ownerID, reminderID uuid.UUID, email, permission string) (ReminderShare, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return ReminderShare{}, fmt.Errorf("email is required")
	}
	if permission != SharePermissionView && permission != SharePermissionEdit {
		return ReminderShare{}, fmt.Errorf("permission must be view or edit")
	}
	if _, err := db.GetReminder(ctx, ownerID, reminderID); err != nil {
		return ReminderShare{}, err
	}
	now := time.Now().UTC()
	doc := reminderShareDoc{
		ID:              uuid.New(),
		ReminderID:      reminderID,
		OwnerID:         ownerID,
		SharedWithEmail: email,
		Permission:      permission,
		Accepted:        false,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	_, err := db.collection("reminder_shares").InsertOne(ctx, doc)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ReminderShare{}, ErrConflict
		}
		return ReminderShare{}, err
	}
	return shareFromDoc(doc), nil
}

func (db *DB) ListSharesForReminder(ctx context.Context, ownerID, reminderID uuid.UUID) ([]ReminderShare, error) {
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}})
	cur, err := db.collection("reminder_shares").Find(ctx, bson.M{
		"reminder_id": reminderID,
		"owner_id":    ownerID,
	}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []ReminderShare
	for cur.Next(ctx) {
		var doc reminderShareDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, shareFromDoc(doc))
	}
	return out, cur.Err()
}

func (db *DB) AcceptShare(ctx context.Context, shareID uuid.UUID, email string) (ReminderShare, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	now := time.Now().UTC()
	res, err := db.collection("reminder_shares").UpdateOne(ctx, bson.M{
		"_id":               shareID,
		"shared_with_email": email,
		"accepted":          false,
	}, bson.M{"$set": bson.M{
		"accepted":   true,
		"updated_at": now,
	}})
	if err != nil {
		return ReminderShare{}, err
	}
	if res.MatchedCount == 0 {
		return ReminderShare{}, ErrNotFound
	}
	var doc reminderShareDoc
	err = db.collection("reminder_shares").FindOne(ctx, bson.M{"_id": shareID}).Decode(&doc)
	if err != nil {
		return ReminderShare{}, err
	}
	return shareFromDoc(doc), nil
}
