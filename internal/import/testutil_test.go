package importdata

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/shivam-20/day-remind/internal/auth"
	"github.com/shivam-20/day-remind/internal/storage"
)

func openTestDB(t *testing.T) *storage.DB {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	db, err := storage.Open(url, nil)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return db
}

func createTestUser(t *testing.T, db *storage.DB) storage.User {
	ctx := context.Background()
	hash, err := auth.HashPassword("testpassword123")
	if err != nil {
		t.Fatal(err)
	}
	email := "import-test-" + randomSuffix() + "@example.com"
	user, err := db.CreateUser(ctx, email, hash, "Import Test", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func randomSuffix() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
