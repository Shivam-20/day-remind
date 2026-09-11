package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const apiKeyPrefix = "dr_"

type APIKey struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	Name       string
	TokenHash  string
	Prefix     string
	LastUsedAt *time.Time
	CreatedAt  time.Time
}

type apiKeyDoc struct {
	ID         uuid.UUID  `bson:"_id"`
	UserID     uuid.UUID  `bson:"user_id"`
	Name       string     `bson:"name"`
	TokenHash  string     `bson:"token_hash"`
	Prefix     string     `bson:"prefix"`
	LastUsedAt *time.Time `bson:"last_used_at,omitempty"`
	CreatedAt  time.Time  `bson:"created_at"`
}

func apiKeyFromDoc(doc apiKeyDoc) APIKey {
	return APIKey{
		ID:         doc.ID,
		UserID:     doc.UserID,
		Name:       doc.Name,
		TokenHash:  doc.TokenHash,
		Prefix:     doc.Prefix,
		LastUsedAt: doc.LastUsedAt,
		CreatedAt:  doc.CreatedAt,
	}
}

func hashAPIKeyToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func newAPIKeyToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("api key token: %w", err)
	}
	return apiKeyPrefix + hex.EncodeToString(b), nil
}

func (db *DB) CreateAPIKey(ctx context.Context, userID uuid.UUID, name string) (APIKey, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return APIKey{}, "", fmt.Errorf("name is required")
	}
	raw, err := newAPIKeyToken()
	if err != nil {
		return APIKey{}, "", err
	}
	now := time.Now().UTC()
	doc := apiKeyDoc{
		ID:        uuid.New(),
		UserID:    userID,
		Name:      name,
		TokenHash: hashAPIKeyToken(raw),
		Prefix:    raw[:12],
		CreatedAt: now,
	}
	_, err = db.collection("api_keys").InsertOne(ctx, doc)
	if err != nil {
		return APIKey{}, "", err
	}
	return apiKeyFromDoc(doc), raw, nil
}

func (db *DB) ListAPIKeys(ctx context.Context, userID uuid.UUID) ([]APIKey, error) {
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
	cur, err := db.collection("api_keys").Find(ctx, bson.M{"user_id": userID}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []APIKey
	for cur.Next(ctx) {
		var doc apiKeyDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, apiKeyFromDoc(doc))
	}
	return out, cur.Err()
}

func (db *DB) RevokeAPIKey(ctx context.Context, userID, id uuid.UUID) error {
	res, err := db.collection("api_keys").DeleteOne(ctx, bson.M{"_id": id, "user_id": userID})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (db *DB) GetUserByAPIKeyHash(ctx context.Context, tokenHash string) (User, error) {
	var keyDoc apiKeyDoc
	err := db.collection("api_keys").FindOne(ctx, bson.M{"token_hash": tokenHash}).Decode(&keyDoc)
	if err == mongo.ErrNoDocuments {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	now := time.Now().UTC()
	_, _ = db.collection("api_keys").UpdateOne(ctx, bson.M{"_id": keyDoc.ID}, bson.M{
		"$set": bson.M{"last_used_at": now},
	})
	return db.GetUserByID(ctx, keyDoc.UserID)
}

func HashAPIKeyToken(token string) string {
	return hashAPIKeyToken(token)
}
