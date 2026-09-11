package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func (db *DB) UpdateUserQuietHours(ctx context.Context, userID uuid.UUID, quietStart, quietEnd string) error {
	res, err := db.collection("users").UpdateOne(ctx, bson.M{"_id": userID}, bson.M{
		"$set": bson.M{
			"quiet_start": strings.TrimSpace(quietStart),
			"quiet_end":   strings.TrimSpace(quietEnd),
			"updated_at":  time.Now().UTC(),
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

func (db *DB) CreatePasswordResetToken(ctx context.Context, email string) (string, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	user, err := db.GetUserByEmail(ctx, email)
	if err == ErrNotFound {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	now := time.Now().UTC()
	_, err = db.collection("password_reset_tokens").InsertOne(ctx, bson.M{
		"_id":        token,
		"user_id":    user.ID,
		"expires_at": now.Add(time.Hour),
		"created_at": now,
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

func (db *DB) ConsumePasswordResetToken(ctx context.Context, token, passwordHash string) error {
	token = strings.TrimSpace(token)
	var doc struct {
		UserID    uuid.UUID `bson:"user_id"`
		ExpiresAt time.Time `bson:"expires_at"`
	}
	err := db.collection("password_reset_tokens").FindOne(ctx, bson.M{"_id": token}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if time.Now().UTC().After(doc.ExpiresAt) {
		_, _ = db.collection("password_reset_tokens").DeleteOne(ctx, bson.M{"_id": token})
		return ErrNotFound
	}
	res, err := db.collection("users").UpdateOne(ctx, bson.M{"_id": doc.UserID}, bson.M{
		"$set": bson.M{
			"password_hash": passwordHash,
			"updated_at":    time.Now().UTC(),
		},
	})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	_, _ = db.collection("password_reset_tokens").DeleteMany(ctx, bson.M{"user_id": doc.UserID})
	return nil
}

func (db *DB) GetUserQuietHours(ctx context.Context, userID uuid.UUID) (start, end string, err error) {
	var doc userDoc
	err = db.collection("users").FindOne(ctx, bson.M{"_id": userID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", err
	}
	return doc.QuietStart, doc.QuietEnd, nil
}
