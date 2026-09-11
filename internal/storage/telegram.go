package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const telegramLinkTTL = 15 * time.Minute

type TelegramLinkToken struct {
	Token     string
	UserID    uuid.UUID
	ExpiresAt time.Time
}

func (db *DB) CreateTelegramLinkToken(ctx context.Context, userID uuid.UUID) (TelegramLinkToken, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return TelegramLinkToken{}, fmt.Errorf("token: %w", err)
	}
	token := hex.EncodeToString(b)
	now := time.Now().UTC()
	doc := struct {
		Token     string    `bson:"_id"`
		UserID    uuid.UUID `bson:"user_id"`
		ExpiresAt time.Time `bson:"expires_at"`
		CreatedAt time.Time `bson:"created_at"`
	}{
		Token:     token,
		UserID:    userID,
		ExpiresAt: now.Add(telegramLinkTTL),
		CreatedAt: now,
	}
	_, err := db.collection("telegram_link_tokens").InsertOne(ctx, doc)
	if err != nil {
		return TelegramLinkToken{}, err
	}
	return TelegramLinkToken{Token: token, UserID: userID, ExpiresAt: doc.ExpiresAt}, nil
}

func (db *DB) ConsumeTelegramLinkToken(ctx context.Context, token, chatID string) error {
	token = strings.TrimSpace(token)
	chatID = strings.TrimSpace(chatID)
	var doc struct {
		UserID    uuid.UUID `bson:"user_id"`
		ExpiresAt time.Time `bson:"expires_at"`
	}
	err := db.collection("telegram_link_tokens").FindOne(ctx, bson.M{"_id": token}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if time.Now().UTC().After(doc.ExpiresAt) {
		_, _ = db.collection("telegram_link_tokens").DeleteOne(ctx, bson.M{"_id": token})
		return ErrNotFound
	}
	prefs, err := db.GetUserNotificationPrefs(ctx, doc.UserID)
	if err != nil {
		return err
	}
	for i := range prefs {
		if prefs[i].Channel == ChannelTelegram {
			prefs[i].Address = chatID
			prefs[i].Enabled = true
		}
	}
	if _, err := db.UpdateUserNotificationPrefs(ctx, doc.UserID, prefs); err != nil {
		return err
	}
	_, _ = db.collection("telegram_link_tokens").DeleteOne(ctx, bson.M{"_id": token})
	return nil
}

func (db *DB) GetUserByTelegramChatID(ctx context.Context, chatID string) (User, error) {
	chatID = strings.TrimSpace(chatID)
	var doc userDoc
	err := db.collection("users").FindOne(ctx, bson.M{
		"notification_prefs": bson.M{
			"$elemMatch": bson.M{
				"channel": ChannelTelegram,
				"address": chatID,
				"enabled": true,
			},
		},
	}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	return userFromDoc(doc), nil
}
