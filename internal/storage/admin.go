package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/reminder"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type AdminStats struct {
	Users     int64
	Reminders int64
	Sessions  int64
}

func (db *DB) EnsureAdminEmails(ctx context.Context) error {
	if len(db.adminEmails) == 0 {
		return nil
	}
	emails := make([]string, 0, len(db.adminEmails))
	for email := range db.adminEmails {
		emails = append(emails, email)
	}
	// Only promote accounts whose ownership of the email is proven: a verified
	// email, or a passwordless account provisioned from a Launchpad SSO
	// identity. Otherwise anyone who registers an admin address first would be
	// promoted on the next restart.
	_, err := db.collection("users").UpdateMany(ctx, bson.M{
		"email": bson.M{"$in": emails},
		"$or": bson.A{
			bson.M{"email_verified_at": bson.M{"$ne": nil}},
			bson.M{"password_hash": ""},
		},
	}, bson.M{
		"$set": bson.M{
			"is_admin":   true,
			"updated_at": time.Now().UTC(),
		},
	})
	return err
}

func (db *DB) ListUsers(ctx context.Context, f UserListFilter) ([]User, error) {
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	filter := bson.M{}
	if q := strings.TrimSpace(f.Query); q != "" {
		filter["email"] = bson.M{"$regex": q, "$options": "i"}
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetLimit(int64(limit)).
		SetSkip(int64(offset))
	cur, err := db.collection("users").Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []User
	for cur.Next(ctx) {
		var doc userDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, userFromDoc(doc))
	}
	return out, cur.Err()
}

func (db *DB) CreateUserAdmin(ctx context.Context, email, passwordHash, displayName, timezone string, isAdmin bool) (User, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return User{}, fmt.Errorf("email is required")
	}
	if timezone == "" {
		timezone = "Asia/Kolkata"
	}
	if db.isConfiguredAdmin(email) {
		isAdmin = true
	}
	id := uuid.New()
	now := time.Now().UTC()
	doc := userDoc{
		ID:                 id,
		Email:              email,
		PasswordHash:       passwordHash,
		DisplayName:        strings.TrimSpace(displayName),
		Timezone:           timezone,
		Locale:             "en",
		IsAdmin:            isAdmin,
		NotificationPrefs:  defaultNotificationPrefs(email),
		EscalationOrder:    defaultEscalationOrder(),
		MaxActiveReminders: defaultMaxActiveReminders,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	_, err := db.collection("users").InsertOne(ctx, doc)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return User{}, ErrConflict
		}
		return User{}, err
	}
	return userFromDoc(doc), nil
}

func (db *DB) UpdateUserAdmin(ctx context.Context, id uuid.UUID, in AdminUserUpdate) (User, error) {
	// Hardcoded admins keep admin access — they cannot be demoted via the API.
	current, err := db.GetUserByID(ctx, id)
	if err != nil {
		return User{}, err
	}
	set := bson.M{"updated_at": time.Now().UTC()}
	unset := bson.M{}
	if in.Email != nil {
		email := strings.TrimSpace(strings.ToLower(*in.Email))
		if email == "" {
			return User{}, fmt.Errorf("email is required")
		}
		set["email"] = email
		// A changed address must be verified again.
		if email != current.Email {
			unset["email_verified_at"] = ""
		}
	}
	if in.DisplayName != nil {
		set["display_name"] = strings.TrimSpace(*in.DisplayName)
	}
	if in.Timezone != nil {
		set["timezone"] = strings.TrimSpace(*in.Timezone)
	}
	if in.Locale != nil {
		set["locale"] = strings.TrimSpace(*in.Locale)
	}
	if in.IsAdmin != nil {
		set["is_admin"] = *in.IsAdmin
	}
	if in.EmailVerified != nil {
		if *in.EmailVerified {
			set["email_verified_at"] = time.Now().UTC()
		} else {
			unset["email_verified_at"] = ""
		}
	}
	if in.NotificationsPaused != nil {
		set["notifications_paused"] = *in.NotificationsPaused
	}
	if in.NotificationsPausedUntil != nil || in.NotificationsPaused != nil {
		set["notifications_paused_until"] = in.NotificationsPausedUntil
	}
	if in.PasswordHash != nil {
		set["password_hash"] = *in.PasswordHash
	}

	// A hardcoded admin stays admin: ignore any demotion request.
	if isHardcodedAdmin(current.Email) {
		set["is_admin"] = true
	}

	update := bson.M{"$set": set}
	if len(unset) > 0 {
		update["$unset"] = unset
	}
	res, err := db.collection("users").UpdateOne(ctx, bson.M{"_id": id}, update)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return User{}, ErrConflict
		}
		return User{}, err
	}
	if res.MatchedCount == 0 {
		return User{}, ErrNotFound
	}

	if email, ok := set["email"].(string); ok && db.isConfiguredAdmin(email) {
		_, _ = db.collection("users").UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"is_admin": true}})
	}

	return db.GetUserByID(ctx, id)
}

