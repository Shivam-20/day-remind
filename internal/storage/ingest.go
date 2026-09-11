package storage

import (
	"context"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func (db *DB) GetUserByEmailIngestToken(ctx context.Context, token string) (User, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return User{}, ErrNotFound
	}
	var doc userDoc
	err := db.collection("users").FindOne(ctx, bson.M{"email_ingest_token": token}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	return userFromDoc(doc), nil
}
