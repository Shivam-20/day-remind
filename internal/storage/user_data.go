package storage

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// UserSettings holds exportable user preferences.
type UserSettings struct {
	QuietStart            string
	QuietEnd              string
	MorningDigestEnabled  bool
	MorningDigestTime     string
}

// ListRecord is a reminder list for export/import.
type ListRecord struct {
	ID     uuid.UUID
	Name   string
	Color  string
	Paused bool
}

// TagRecord is a tag for export/import.
type TagRecord struct {
	ID   uuid.UUID
	Name string
}

// SubtaskRecord is a checklist item on a reminder.
type SubtaskRecord struct {
	ID    uuid.UUID
	Title string
	Done  bool
	Sort  int
}

// OffsetRecord is an advance alert offset.
type OffsetRecord struct {
	OffsetMinutes int
}

// ReminderRecord is the full reminder shape used for export/import.
type ReminderRecord struct {
	ID               uuid.UUID
	Title            string
	Description      string
	DueAt            time.Time
	Timezone         string
	Status           string
	Priority         string
	ListID           *uuid.UUID
	TagIDs           []uuid.UUID
	RecurrenceRule   string
	Offsets          []OffsetRecord
	Subtasks         []SubtaskRecord
	ChannelOverrides map[string]bool
}

type exportReminderDoc struct {
	reminderDoc
	RecurrenceRule   string          `bson:"recurrence_rule,omitempty"`
	TagIDs           []uuid.UUID     `bson:"tag_ids,omitempty"`
	ChannelOverrides map[string]bool `bson:"channel_overrides,omitempty"`
}

// DigestUser is a user eligible for morning digest delivery.
type DigestUser struct {
	ID                  uuid.UUID
	Email               string
	Timezone            string
	MorningDigestTime   string
	NotificationPrefs   []NotificationPref
}

// LoadUserSettings returns exportable settings for a user.
func (db *DB) LoadUserSettings(ctx context.Context, userID uuid.UUID) (UserSettings, error) {
	var doc userDoc
	err := db.collection("users").FindOne(ctx, bson.M{"_id": userID}).Decode(&doc)
	if err != nil {
		return UserSettings{}, err
	}
	return UserSettings{
		QuietStart:           doc.QuietStart,
		QuietEnd:             doc.QuietEnd,
		MorningDigestEnabled: doc.MorningDigestEnabled,
		MorningDigestTime:    doc.MorningDigestTime,
	}, nil
}

// ListUserLists returns all lists for a user.
func (db *DB) ListUserLists(ctx context.Context, userID uuid.UUID) ([]ListRecord, error) {
	cur, err := db.collection("lists").Find(ctx, bson.M{"user_id": userID},
		options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []ListRecord
	for cur.Next(ctx) {
		var doc listDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, ListRecord{ID: doc.ID, Name: doc.Name, Color: doc.Color, Paused: doc.Paused})
	}
	return out, cur.Err()
}

// ListUserTags returns all tags for a user.
func (db *DB) ListUserTags(ctx context.Context, userID uuid.UUID) ([]TagRecord, error) {
	cur, err := db.collection("tags").Find(ctx, bson.M{"user_id": userID},
		options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []TagRecord
	for cur.Next(ctx) {
		var doc tagDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, TagRecord{ID: doc.ID, Name: doc.Name})
	}
	return out, cur.Err()
}

