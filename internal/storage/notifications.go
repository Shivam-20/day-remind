package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	ChannelEmail    = "email"
	ChannelTelegram = "telegram"
)

type NotificationPref struct {
	Channel string
	Enabled bool
	Address string
}

type DeliverySettings struct {
	EmailEnabled       bool
	EmailFrom          string
	ResendAPIKey       string
	TelegramEnabled    bool
	TelegramBotToken   string
	UpdatedAt          time.Time
}

type DeliverySettingsUpdate struct {
	EmailEnabled     *bool
	EmailFrom        *string
	ResendAPIKey     *string
	TelegramEnabled  *bool
	TelegramBotToken *string
}

type deliverySettingsDoc struct {
	ID               string    `bson:"_id"`
	EmailEnabled     bool      `bson:"email_enabled"`
	EmailFrom        string    `bson:"email_from"`
	ResendAPIKey     string    `bson:"resend_api_key"`
	TelegramEnabled  bool      `bson:"telegram_enabled"`
	TelegramBotToken string    `bson:"telegram_bot_token"`
	UpdatedAt        time.Time `bson:"updated_at"`
}

const deliverySettingsID = "delivery"

func defaultNotificationPrefs(email string) []notificationPrefDoc {
	return []notificationPrefDoc{
		{Channel: ChannelEmail, Enabled: true, Address: email},
		{Channel: ChannelTelegram, Enabled: false, Address: ""},
	}
}

func prefFromDoc(d notificationPrefDoc) NotificationPref {
	return NotificationPref{Channel: d.Channel, Enabled: d.Enabled, Address: d.Address}
}

func prefsFromDocs(docs []notificationPrefDoc) []NotificationPref {
	out := make([]NotificationPref, 0, len(docs))
	for _, d := range docs {
		out = append(out, prefFromDoc(d))
	}
	return out
}

