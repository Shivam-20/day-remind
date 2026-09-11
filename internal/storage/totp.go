package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/auth"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type TOTPStatus struct {
	Enabled bool
}

// BeginTOTPSetup stores a pending TOTP secret (not yet enabled).
func (db *DB) BeginTOTPSetup(ctx context.Context, userID uuid.UUID) (string, error) {
	secret, err := auth.GenerateTOTPSecret()
	if err != nil {
		return "", err
	}
	res, err := db.collection("users").UpdateOne(ctx, bson.M{"_id": userID}, bson.M{
		"$set": bson.M{
			"totp_secret":  secret,
			"totp_enabled": false,
			"updated_at":   time.Now().UTC(),
		},
	})
	if err != nil {
		return "", err
	}
	if res.MatchedCount == 0 {
		return "", ErrNotFound
	}
	return secret, nil
}

// EnableTOTP verifies a code and enables 2FA for the user.
func (db *DB) EnableTOTP(ctx context.Context, userID uuid.UUID, code string) error {
	var doc userDoc
	err := db.collection("users").FindOne(ctx, bson.M{"_id": userID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if doc.TOTPSecret == "" {
		return fmt.Errorf("call setup first")
	}
	if !auth.ValidateTOTPWithWindow(doc.TOTPSecret, code, 1) {
		return fmt.Errorf("invalid verification code")
	}
	_, err = db.collection("users").UpdateOne(ctx, bson.M{"_id": userID}, bson.M{
		"$set": bson.M{
			"totp_enabled": true,
			"updated_at":   time.Now().UTC(),
		},
	})
	return err
}

// DisableTOTP turns off 2FA after verifying a code.
func (db *DB) DisableTOTP(ctx context.Context, userID uuid.UUID, code string) error {
	var doc userDoc
	err := db.collection("users").FindOne(ctx, bson.M{"_id": userID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !doc.TOTPEnabled {
		return nil
	}
	if !auth.ValidateTOTPWithWindow(doc.TOTPSecret, code, 1) {
		return fmt.Errorf("invalid verification code")
	}
	_, err = db.collection("users").UpdateOne(ctx, bson.M{"_id": userID}, bson.M{
		"$set": bson.M{
			"totp_enabled": false,
			"totp_secret":  "",
			"updated_at":   time.Now().UTC(),
		},
	})
	return err
}

// GetTOTPStatus returns whether 2FA is enabled for the user.
func (db *DB) GetTOTPStatus(ctx context.Context, userID uuid.UUID) (TOTPStatus, error) {
	var doc userDoc
	err := db.collection("users").FindOne(ctx, bson.M{"_id": userID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return TOTPStatus{}, ErrNotFound
	}
	if err != nil {
		return TOTPStatus{}, err
	}
	return TOTPStatus{Enabled: doc.TOTPEnabled}, nil
}
