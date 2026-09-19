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
	"go.mongodb.org/mongo-driver/mongo/options"
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

func (db *DB) ConsumeTelegramLinkToken(ctx context.Context, token, chatID, username, firstName string) error {
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
	now := time.Now().UTC()
	if _, err := db.collection("users").UpdateOne(ctx, bson.M{"_id": doc.UserID}, bson.M{
		"$set": bson.M{
			"telegram_chat_id":    chatID,
			"telegram_username":   strings.TrimSpace(username),
			"telegram_first_name": strings.TrimSpace(firstName),
			"telegram_linked_at":  now,
			"updated_at":          now,
		},
	}); err != nil {
		return err
	}
	_, _ = db.collection("telegram_link_tokens").DeleteOne(ctx, bson.M{"_id": token})
	return nil
}

// TelegramLinkInfo describes a user linked to the Telegram bot.
type TelegramLinkInfo struct {
	UserID      uuid.UUID
	Email       string
	DisplayName string
	ChatID      string
	Enabled     bool
	Username    string
	FirstName   string
	LinkedAt    *time.Time
}

// ListTelegramLinkedUsers returns users with a Telegram chat id configured.
func (db *DB) ListTelegramLinkedUsers(ctx context.Context, query string, limit, offset int) ([]TelegramLinkInfo, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	filter := bson.M{
		"notification_prefs": bson.M{
			"$elemMatch": bson.M{
				"channel": ChannelTelegram,
				"address": bson.M{"$ne": ""},
			},
		},
	}
	if q := strings.TrimSpace(query); q != "" {
		filter["$or"] = []bson.M{
			{"email": bson.M{"$regex": q, "$options": "i"}},
			{"display_name": bson.M{"$regex": q, "$options": "i"}},
			{"telegram_username": bson.M{"$regex": q, "$options": "i"}},
		}
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "telegram_linked_at", Value: -1}}).
		SetLimit(int64(limit)).
		SetSkip(int64(offset))
	cur, err := db.collection("users").Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	out := make([]TelegramLinkInfo, 0)
	for cur.Next(ctx) {
		var doc userDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		address, enabled := "", false
		for _, p := range doc.NotificationPrefs {
			if p.Channel == ChannelTelegram {
				address, enabled = p.Address, p.Enabled
				break
			}
		}
		if address == "" {
			address = doc.TelegramChatID
		}
		out = append(out, TelegramLinkInfo{
			UserID:      doc.ID,
			Email:       doc.Email,
			DisplayName: doc.DisplayName,
			ChatID:      address,
			Enabled:     enabled,
			Username:    doc.TelegramUsername,
			FirstName:   doc.TelegramFirstName,
			LinkedAt:    doc.TelegramLinkedAt,
		})
	}
	return out, cur.Err()
}

// UnlinkTelegram disables the Telegram notification pref and clears stored bot
// identity for the user.
func (db *DB) UnlinkTelegram(ctx context.Context, userID uuid.UUID) error {
	prefs, err := db.GetUserNotificationPrefs(ctx, userID)
	if err != nil {
		return err
	}
	for i := range prefs {
		if prefs[i].Channel == ChannelTelegram {
			prefs[i].Enabled = false
			prefs[i].Address = ""
		}
	}
	if _, err := db.UpdateUserNotificationPrefs(ctx, userID, prefs); err != nil {
		return err
	}
	_, err = db.collection("users").UpdateOne(ctx, bson.M{"_id": userID}, bson.M{
		"$unset": bson.M{
			"telegram_chat_id":   "",
			"telegram_linked_at": "",
		},
		"$set": bson.M{"updated_at": time.Now().UTC()},
	})
	return err
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
