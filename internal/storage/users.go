package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")
var ErrForbidden = errors.New("forbidden")

type User struct {
	ID                       uuid.UUID
	Email                    string
	PasswordHash             string
	DisplayName              string
	Timezone                 string
	Locale                   string
	IsAdmin                  bool
	NotificationsPaused      bool
	NotificationsPausedUntil *time.Time
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

type UserListFilter struct {
	Query  string
	Limit  int
	Offset int
}

type AdminUserUpdate struct {
	Email                    *string
	DisplayName              *string
	Timezone                 *string
	Locale                   *string
	IsAdmin                  *bool
	NotificationsPaused      *bool
	NotificationsPausedUntil *time.Time
	PasswordHash             *string
}

type notificationPrefDoc struct {
	Channel string `bson:"channel"`
	Enabled bool   `bson:"enabled"`
	Address string `bson:"address"`
}

const defaultMaxActiveReminders = 50

type userDoc struct {
	ID                       uuid.UUID             `bson:"_id"`
	Email                    string                `bson:"email"`
	PasswordHash             string                `bson:"password_hash"`
	DisplayName              string                `bson:"display_name"`
	Timezone                 string                `bson:"timezone"`
	Locale                   string                `bson:"locale"`
	NotificationsPaused      bool                  `bson:"notifications_paused"`
	NotificationsPausedUntil *time.Time            `bson:"notifications_paused_until,omitempty"`
	IsAdmin                  bool                  `bson:"is_admin"`
	QuietStart               string                `bson:"quiet_start,omitempty"`
	QuietEnd                 string                `bson:"quiet_end,omitempty"`
	MorningDigestEnabled     bool                  `bson:"morning_digest_enabled"`
	MorningDigestTime        string                `bson:"morning_digest_time,omitempty"`
	NotificationPrefs        []notificationPrefDoc `bson:"notification_prefs"`
	EmailIngestToken         string                `bson:"email_ingest_token,omitempty"`
	EscalationOrder          []string              `bson:"escalation_order,omitempty"`
	TOTPSecret               string                `bson:"totp_secret,omitempty"`
	TOTPEnabled              bool                  `bson:"totp_enabled"`
	MaxActiveReminders       int                   `bson:"max_active_reminders"`
	WeeklyBackupEnabled      bool                  `bson:"weekly_backup_enabled"`
	CreatedAt                time.Time             `bson:"created_at"`
	UpdatedAt                time.Time             `bson:"updated_at"`
}

func defaultEscalationOrder() []string {
	return []string{ChannelEmail, ChannelTelegram}
}

func userFromDoc(doc userDoc) User {
	return User{
		ID:                       doc.ID,
		Email:                    doc.Email,
		PasswordHash:             doc.PasswordHash,
		DisplayName:              doc.DisplayName,
		Timezone:                 doc.Timezone,
		Locale:                   doc.Locale,
		IsAdmin:                  doc.IsAdmin,
		NotificationsPaused:      doc.NotificationsPaused,
		NotificationsPausedUntil: doc.NotificationsPausedUntil,
		CreatedAt:                doc.CreatedAt,
		UpdatedAt:                doc.UpdatedAt,
	}
}

// CreateUser handles public signup (/api/auth/signup).
// It ALWAYS creates a regular (non-admin) user. Admin access is granted
// only via the hardcoded list in db.go (promoted on startup) — never
// through account creation.
func (db *DB) CreateUser(ctx context.Context, email, passwordHash, displayName, timezone string) (User, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return User{}, fmt.Errorf("email is required")
	}
	if timezone == "" {
		timezone = "Asia/Kolkata"
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
		IsAdmin:            false,
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

func (db *DB) GetUserByEmail(ctx context.Context, email string) (User, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	var doc userDoc
	err := db.collection("users").FindOne(ctx, bson.M{"email": email}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	return userFromDoc(doc), nil
}

func (db *DB) GetUserByID(ctx context.Context, id uuid.UUID) (User, error) {
	var doc userDoc
	err := db.collection("users").FindOne(ctx, bson.M{"_id": id}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	return userFromDoc(doc), nil
}

func (db *DB) SetNotificationsPaused(ctx context.Context, userID uuid.UUID, paused bool, until *time.Time) error {
	res, err := db.collection("users").UpdateOne(ctx, bson.M{"_id": userID}, bson.M{
		"$set": bson.M{
			"notifications_paused":       paused,
			"notifications_paused_until": until,
			"updated_at":                   time.Now().UTC(),
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
