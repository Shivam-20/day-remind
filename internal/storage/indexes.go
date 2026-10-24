package storage

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func ensureIndexes(ctx context.Context, db *mongo.Database) error {
	models := []struct {
		collection string
		indexes    []mongo.IndexModel
	}{
		{
			collection: "users",
			indexes: []mongo.IndexModel{
				{
					Keys:    bson.D{{Key: "email", Value: 1}},
					Options: options.Index().SetUnique(true),
				},
				{
					// Supports the last-admin guard and the admin count.
					Keys:    bson.D{{Key: "role", Value: 1}},
					Options: options.Index(),
				},
				{
					// Resolving an incoming Telegram message to an account.
					Keys:    bson.D{{Key: "telegram_chat_id", Value: 1}},
					Options: options.Index().SetSparse(true),
				},
			},
		},
		{
			collection: "sessions",
			indexes: []mongo.IndexModel{
				{
					Keys:    bson.D{{Key: "user_id", Value: 1}},
					Options: options.Index(),
				},
				{
					Keys:    bson.D{{Key: "expires_at", Value: 1}},
					Options: options.Index().SetExpireAfterSeconds(0),
				},
			},
		},
		{
			collection: "deliveries",
			indexes: []mongo.IndexModel{
				{
					Keys:    bson.D{{Key: "created_at", Value: -1}},
					Options: options.Index(),
				},
				{
					// The log is diagnostic, not a system of record; drop
					// entries after 30 days so it cannot grow without bound.
					Keys:    bson.D{{Key: "created_at", Value: 1}},
					Options: options.Index().SetExpireAfterSeconds(30 * 24 * 60 * 60),
				},
			},
		},
		{
			collection: "reminders",
			indexes: []mongo.IndexModel{
				{
					Keys: bson.D{
						{Key: "user_id", Value: 1},
						{Key: "due_at", Value: 1},
					},
					Options: options.Index(),
				},
				{
					Keys: bson.D{
						{Key: "user_id", Value: 1},
						{Key: "status", Value: 1},
						{Key: "due_at", Value: 1},
					},
					Options: options.Index(),
				},
				{
					// The scheduler's sweep: unclaimed scheduled reminders.
					Keys: bson.D{
						{Key: "status", Value: 1},
						{Key: "due_at", Value: 1},
					},
					Options: options.Index(),
				},
			},
		},
	}

	for _, group := range models {
		if _, err := db.Collection(group.collection).Indexes().CreateMany(ctx, group.indexes); err != nil {
			return fmt.Errorf("ensure indexes on %s: %w", group.collection, err)
		}
	}
	return nil
}
