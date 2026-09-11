package storage

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

const defaultDatabase = "day_remind"

type DB struct {
	client      *mongo.Client
	db          *mongo.Database
	adminEmails map[string]struct{}
}

func Open(databaseURL string, adminEmails []string) (*DB, error) {
	databaseURL = strings.TrimSpace(databaseURL)
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	dbName, err := databaseName(databaseURL)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(databaseURL))
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}
	database := client.Database(dbName)
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("ping database: %w", err)
	}
	if err := ensureIndexes(ctx, database); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}
	store := &DB{
		client:      client,
		db:          database,
		adminEmails: normalizeAdminEmails(adminEmails),
	}
	if err := store.EnsureAdminEmails(ctx); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}
	return store, nil
}

func normalizeAdminEmails(emails []string) map[string]struct{} {
	out := make(map[string]struct{}, len(emails))
	for _, email := range emails {
		email = strings.TrimSpace(strings.ToLower(email))
		if email != "" {
			out[email] = struct{}{}
		}
	}
	return out
}

func (db *DB) isConfiguredAdmin(email string) bool {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return false
	}
	_, ok := db.adminEmails[email]
	return ok
}

func databaseName(databaseURL string) (string, error) {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return "", fmt.Errorf("parse database url: %w", err)
	}
	name := strings.TrimPrefix(u.Path, "/")
	if idx := strings.Index(name, "?"); idx >= 0 {
		name = name[:idx]
	}
	if name == "" {
		return defaultDatabase, nil
	}
	return name, nil
}

func (db *DB) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return db.client.Disconnect(ctx)
}

func (db *DB) collection(name string) *mongo.Collection {
	return db.db.Collection(name)
}
