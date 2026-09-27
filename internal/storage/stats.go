package storage

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/reminder"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type DashboardStats struct {
	Overdue    int64
	Today      int64
	Upcoming7d int64
	DoneCount  int64
	NextFive   []reminder.Reminder
}

func (db *DB) GetDashboardStats(ctx context.Context, userID uuid.UUID) (DashboardStats, error) {
	now := time.Now().UTC()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := startOfDay.Add(24 * time.Hour)
	endOfWeek := startOfDay.Add(7 * 24 * time.Hour)

	activeStatuses := []string{reminder.StatusScheduled, reminder.StatusPaused}

	overdue, err := db.collection("reminders").CountDocuments(ctx, bson.M{
		"user_id": userID,
		"status":  bson.M{"$in": activeStatuses},
		"due_at":  bson.M{"$lt": startOfDay},
	})
	if err != nil {
		return DashboardStats{}, err
	}

	today, err := db.collection("reminders").CountDocuments(ctx, bson.M{
		"user_id": userID,
		"status":  bson.M{"$in": activeStatuses},
		"due_at":  bson.M{"$gte": startOfDay, "$lt": endOfDay},
	})
	if err != nil {
		return DashboardStats{}, err
	}

	upcoming7d, err := db.collection("reminders").CountDocuments(ctx, bson.M{
		"user_id": userID,
		"status":  bson.M{"$in": activeStatuses},
		"due_at":  bson.M{"$gte": endOfDay, "$lt": endOfWeek},
	})
	if err != nil {
		return DashboardStats{}, err
	}

	doneCount, err := db.collection("reminders").CountDocuments(ctx, bson.M{
		"user_id": userID,
		"status":  reminder.StatusDone,
	})
	if err != nil {
		return DashboardStats{}, err
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "due_at", Value: 1}}).
		SetLimit(5)
	cur, err := db.collection("reminders").Find(ctx, bson.M{
		"user_id": userID,
		"status":  reminder.StatusScheduled,
		"due_at":  bson.M{"$gte": now},
	}, opts)
	if err != nil {
		return DashboardStats{}, err
	}
	defer cur.Close(ctx)

	var nextFive []reminder.Reminder
	for cur.Next(ctx) {
		var doc reminderDoc
		if err := cur.Decode(&doc); err != nil {
			return DashboardStats{}, err
		}
		nextFive = append(nextFive, reminderFromDoc(doc))
	}
	if err := cur.Err(); err != nil {
		return DashboardStats{}, err
	}

	return DashboardStats{
		Overdue:    overdue,
		Today:      today,
		Upcoming7d: upcoming7d,
		DoneCount:  doneCount,
		NextFive:   nextFive,
	}, nil
}
