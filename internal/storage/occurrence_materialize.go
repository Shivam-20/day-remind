package storage

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/recurrence"
	"github.com/shivam-20/day-remind/internal/reminder"
	"go.mongodb.org/mongo-driver/bson"
)

// MaterializeOccurrencesForReminder creates occurrence rows for the main due time and each offset.
func (db *DB) MaterializeOccurrencesForReminder(ctx context.Context, userID uuid.UUID, rem reminder.Reminder) error {
	if _, err := db.GetReminder(ctx, userID, rem.ID); err != nil {
		return err
	}
	_, err := db.CreateOccurrence(ctx, userID, OccurrenceCreateInput{
		ReminderID: rem.ID,
		FireAt:     rem.DueAt,
		Kind:       OccurrenceKindMain,
	})
	if err != nil {
		return err
	}
	offsets, err := db.ListOffsetsForReminder(ctx, userID, rem.ID)
	if err != nil {
		return err
	}
	for _, off := range offsets {
		fireAt := rem.DueAt.Add(-time.Duration(off.OffsetMinutes) * time.Minute)
		_, err := db.CreateOccurrence(ctx, userID, OccurrenceCreateInput{
			ReminderID:    rem.ID,
			FireAt:        fireAt,
			Kind:          OccurrenceKindOffset,
			OffsetMinutes: off.OffsetMinutes,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// RematerializeOccurrences deletes unfired occurrences and rebuilds from current due_at and offsets.
func (db *DB) RematerializeOccurrences(ctx context.Context, userID uuid.UUID, rem reminder.Reminder) error {
	_, err := db.collection("reminder_occurrences").DeleteMany(ctx, bson.M{
		"reminder_id": rem.ID,
		"user_id":     userID,
		"fired":       false,
	})
	if err != nil {
		return err
	}
	return db.MaterializeOccurrencesForReminder(ctx, userID, rem)
}

// ScheduleNextRecurrence advances a recurring reminder to its next due_at and rematerializes occurrences.
func (db *DB) ScheduleNextRecurrence(ctx context.Context, userID uuid.UUID, rem reminder.Reminder) (reminder.Reminder, error) {
	rule := strings.TrimSpace(rem.RecurrenceRule)
	if rule == "" {
		return rem, nil
	}
	next, err := recurrence.NextOccurrence(rule, rem.DueAt, rem.Timezone)
	if err != nil {
		return reminder.Reminder{}, err
	}
	if rem.RecurrenceUntil != nil && next.After(rem.RecurrenceUntil.UTC()) {
		return rem, nil
	}
	scheduled := reminder.StatusScheduled
	now := time.Now().UTC()
	res, err := db.collection("reminders").UpdateOne(ctx, bson.M{"_id": rem.ID, "user_id": userID}, bson.M{
		"$set": bson.M{
			"due_at":        next.UTC(),
			"status":        scheduled,
			"completed_at":  nil,
			"snoozed_until": nil,
			"updated_at":    now,
		},
	})
	if err != nil {
		return reminder.Reminder{}, err
	}
	if res.MatchedCount == 0 {
		return reminder.Reminder{}, ErrNotFound
	}
	updated, err := db.GetReminder(ctx, userID, rem.ID)
	if err != nil {
		return reminder.Reminder{}, err
	}
	if err := db.RematerializeOccurrences(ctx, userID, updated); err != nil {
		return reminder.Reminder{}, err
	}
	return updated, nil
}

// CountRemindersInList returns active reminders assigned to a list.
func (db *DB) CountRemindersInList(ctx context.Context, userID, listID uuid.UUID) (int64, error) {
	return db.collection("reminders").CountDocuments(ctx, bson.M{
		"user_id": userID,
		"list_id": listID,
		"status":  bson.M{"$in": []string{reminder.StatusScheduled, reminder.StatusPaused}},
	})
}
