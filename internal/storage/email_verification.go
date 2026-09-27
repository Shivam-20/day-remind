package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const emailVerificationTokenTTL = 24 * time.Hour

// CreateEmailVerificationToken issues a fresh verification token for a user,
// replacing any outstanding tokens first so at most one link is valid.
func (db *DB) CreateEmailVerificationToken(ctx context.Context, userID uuid.UUID) (string, error) {
	if _, err := db.GetUserByID(ctx, userID); err != nil {
		return "", err
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	now := time.Now().UTC()
	_, err := db.collection("email_verification_tokens").InsertOne(ctx, bson.M{
		"_id":        token,
		"user_id":    userID,
		"expires_at": now.Add(emailVerificationTokenTTL),
		"created_at": now,
	})
	if err != nil {
		return "", err
	}
	_, _ = db.collection("email_verification_tokens").DeleteMany(ctx, bson.M{
		"user_id": userID,
		"_id":     bson.M{"$ne": token},
	})
	return token, nil
}

// ConsumeEmailVerificationToken marks the user's email verified and removes
// their tokens. Expired or unknown tokens return ErrNotFound.
func (db *DB) ConsumeEmailVerificationToken(ctx context.Context, token string) error {
	token = hexToken(token)
	if token == "" {
		return ErrNotFound
	}
	var doc struct {
		UserID    uuid.UUID `bson:"user_id"`
		ExpiresAt time.Time `bson:"expires_at"`
	}
	err := db.collection("email_verification_tokens").FindOne(ctx, bson.M{"_id": token}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if time.Now().UTC().After(doc.ExpiresAt) {
		_, _ = db.collection("email_verification_tokens").DeleteOne(ctx, bson.M{"_id": token})
		return ErrNotFound
	}
	res, err := db.collection("users").UpdateOne(ctx, bson.M{"_id": doc.UserID}, bson.M{
		"$set": bson.M{
			"email_verified_at": time.Now().UTC(),
			"updated_at":        time.Now().UTC(),
		},
	})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	_, _ = db.collection("email_verification_tokens").DeleteMany(ctx, bson.M{"user_id": doc.UserID})
	return nil
}

// SetUserEmailVerified sets or clears a user's verified state (admin action).
func (db *DB) SetUserEmailVerified(ctx context.Context, userID uuid.UUID, verified bool) error {
	var update bson.M
	if verified {
		update = bson.M{"$set": bson.M{"email_verified_at": time.Now().UTC(), "updated_at": time.Now().UTC()}}
	} else {
		update = bson.M{"$unset": bson.M{"email_verified_at": ""}, "$set": bson.M{"updated_at": time.Now().UTC()}}
	}
	res, err := db.collection("users").UpdateOne(ctx, bson.M{"_id": userID}, update)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func hexToken(raw string) string {
	out := make([]byte, 0, len(raw))
	for _, c := range raw {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			out = append(out, byte(c))
		}
	}
	if len(out) != 48 { // 24 bytes hex-encoded
		return ""
	}
	return string(out)
}
