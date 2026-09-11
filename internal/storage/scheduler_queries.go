package storage

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/reminder"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (db *DB) ListDueReminders(ctx context.Context, now time.Time, limit int) ([]reminder.Reminder, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	filter := bson.M{
		"status": reminder.StatusScheduled,
		"due_at": bson.M{"$lte": now.UTC()},
		"$or": []bson.M{
			{"snoozed_until": bson.M{"$exists": false}},
			{"snoozed_until": nil},
			{"snoozed_until": bson.M{"$lte": now.UTC()}},
		},
	}
	cur, err := db.collection("reminders").Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "due_at", Value: 1}}).SetLimit(int64(limit)))
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

type UserNotifyState struct {
	NotificationsPaused      bool
	NotificationsPausedUntil *time.Time
	QuietStart               string
	QuietEnd                 string
	Prefs                    []NotificationPref
}

func (db *DB) GetUserNotifyState(ctx context.Context, userID uuid.UUID) (UserNotifyState, error) {
	var doc userDoc
	err := db.collection("users").FindOne(ctx, bson.M{"_id": userID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return UserNotifyState{}, ErrNotFound
	}
	if err != nil {
		return UserNotifyState{}, err
	}
	prefs := normalizeUserPrefs(doc.Email, doc.NotificationPrefs)
	return UserNotifyState{
		NotificationsPaused:      doc.NotificationsPaused,
		NotificationsPausedUntil: doc.NotificationsPausedUntil,
		QuietStart:               doc.QuietStart,
		QuietEnd:                 doc.QuietEnd,
		Prefs:                    prefs,
	}, nil
}