// SetUserAdmin promotes or demotes a user. Used by SSO role mapping so a
// Launchpad "owner"/"admin" grants day-remind admin without a restart.
// Hardcoded and ADMIN_EMAILS admins are never demoted (they would only be
// re-promoted on the next restart).
func (db *DB) SetUserAdmin(ctx context.Context, id uuid.UUID, isAdmin bool) error {
	if !isAdmin {
		current, err := db.GetUserByID(ctx, id)
		if err != nil {
			return err
		}
		if isHardcodedAdmin(current.Email) || db.isConfiguredAdmin(current.Email) {
			return nil
		}
	}
	res, err := db.collection("users").UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"is_admin": isAdmin}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (db *DB) DeleteUser(ctx context.Context, id uuid.UUID) error {
	if current, err := db.GetUserByID(ctx, id); err != nil {
		return err
	} else if isHardcodedAdmin(current.Email) {
		return ErrForbidden
	}
	if _, err := db.collection("reminders").DeleteMany(ctx, bson.M{"user_id": id}); err != nil {
		return err
	}
	if _, err := db.collection("sessions").DeleteMany(ctx, bson.M{"user_id": id}); err != nil {
		return err
	}
	// Stop outbound webhooks for a deleted account.
	for _, coll := range []string{"webhooks", "webhook_deliveries"} {
		if _, err := db.collection(coll).DeleteMany(ctx, bson.M{"user_id": id}); err != nil {
			return err
		}
	}
	res, err := db.collection("users").DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (db *DB) AdminStats(ctx context.Context) (AdminStats, error) {
	users, err := db.collection("users").CountDocuments(ctx, bson.M{})
	if err != nil {
		return AdminStats{}, err
	}
	reminders, err := db.collection("reminders").CountDocuments(ctx, bson.M{})
	if err != nil {
		return AdminStats{}, err
	}
	sessions, err := db.collection("sessions").CountDocuments(ctx, bson.M{})
	if err != nil {
		return AdminStats{}, err
	}
	return AdminStats{Users: users, Reminders: reminders, Sessions: sessions}, nil
}

type AdminReminderFilter struct {
	UserID *uuid.UUID
	Status string
	From   *time.Time
	To     *time.Time
	Query  string
	Limit  int
	Offset int
}

func (db *DB) AdminListReminders(ctx context.Context, f AdminReminderFilter) ([]reminder.Reminder, error) {
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	filter := bson.M{}
	if f.UserID != nil {
		filter["user_id"] = *f.UserID
	}
	if f.Status != "" {
		filter["status"] = f.Status
	}
	if f.From != nil || f.To != nil {
		dueFilter := bson.M{}
		if f.From != nil {
			dueFilter["$gte"] = f.From.UTC()
		}
		if f.To != nil {
			dueFilter["$lte"] = f.To.UTC()
		}
		filter["due_at"] = dueFilter
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		filter["$or"] = []bson.M{
			{"title": bson.M{"$regex": q, "$options": "i"}},
			{"description": bson.M{"$regex": q, "$options": "i"}},
		}
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "due_at", Value: 1}}).
		SetLimit(int64(limit)).
		SetSkip(int64(offset))
	cur, err := db.collection("reminders").Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []reminder.Reminder
	for cur.Next(ctx) {
		var doc reminderDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, reminderFromDoc(doc))
	}
	return out, cur.Err()
}

func (db *DB) AdminGetReminder(ctx context.Context, id uuid.UUID) (reminder.Reminder, error) {
	var doc reminderDoc
	err := db.collection("reminders").FindOne(ctx, bson.M{"_id": id}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return reminder.Reminder{}, ErrNotFound
	}
	if err != nil {
		return reminder.Reminder{}, err
	}
	return reminderFromDoc(doc), nil
}

func (db *DB) AdminDeleteReminder(ctx context.Context, id uuid.UUID) error {
	res, err := db.collection("reminders").DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (db *DB) AdminCreateReminder(ctx context.Context, userID uuid.UUID, in reminder.CreateInput) (reminder.Reminder, error) {
	if _, err := db.GetUserByID(ctx, userID); err != nil {
		return reminder.Reminder{}, err
	}
	return db.CreateReminder(ctx, userID, in)
}

func (db *DB) AdminUpdateReminder(ctx context.Context, id uuid.UUID, in reminder.UpdateInput) (reminder.Reminder, error) {
	item, err := db.AdminGetReminder(ctx, id)
	if err != nil {
		return reminder.Reminder{}, err
	}
	return db.UpdateReminder(ctx, item.UserID, id, in)
}

func (db *DB) AdminPauseReminder(ctx context.Context, id uuid.UUID, resumeAt *time.Time) (reminder.Reminder, error) {
	item, err := db.AdminGetReminder(ctx, id)
	if err != nil {
		return reminder.Reminder{}, err
	}
	return db.PauseReminder(ctx, item.UserID, id, resumeAt)
}

func (db *DB) AdminResumeReminder(ctx context.Context, id uuid.UUID) (reminder.Reminder, error) {
	item, err := db.AdminGetReminder(ctx, id)
	if err != nil {
		return reminder.Reminder{}, err
	}
	return db.ResumeReminder(ctx, item.UserID, id)
}

func (db *DB) AdminSnoozeReminder(ctx context.Context, id uuid.UUID, until time.Time) (reminder.Reminder, error) {
	item, err := db.AdminGetReminder(ctx, id)
	if err != nil {
		return reminder.Reminder{}, err
	}
	return db.SnoozeReminder(ctx, item.UserID, id, until)
}
