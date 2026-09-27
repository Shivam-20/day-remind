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
					Keys:    bson.D{{Key: "email_ingest_token", Value: 1}},
					Options: options.Index().SetUnique(true).SetSparse(true),
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
					Keys: bson.D{
						{Key: "user_id", Value: 1},
						{Key: "list_id", Value: 1},
					},
					Options: options.Index(),
				},
				{
					Keys: bson.D{
						{Key: "user_id", Value: 1},
						{Key: "tag_ids", Value: 1},
					},
					Options: options.Index(),
				},
				{
					Keys: bson.D{
						{Key: "user_id", Value: 1},
						{Key: "priority", Value: 1},
					},
					Options: options.Index(),
				},
			},
		},
		{
			collection: "lists",
			indexes: []mongo.IndexModel{
				{
					Keys: bson.D{
						{Key: "user_id", Value: 1},
						{Key: "sort_order", Value: 1},
					},
					Options: options.Index(),
				},
			},
		},
		{
			collection: "tags",
			indexes: []mongo.IndexModel{
				{
					Keys: bson.D{
						{Key: "user_id", Value: 1},
						{Key: "name", Value: 1},
					},
					Options: options.Index().SetUnique(true),
				},
			},
		},
		{
			collection: "subtasks",
			indexes: []mongo.IndexModel{
				{
					Keys: bson.D{
						{Key: "reminder_id", Value: 1},
						{Key: "sort_order", Value: 1},
					},
					Options: options.Index(),
				},
				{
					Keys: bson.D{
						{Key: "user_id", Value: 1},
						{Key: "reminder_id", Value: 1},
					},
					Options: options.Index(),
				},
			},
		},
		{
			collection: "reminder_offsets",
			indexes: []mongo.IndexModel{
				{
					Keys: bson.D{
						{Key: "reminder_id", Value: 1},
						{Key: "offset_minutes", Value: -1},
					},
					Options: options.Index(),
				},
				{
					Keys: bson.D{
						{Key: "user_id", Value: 1},
						{Key: "reminder_id", Value: 1},
					},
					Options: options.Index(),
				},
			},
		},
		{
			collection: "reminder_occurrences",
			indexes: []mongo.IndexModel{
				{
					Keys: bson.D{
						{Key: "fired", Value: 1},
						{Key: "fire_at", Value: 1},
					},
					Options: options.Index(),
				},
				{
					Keys: bson.D{
						{Key: "user_id", Value: 1},
						{Key: "reminder_id", Value: 1},
						{Key: "fire_at", Value: 1},
					},
					Options: options.Index(),
				},
			},
		},
		{
			collection: "reminder_deliveries",
			indexes: []mongo.IndexModel{
				{
					Keys:    bson.D{{Key: "idempotency_key", Value: 1}},
					Options: options.Index().SetUnique(true),
				},
				{
					Keys: bson.D{
						{Key: "reminder_id", Value: 1},
						{Key: "created_at", Value: -1},
					},
					Options: options.Index(),
				},
				{
					Keys: bson.D{
						{Key: "user_id", Value: 1},
						{Key: "created_at", Value: -1},
					},
					Options: options.Index(),
				},
			},
		},
		{
			collection: "telegram_link_tokens",
			indexes: []mongo.IndexModel{
				{
					Keys:    bson.D{{Key: "expires_at", Value: 1}},
					Options: options.Index().SetExpireAfterSeconds(0),
				},
			},
		},
		{
			collection: "password_reset_tokens",
			indexes: []mongo.IndexModel{
				{
					Keys:    bson.D{{Key: "expires_at", Value: 1}},
					Options: options.Index().SetExpireAfterSeconds(0),
				},
			},
		},
		{
			collection: "email_verification_tokens",
			indexes: []mongo.IndexModel{
				{
					Keys:    bson.D{{Key: "expires_at", Value: 1}},
					Options: options.Index().SetExpireAfterSeconds(0),
				},
			},
		},
		{
			collection: "templates",
			indexes: []mongo.IndexModel{
				{
					Keys: bson.D{
						{Key: "user_id", Value: 1},
						{Key: "name", Value: 1},
					},
					Options: options.Index(),
				},
				{
					Keys:    bson.D{{Key: "is_system", Value: 1}},
					Options: options.Index(),
				},
			},
		},
		{
			collection: "api_keys",
			indexes: []mongo.IndexModel{
				{
					Keys:    bson.D{{Key: "token_hash", Value: 1}},
					Options: options.Index().SetUnique(true),
				},
				{
					Keys: bson.D{
						{Key: "user_id", Value: 1},
						{Key: "created_at", Value: -1},
					},
					Options: options.Index(),
				},
			},
		},
		{
			collection: "reminder_shares",
			indexes: []mongo.IndexModel{
				{
					Keys: bson.D{
						{Key: "reminder_id", Value: 1},
						{Key: "shared_with_email", Value: 1},
					},
					Options: options.Index().SetUnique(true),
				},
				{
					Keys:    bson.D{{Key: "shared_with_email", Value: 1}},
					Options: options.Index(),
				},
			},
		},
		{
			collection: "groups",
			indexes: []mongo.IndexModel{
				{
					Keys: bson.D{
						{Key: "owner_id", Value: 1},
						{Key: "name", Value: 1},
					},
					Options: options.Index(),
				},
			},
		},
		{
			collection: "group_members",
			indexes: []mongo.IndexModel{
				{
					Keys: bson.D{
						{Key: "group_id", Value: 1},
						{Key: "email", Value: 1},
					},
					Options: options.Index().SetUnique(true),
				},
				{
					Keys:    bson.D{{Key: "user_id", Value: 1}},
					Options: options.Index(),
				},
			},
		},
		{
			collection: "reminder_comments",
			indexes: []mongo.IndexModel{
				{
					Keys: bson.D{
						{Key: "reminder_id", Value: 1},
						{Key: "created_at", Value: 1},
					},
					Options: options.Index(),
				},
			},
		},
		{
			collection: "webhooks",
			indexes: []mongo.IndexModel{
				{
					Keys: bson.D{
						{Key: "user_id", Value: 1},
						{Key: "created_at", Value: -1},
					},
					Options: options.Index(),
				},
				{
					Keys: bson.D{
						{Key: "enabled", Value: 1},
						{Key: "events", Value: 1},
					},
					Options: options.Index(),
				},
			},
		},
		{
			collection: "push_subscriptions",
			indexes: []mongo.IndexModel{
				{
					Keys:    bson.D{{Key: "user_id", Value: 1}},
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
