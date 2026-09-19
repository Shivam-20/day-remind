package storage

import (
	"context"
	"errors"
	neturl "net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/auth"
	"github.com/shivam-20/day-remind/internal/reminder"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func uniqueEmail() string {
	return "test-" + uuid.NewString() + "@example.com"
}

// testDB gives each test its own database, dropped afterwards.
//
// This is not only about isolation between tests: the settings document is a
// singleton, so a test that clears it would otherwise wipe the delivery
// configuration of whatever database DATABASE_URL happens to point at —
// including a developer's own running instance.
func testDB(t *testing.T) *DB {
	t.Helper()
	raw := os.Getenv("DATABASE_URL")
	if raw == "" {
		t.Skip("DATABASE_URL not set")
	}
	name := "day_remind_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	u, err := neturl.Parse(raw)
	if err != nil {
		t.Fatalf("parse DATABASE_URL: %v", err)
	}
	u.Path = "/" + name

	db, err := Open(u.String())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if c, err := mongo.Connect(ctx, options.Client().ApplyURI(u.String())); err == nil {
			_ = c.Database(name).Drop(ctx)
			_ = c.Disconnect(ctx)
		}
		_ = db.Close()
	})
	return db
}

func TestUserReminderFlow(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	email := uniqueEmail()
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

// A PATCH that carries only one field must leave the others alone.
func TestUpdateReminderPartial(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatal(err)
	}
	user, err := db.CreateUser(ctx, uniqueEmail(), hash, "Test", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	item, err := db.CreateReminder(ctx, user.ID, reminder.CreateInput{
		Title:       "Pay bill",
		Description: "electricity",
		DueAt:       time.Now().UTC().Add(2 * time.Hour),
		Timezone:    "Asia/Kolkata",
		Priority:    reminder.PriorityHigh,
	})
	if err != nil {
		t.Fatal(err)
	}

	newDue := item.DueAt.Add(24 * time.Hour)
	got, err := db.UpdateReminder(ctx, user.ID, item.ID, reminder.UpdateInput{DueAt: &newDue})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Title != "Pay bill" {
		t.Errorf("title clobbered: %q", got.Title)
	}
	if got.Description != "electricity" {
		t.Errorf("description clobbered: %q", got.Description)
	}
	if got.Priority != reminder.PriorityHigh {
		t.Errorf("priority clobbered: %q", got.Priority)
	}
	if got.Timezone != "Asia/Kolkata" {
		t.Errorf("timezone clobbered: %q", got.Timezone)
	}
	// BSON datetimes are millisecond precision, so compare at that resolution.
	if !got.DueAt.Equal(newDue.UTC().Truncate(time.Millisecond)) {
		t.Errorf("due_at not applied: got %v want %v", got.DueAt, newDue.UTC())
	}
}

// completed_at must not survive a reminder being reopened.
func TestCompletedAtClearedOnReopen(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	hash, _ := auth.HashPassword("password123")
	user, err := db.CreateUser(ctx, uniqueEmail(), hash, "Test", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	item, err := db.CreateReminder(ctx, user.ID, reminder.CreateInput{
		Title: "Ship it",
		DueAt: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	done := reminder.StatusDone
	item, err = db.UpdateReminder(ctx, user.ID, item.ID, reminder.UpdateInput{Status: &done})
	if err != nil {
		t.Fatal(err)
	}
	if item.CompletedAt == nil {
		t.Fatal("expected completed_at to be set when marked done")
	}

	scheduled := reminder.StatusScheduled
	item, err = db.UpdateReminder(ctx, user.ID, item.ID, reminder.UpdateInput{Status: &scheduled})
	if err != nil {
		t.Fatal(err)
	}
	if item.CompletedAt != nil {
		t.Errorf("expected completed_at to be cleared, got %v", item.CompletedAt)
	}
}

func TestUpdateReminderRejectsUnknownEnums(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	hash, _ := auth.HashPassword("password123")
	user, err := db.CreateUser(ctx, uniqueEmail(), hash, "Test", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	item, err := db.CreateReminder(ctx, user.ID, reminder.CreateInput{
		Title: "Guard rails",
		DueAt: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	bogus := "banana"
	if _, err := db.UpdateReminder(ctx, user.ID, item.ID, reminder.UpdateInput{Priority: &bogus}); !errors.Is(err, ErrInvalid) {
		t.Errorf("expected ErrInvalid for priority, got %v", err)
	}
	if _, err := db.UpdateReminder(ctx, user.ID, item.ID, reminder.UpdateInput{Status: &bogus}); !errors.Is(err, ErrInvalid) {
		t.Errorf("expected ErrInvalid for status, got %v", err)
	}
}

// Every reminder query is scoped by user_id; a second user must not be able to
// read or mutate the first user's rows.
func TestReminderIsolationBetweenUsers(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	hash, _ := auth.HashPassword("password123")
	owner, err := db.CreateUser(ctx, uniqueEmail(), hash, "Owner", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	other, err := db.CreateUser(ctx, uniqueEmail(), hash, "Other", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	item, err := db.CreateReminder(ctx, owner.ID, reminder.CreateInput{
		Title: "Private",
		DueAt: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.GetReminder(ctx, other.ID, item.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound reading another user's reminder, got %v", err)
	}
	if err := db.DeleteReminder(ctx, other.ID, item.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound deleting another user's reminder, got %v", err)
	}
	if _, err := db.GetReminder(ctx, owner.ID, item.ID); err != nil {
		t.Errorf("owner lost access to their own reminder: %v", err)
	}
}

// A search term is matched literally, not compiled as a regular expression.
func TestSearchTreatsQueryAsLiteral(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	hash, _ := auth.HashPassword("password123")
	user, err := db.CreateUser(ctx, uniqueEmail(), hash, "Test", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateReminder(ctx, user.ID, reminder.CreateInput{
		Title: "Renew a+b policy",
		DueAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	// An unescaped "(((" is an invalid regex and used to fail the whole query.
	if _, err := db.ListReminders(ctx, reminder.ListFilter{UserID: user.ID, Query: "((("}); err != nil {
		t.Errorf("malformed regex characters should be literal, got %v", err)
	}
	got, err := db.ListReminders(ctx, reminder.ListFilter{UserID: user.ID, Query: "a+b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("expected literal match on %q, got %d results", "a+b", len(got))
	}
}

func TestCreateUserRejectsBadEmail(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	hash, _ := auth.HashPassword("password123")
	for _, email := range []string{"", "   ", "not-an-email", "no@domain", "two@@at.com"} {
		if _, err := db.CreateUser(ctx, email, hash, "Test", "UTC"); !errors.Is(err, ErrInvalid) {
			t.Errorf("CreateUser(%q): expected ErrInvalid, got %v", email, err)
		}
	}
}

func newTestUser(t *testing.T, db *DB, ctx context.Context) User {
	t.Helper()
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatal(err)
	}
	u, err := db.CreateUser(ctx, uniqueEmail(), hash, "Test", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// A user document written before the role field existed must read as a plain
// user rather than as an empty role.
func TestRoleDefaultsAndTransitions(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	user := newTestUser(t, db, ctx)
	if user.Role != RoleUser {
		t.Fatalf("new user should default to %q, got %q", RoleUser, user.Role)
	}

	// Simulate a legacy document by removing the field entirely.
	if _, err := db.collection("users").UpdateOne(ctx, bson.M{"_id": user.ID},
		bson.M{"$unset": bson.M{"role": ""}}); err != nil {
		t.Fatal(err)
	}
	legacy, err := db.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Role != RoleUser {
		t.Errorf("missing role field should read as %q, got %q", RoleUser, legacy.Role)
	}

	promoted, err := db.SetUserRole(ctx, user.ID, RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if promoted.Role != RoleAdmin {
		t.Errorf("expected admin, got %q", promoted.Role)
	}
	if _, err := db.SetUserRole(ctx, user.ID, "superuser"); !errors.Is(err, ErrInvalid) {
		t.Errorf("expected ErrInvalid for unknown role, got %v", err)
	}
	if _, err := db.SetUserRole(ctx, uuid.New(), RoleAdmin); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for unknown user, got %v", err)
	}

	// Leave the collection as we found it.
	if _, err := db.SetUserRole(ctx, user.ID, RoleUser); err != nil {
		t.Fatal(err)
	}
}

// Deleting a user must not leave reminders or sessions pointing at them.
func TestDeleteUserCascades(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	user := newTestUser(t, db, ctx)
	for i := 0; i < 3; i++ {
		if _, err := db.CreateReminder(ctx, user.ID, reminder.CreateInput{
			Title: "keep me private",
			DueAt: time.Now().UTC().Add(time.Duration(i+1) * time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.CreateSession(ctx, user.ID); err != nil {
		t.Fatal(err)
	}

	if err := db.DeleteUser(ctx, user.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := db.GetUserByID(ctx, user.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("user should be gone, got %v", err)
	}
	remaining, err := db.collection("reminders").CountDocuments(ctx, bson.M{"user_id": user.ID})
	if err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Errorf("expected reminders to cascade, %d left", remaining)
	}
	sessions, err := db.collection("sessions").CountDocuments(ctx, bson.M{"user_id": user.ID})
	if err != nil {
		t.Fatal(err)
	}
	if sessions != 0 {
		t.Errorf("expected sessions to cascade, %d left", sessions)
	}
	if err := db.DeleteUser(ctx, user.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete should report ErrNotFound, got %v", err)
	}
}

func TestListUsersForAdminCarriesStatsNotContent(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	user := newTestUser(t, db, ctx)
	defer func() { _ = db.DeleteUser(ctx, user.ID) }()

	past, err := db.CreateReminder(ctx, user.ID, reminder.CreateInput{
		Title: "overdue one",
		DueAt: time.Now().UTC().Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateReminder(ctx, user.ID, reminder.CreateInput{
		Title: "future one",
		DueAt: time.Now().UTC().Add(48 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	doneItem, err := db.CreateReminder(ctx, user.ID, reminder.CreateInput{
		Title: "finished one",
		DueAt: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	done := reminder.StatusDone
	if _, err := db.UpdateReminder(ctx, user.ID, doneItem.ID, reminder.UpdateInput{Status: &done}); err != nil {
		t.Fatal(err)
	}

	rows, err := db.ListUsersForAdmin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var got *AdminUser
	for i := range rows {
		if rows[i].ID == user.ID {
			got = &rows[i]
			break
		}
	}
	if got == nil {
		t.Fatal("created user missing from admin listing")
	}
	if got.Stats.Total != 3 {
		t.Errorf("total = %d, want 3", got.Stats.Total)
	}
	if got.Stats.Scheduled != 2 {
		t.Errorf("scheduled = %d, want 2", got.Stats.Scheduled)
	}
	if got.Stats.Done != 1 {
		t.Errorf("done = %d, want 1", got.Stats.Done)
	}
	if got.Stats.Overdue != 1 {
		t.Errorf("overdue = %d, want 1 (the one due %v)", got.Stats.Overdue, past.DueAt)
	}
	if got.Stats.NextDueAt == nil {
		t.Error("expected a next due date from the future reminder")
	} else if got.Stats.NextDueAt.Before(time.Now().UTC()) {
		t.Errorf("next due should be in the future, got %v", got.Stats.NextDueAt)
	}
}

func TestCountAdmins(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	before, err := db.CountAdmins(ctx)
	if err != nil {
		t.Fatal(err)
	}
	user := newTestUser(t, db, ctx)
	defer func() { _ = db.DeleteUser(ctx, user.ID) }()

	if _, err := db.SetUserRole(ctx, user.ID, RoleAdmin); err != nil {
		t.Fatal(err)
	}
	after, err := db.CountAdmins(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after != before+1 {
		t.Errorf("admin count = %d, want %d", after, before+1)
	}
}