func (db *DB) GetUserNotificationPrefs(ctx context.Context, userID uuid.UUID) ([]NotificationPref, error) {
	var doc userDoc
	err := db.collection("users").FindOne(ctx, bson.M{"_id": userID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(doc.NotificationPrefs) == 0 {
		return prefsFromDocs(defaultNotificationPrefs(doc.Email)), nil
	}
	return normalizeUserPrefs(doc.Email, doc.NotificationPrefs), nil
}

func normalizeUserPrefs(accountEmail string, docs []notificationPrefDoc) []NotificationPref {
	byChannel := map[string]notificationPrefDoc{}
	for _, d := range docs {
		byChannel[d.Channel] = d
	}
	if _, ok := byChannel[ChannelEmail]; !ok {
		byChannel[ChannelEmail] = notificationPrefDoc{Channel: ChannelEmail, Enabled: true, Address: accountEmail}
	}
	if _, ok := byChannel[ChannelTelegram]; !ok {
		byChannel[ChannelTelegram] = notificationPrefDoc{Channel: ChannelTelegram, Enabled: false, Address: ""}
	}
	return []NotificationPref{
		prefFromDoc(byChannel[ChannelEmail]),
		prefFromDoc(byChannel[ChannelTelegram]),
	}
}

func (db *DB) UpdateUserNotificationPrefs(ctx context.Context, userID uuid.UUID, prefs []NotificationPref) ([]NotificationPref, error) {
	user, err := db.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	next, err := validateNotificationPrefs(user.Email, prefs)
	if err != nil {
		return nil, err
	}
	docs := make([]notificationPrefDoc, 0, len(next))
	for _, p := range next {
		docs = append(docs, notificationPrefDoc{Channel: p.Channel, Enabled: p.Enabled, Address: p.Address})
	}
	res, err := db.collection("users").UpdateOne(ctx, bson.M{"_id": userID}, bson.M{
		"$set": bson.M{
			"notification_prefs": docs,
			"updated_at":           time.Now().UTC(),
		},
	})
	if err != nil {
		return nil, err
	}
	if res.MatchedCount == 0 {
		return nil, ErrNotFound
	}
	return next, nil
}

func validateNotificationPrefs(accountEmail string, prefs []NotificationPref) ([]NotificationPref, error) {
	email := NotificationPref{Channel: ChannelEmail, Enabled: true, Address: accountEmail}
	telegram := NotificationPref{Channel: ChannelTelegram, Enabled: false, Address: ""}
	for _, p := range prefs {
		switch p.Channel {
		case ChannelEmail:
			addr := strings.TrimSpace(strings.ToLower(p.Address))
			if p.Enabled && addr == "" {
				return nil, fmt.Errorf("email alert address is required when enabled")
			}
			if addr != "" && !strings.Contains(addr, "@") {
				return nil, fmt.Errorf("invalid email alert address")
			}
			if addr == "" {
				addr = accountEmail
			}
			email = NotificationPref{Channel: ChannelEmail, Enabled: p.Enabled, Address: addr}
		case ChannelTelegram:
			addr := strings.TrimSpace(p.Address)
			if p.Enabled && addr == "" {
				return nil, fmt.Errorf("telegram chat id is required when enabled")
			}
			if addr != "" {
				for _, c := range addr {
					if c < '0' || c > '9' {
						if c != '-' && c != ' ' {
							return nil, fmt.Errorf("telegram chat id must be numeric")
						}
					}
				}
				addr = strings.ReplaceAll(strings.ReplaceAll(addr, " ", ""), "-", "")
			}
			telegram = NotificationPref{Channel: ChannelTelegram, Enabled: p.Enabled, Address: addr}
		}
	}
	return []NotificationPref{email, telegram}, nil
}

func (db *DB) GetDeliverySettings(ctx context.Context) (DeliverySettings, error) {
	var doc deliverySettingsDoc
	err := db.collection("system_settings").FindOne(ctx, bson.M{"_id": deliverySettingsID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return DeliverySettings{}, nil
	}
	if err != nil {
		return DeliverySettings{}, err
	}
	return deliveryFromDoc(doc), nil
}

func (db *DB) UpdateDeliverySettings(ctx context.Context, in DeliverySettingsUpdate) (DeliverySettings, error) {
	cur, err := db.GetDeliverySettings(ctx)
	if err != nil {
		return DeliverySettings{}, err
	}
	if in.EmailEnabled != nil {
		cur.EmailEnabled = *in.EmailEnabled
	}
	if in.EmailFrom != nil {
		cur.EmailFrom = strings.TrimSpace(*in.EmailFrom)
	}
	if in.ResendAPIKey != nil {
		key := strings.TrimSpace(*in.ResendAPIKey)
		if key != "" {
			cur.ResendAPIKey = key
		}
	}
	if in.TelegramEnabled != nil {
		cur.TelegramEnabled = *in.TelegramEnabled
	}
	if in.TelegramBotToken != nil {
		token := strings.TrimSpace(*in.TelegramBotToken)
		if token != "" {
			cur.TelegramBotToken = token
		}
	}
	cur.UpdatedAt = time.Now().UTC()
	doc := deliverySettingsDoc{
		ID:               deliverySettingsID,
		EmailEnabled:     cur.EmailEnabled,
		EmailFrom:        cur.EmailFrom,
		ResendAPIKey:     cur.ResendAPIKey,
		TelegramEnabled:  cur.TelegramEnabled,
		TelegramBotToken: cur.TelegramBotToken,
		UpdatedAt:        cur.UpdatedAt,
	}
	opts := options.Update().SetUpsert(true)
	_, err = db.collection("system_settings").UpdateOne(ctx, bson.M{"_id": deliverySettingsID}, bson.M{"$set": doc}, opts)
	if err != nil {
		return DeliverySettings{}, err
	}
	return cur, nil
}

func deliveryFromDoc(doc deliverySettingsDoc) DeliverySettings {
	return DeliverySettings{
		EmailEnabled:     doc.EmailEnabled,
		EmailFrom:        doc.EmailFrom,
		ResendAPIKey:     doc.ResendAPIKey,
		TelegramEnabled:  doc.TelegramEnabled,
		TelegramBotToken: doc.TelegramBotToken,
		UpdatedAt:        doc.UpdatedAt,
	}
}
