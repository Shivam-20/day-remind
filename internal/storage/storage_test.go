package storage

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/shivam-20/day-remind/internal/auth"
	"github.com/shivam-20/day-remind/internal/reminder"
)

func testDB(t *testing.T) *DB {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	db, err := Open(url)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return db
}

func TestUserReminderFlow(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := context.Background()

	email := "test-" + time.Now().Format("150405") + "@example.com"
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatal(err)
	}
	user, err := db.CreateUser(ctx, email, hash, "Test", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	due := time.Now().UTC().Add(2 * time.Hour)
	item, err := db.CreateReminder(ctx, user.ID, reminder.CreateInput{
		Title: "Pay bill",
		DueAt: due,
	})
	if err != nil {
		t.Fatal(err)
	}
	item, err = db.PauseReminder(ctx, user.ID, item.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != reminder.StatusPaused {
		t.Fatalf("expected paused, got %s", item.Status)
	}
	list, err := db.ListReminders(ctx, reminder.ListFilter{UserID: user.ID})
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v len=%d", err, len(list))
	}
}
