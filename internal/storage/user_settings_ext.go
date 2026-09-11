package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

func (db *DB) UpdateMorningDigest(ctx context.Context, userID uuid.UUID, enabled bool, digestTime string) error {
	digestTime = strings.TrimSpace(digestTime)
	res, err := db.collection("users").UpdateOne(ctx, bson.M{"_id": userID}, bson.M{
		"$set": bson.M{
			"morning_digest_enabled": enabled,
			"morning_digest_time":    digestTime,
			"updated_at":             time.Now().UTC(),
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

func (db *DB) EnsureEmailIngestToken(ctx context.Context, userID uuid.UUID) (string, error) {
	var doc userDoc
	err := db.collection("users").FindOne(ctx, bson.M{"_id": userID}).Decode(&doc)
	if err != nil {
		return "", err
	}
	if doc.EmailIngestToken != "" {
		return doc.EmailIngestToken, nil
	}
	token, err := newEmailIngestToken()
	if err != nil {
		return "", err
	}
	_, err = db.collection("users").UpdateOne(ctx, bson.M{"_id": userID}, bson.M{
		"$set": bson.M{
			"email_ingest_token": token,
			"updated_at":         time.Now().UTC(),
		},
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

func newEmailIngestToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("email ingest token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