// ListUserReminderRecords returns full reminder records for export.
func (db *DB) ListUserReminderRecords(ctx context.Context, userID uuid.UUID) ([]ReminderRecord, error) {
	cur, err := db.collection("reminders").Find(ctx, bson.M{"user_id": userID},
		options.Find().SetSort(bson.D{{Key: "due_at", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []ReminderRecord
	for cur.Next(ctx) {
		var doc exportReminderDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		rec := ReminderRecord{
			ID:               doc.ID,
			Title:            doc.Title,
			Description:      doc.Description,
			DueAt:            doc.DueAt,
			Timezone:         doc.Timezone,
			Status:           doc.Status,
			Priority:         doc.Priority,
			ListID:           doc.ListID,
			TagIDs:           doc.TagIDs,
			RecurrenceRule:   doc.RecurrenceRule,
			ChannelOverrides: doc.ChannelOverrides,
		}
		subtasks, err := db.listSubtasksForReminder(ctx, userID, doc.ID)
		if err != nil {
			return nil, err
		}
		offsets, err := db.listOffsetsForReminder(ctx, userID, doc.ID)
		if err != nil {
			return nil, err
		}
		rec.Subtasks = subtasks
		rec.Offsets = offsets
		out = append(out, rec)
	}
	return out, cur.Err()
}

func (db *DB) listSubtasksForReminder(ctx context.Context, userID, reminderID uuid.UUID) ([]SubtaskRecord, error) {
	cur, err := db.collection("subtasks").Find(ctx, bson.M{"user_id": userID, "reminder_id": reminderID},
		options.Find().SetSort(bson.D{{Key: "sort_order", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []SubtaskRecord
	for cur.Next(ctx) {
		var doc subtaskDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, SubtaskRecord{ID: doc.ID, Title: doc.Title, Done: doc.Done, Sort: doc.SortOrder})
	}
	return out, cur.Err()
}

func (db *DB) listOffsetsForReminder(ctx context.Context, userID, reminderID uuid.UUID) ([]OffsetRecord, error) {
	cur, err := db.collection("reminder_offsets").Find(ctx, bson.M{"user_id": userID, "reminder_id": reminderID})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []OffsetRecord
	for cur.Next(ctx) {
		var doc offsetDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, OffsetRecord{OffsetMinutes: doc.OffsetMinutes})
	}
	return out, cur.Err()
}

// ListDigestUsers returns users with morning digest enabled.
func (db *DB) ListDigestUsers(ctx context.Context) ([]DigestUser, error) {
	cur, err := db.collection("users").Find(ctx, bson.M{"morning_digest_enabled": true})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []DigestUser
	for cur.Next(ctx) {
		var doc userDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		prefs := normalizeUserPrefs(doc.Email, doc.NotificationPrefs)
		out = append(out, DigestUser{
			ID:                doc.ID,
			Email:             doc.Email,
			Timezone:          doc.Timezone,
			MorningDigestTime: doc.MorningDigestTime,
			NotificationPrefs: prefs,
		})
	}
	return out, cur.Err()
}

// ListRemindersForDay returns scheduled reminders due on a local calendar day.
func (db *DB) ListRemindersForDay(ctx context.Context, userID uuid.UUID, dayStart, dayEnd time.Time) ([]reminderDoc, error) {
	filter := bson.M{
		"user_id": userID,
		"status":  bson.M{"$in": []string{"scheduled", "paused"}},
		"due_at":  bson.M{"$gte": dayStart.UTC(), "$lt": dayEnd.UTC()},
	}
	cur, err := db.collection("reminders").Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "due_at", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []reminderDoc
	for cur.Next(ctx) {
		var doc reminderDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, doc)
	}
	return out, cur.Err()
}

// DeleteUserExportData removes lists, tags, reminders, subtasks, and offsets for replace imports.
func (db *DB) DeleteUserExportData(ctx context.Context, userID uuid.UUID) error {
	collections := []string{"lists", "tags", "reminders", "subtasks", "reminder_offsets"}
	for _, name := range collections {
		_, err := db.collection(name).DeleteMany(ctx, bson.M{"user_id": userID})
		if err != nil {
			return err
		}
	}
	return nil
}

// UpsertListRecord inserts or updates a list during import.
func (db *DB) UpsertListRecord(ctx context.Context, userID uuid.UUID, rec ListRecord) error {
	now := time.Now().UTC()
	doc := listDoc{
		ID:        rec.ID,
		UserID:    userID,
		Name:      rec.Name,
		Color:     rec.Color,
		Paused:    rec.Paused,
		UpdatedAt: now,
	}
	_, err := db.collection("lists").UpdateOne(ctx, bson.M{"_id": rec.ID, "user_id": userID},
		bson.M{"$set": doc, "$setOnInsert": bson.M{"created_at": now}},
		options.Update().SetUpsert(true))
	return err
}

// UpsertTagRecord inserts or updates a tag during import.
func (db *DB) UpsertTagRecord(ctx context.Context, userID uuid.UUID, rec TagRecord) error {
	doc := tagDoc{ID: rec.ID, UserID: userID, Name: rec.Name}
	_, err := db.collection("tags").UpdateOne(ctx, bson.M{"_id": rec.ID, "user_id": userID},
		bson.M{"$set": doc}, options.Update().SetUpsert(true))
	return err
}

// ReminderExists checks whether a reminder id already belongs to the user.
func (db *DB) ReminderExists(ctx context.Context, userID, id uuid.UUID) (bool, error) {
	n, err := db.collection("reminders").CountDocuments(ctx, bson.M{"_id": id, "user_id": userID})
	return n > 0, err
}

// UpsertReminderRecord inserts or updates a reminder during import.
func (db *DB) UpsertReminderRecord(ctx context.Context, userID uuid.UUID, rec ReminderRecord) error {
	now := time.Now().UTC()
	doc := bson.M{
		"_id":               rec.ID,
		"user_id":           userID,
		"title":             rec.Title,
		"description":       rec.Description,
		"due_at":            rec.DueAt.UTC(),
		"timezone":          rec.Timezone,
		"status":            rec.Status,
		"priority":          rec.Priority,
		"list_id":           rec.ListID,
		"tag_ids":           rec.TagIDs,
		"recurrence_rule":   rec.RecurrenceRule,
		"channel_overrides": rec.ChannelOverrides,
		"updated_at":        now,
	}
	_, err := db.collection("reminders").UpdateOne(ctx, bson.M{"_id": rec.ID, "user_id": userID},
		bson.M{"$set": doc, "$setOnInsert": bson.M{"created_at": now}},
		options.Update().SetUpsert(true))
	if err != nil {
		return err
	}
	_, _ = db.collection("subtasks").DeleteMany(ctx, bson.M{"user_id": userID, "reminder_id": rec.ID})
	for _, st := range rec.Subtasks {
		stDoc := subtaskDoc{
			ID:         st.ID,
			ReminderID: rec.ID,
			UserID:     userID,
			Title:      st.Title,
			Done:       st.Done,
			SortOrder:  st.Sort,
		}
		if stDoc.ID == uuid.Nil {
			stDoc.ID = uuid.New()
		}
		if _, err := db.collection("subtasks").InsertOne(ctx, stDoc); err != nil {
			return err
		}
	}
	_, _ = db.collection("reminder_offsets").DeleteMany(ctx, bson.M{"user_id": userID, "reminder_id": rec.ID})
	for _, off := range rec.Offsets {
		offDoc := offsetDoc{
			ID:            uuid.New(),
			ReminderID:    rec.ID,
			UserID:        userID,
			OffsetMinutes: off.OffsetMinutes,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if _, err := db.collection("reminder_offsets").InsertOne(ctx, offDoc); err != nil {
			return err
		}
	}
	return nil
}
