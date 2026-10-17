package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const sessionTTL = 30 * 24 * time.Hour

type sessionDoc struct {
	Token     string    `bson:"_id"`
	UserID    uuid.UUID `bson:"user_id"`
	ExpiresAt time.Time `bson:"expires_at"`
	CreatedAt time.Time `bson:"created_at"`
}

func (db *DB) CreateSession(ctx context.Context, userID uuid.UUID) (string, error) {
	token, err := newSessionToken()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	doc := sessionDoc{
		Token:     token,
		UserID:    userID,
		ExpiresAt: now.Add(sessionTTL),
		CreatedAt: now,
	}
	_, err = db.collection("sessions").InsertOne(ctx, doc)
	if err != nil {
		return "", err
	}
	return token, nil
}

func (db *DB) DeleteSession(ctx context.Context, token string) error {
	_, err := db.collection("sessions").DeleteOne(ctx, bson.M{"_id": token})
	return err
}

// DeleteOtherSessions signs a user out everywhere except the given token, e.g.
// after a password change so stolen cookies stop working.
func (db *DB) DeleteOtherSessions(ctx context.Context, userID uuid.UUID, keepToken string) error {
	_, err := db.collection("sessions").DeleteMany(ctx, bson.M{
		"user_id": userID,
		"_id":     bson.M{"$ne": keepToken},
	})
	return err
}

func (db *DB) GetSessionUserID(ctx context.Context, token string) (uuid.UUID, error) {
	var doc sessionDoc
	err := db.collection("sessions").FindOne(ctx, bson.M{"_id": token}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, err
	}
	if time.Now().UTC().After(doc.ExpiresAt) {
		_, _ = db.collection("sessions").DeleteOne(ctx, bson.M{"_id": token})
		return uuid.Nil, ErrNotFound
	}
	return doc.UserID, nil
}

func newSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("session token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
