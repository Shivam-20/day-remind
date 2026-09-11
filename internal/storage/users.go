package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")

type User struct {
	ID                      uuid.UUID
	Email                   string
	PasswordHash            string
	DisplayName             string
	Timezone                string
	Locale                  string
	NotificationsPaused     bool
	NotificationsPausedUntil *time.Time
	CreatedAt               time.Time
}

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
	_, err := db.SQL.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, display_name, timezone, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $6)`,
		id, email, passwordHash, strings.TrimSpace(displayName), timezone, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			return User{}, ErrConflict
		}
		return User{}, err
	}
	if err := db.seedDefaultNotificationPrefs(ctx, id, email); err != nil {
		return User{}, err
	}
	return User{
		ID:           id,
		Email:        email,
		PasswordHash: passwordHash,
		DisplayName:  strings.TrimSpace(displayName),
		Timezone:     timezone,
		Locale:       "en",
		CreatedAt:    now,
	}, nil
}

func (db *DB) seedDefaultNotificationPrefs(ctx context.Context, userID uuid.UUID, email string) error {
	_, err := db.SQL.ExecContext(ctx, `
		INSERT INTO user_notification_prefs (user_id, channel, enabled, address)
		VALUES ($1, 'email', TRUE, $2)
		ON CONFLICT (user_id, channel) DO NOTHING`, userID, email)
	return err
}

func (db *DB) GetUserByEmail(ctx context.Context, email string) (User, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	var u User
	var pausedUntil sql.NullTime
	err := db.SQL.QueryRowContext(ctx, `
		SELECT id, email, password_hash, display_name, timezone, locale,
		       notifications_paused, notifications_paused_until, created_at
		FROM users WHERE email = $1`, email).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.Timezone, &u.Locale,
		&u.NotificationsPaused, &pausedUntil, &u.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	if pausedUntil.Valid {
		u.NotificationsPausedUntil = &pausedUntil.Time
	}
	return u, nil
}

func (db *DB) GetUserByID(ctx context.Context, id uuid.UUID) (User, error) {
	var u User
	var pausedUntil sql.NullTime
	err := db.SQL.QueryRowContext(ctx, `
		SELECT id, email, password_hash, display_name, timezone, locale,
		       notifications_paused, notifications_paused_until, created_at
		FROM users WHERE id = $1`, id).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.Timezone, &u.Locale,
		&u.NotificationsPaused, &pausedUntil, &u.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	if pausedUntil.Valid {
		u.NotificationsPausedUntil = &pausedUntil.Time
	}
	return u, nil
}

func (db *DB) SetNotificationsPaused(ctx context.Context, userID uuid.UUID, paused bool, until *time.Time) error {
	_, err := db.SQL.ExecContext(ctx, `
		UPDATE users SET notifications_paused = $2, notifications_paused_until = $3, updated_at = NOW()
		WHERE id = $1`, userID, paused, until)
	return err
}
