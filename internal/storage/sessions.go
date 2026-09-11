package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const sessionTTL = 30 * 24 * time.Hour

func (db *DB) CreateSession(ctx context.Context, userID uuid.UUID) (string, error) {
	token, err := newSessionToken()
	if err != nil {
		return "", err
	}
	expires := time.Now().UTC().Add(sessionTTL)
	_, err = db.SQL.ExecContext(ctx, `
		INSERT INTO sessions (token, user_id, expires_at) VALUES ($1, $2, $3)`,
		token, userID, expires)
	if err != nil {
		return "", err
	}
	return token, nil
}

func (db *DB) DeleteSession(ctx context.Context, token string) error {
	_, err := db.SQL.ExecContext(ctx, `DELETE FROM sessions WHERE token = $1`, token)
	return err
}

func (db *DB) GetSessionUserID(ctx context.Context, token string) (uuid.UUID, error) {
	var userID uuid.UUID
	var expires time.Time
	err := db.SQL.QueryRowContext(ctx, `
		SELECT user_id, expires_at FROM sessions WHERE token = $1`, token).Scan(&userID, &expires)
	if err == sql.ErrNoRows {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, err
	}
	if time.Now().UTC().After(expires) {
		_, _ = db.SQL.ExecContext(ctx, `DELETE FROM sessions WHERE token = $1`, token)
		return uuid.Nil, ErrNotFound
	}
	return userID, nil
}

func newSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("session token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
