package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/reminder"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type reminderDoc struct {
	ID                 uuid.UUID         `bson:"_id"`
	UserID             uuid.UUID         `bson:"user_id"`
	ListID             *uuid.UUID        `bson:"list_id,omitempty"`
	Title              string            `bson:"title"`
	Description        string            `bson:"description"`
	DueAt              time.Time         `bson:"due_at"`
	Timezone           string            `bson:"timezone"`
	Status             string            `bson:"status"`
	Priority           string            `bson:"priority"`
	Pinned             bool              `bson:"pinned"`
	TagIDs             []uuid.UUID       `bson:"tag_ids,omitempty"`
	RecurrenceRule     string            `bson:"recurrence_rule,omitempty"`
	SeriesID           *uuid.UUID        `bson:"series_id,omitempty"`
	RecurrenceUntil    *time.Time        `bson:"recurrence_until,omitempty"`
	ChannelOverrides   map[string]bool   `bson:"channel_overrides,omitempty"`
	RequireAllSubtasks bool              `bson:"require_all_subtasks"`
	AttachmentURL      string            `bson:"attachment_url,omitempty"`
	AssigneeID         *uuid.UUID        `bson:"assignee_id,omitempty"`
	GroupID            *uuid.UUID        `bson:"group_id,omitempty"`
	SnoozedUntil       *time.Time        `bson:"snoozed_until,omitempty"`
	PausedAt           *time.Time        `bson:"paused_at,omitempty"`
	ResumeAt           *time.Time        `bson:"resume_at,omitempty"`
	CompletedAt        *time.Time        `bson:"completed_at,omitempty"`
	CreatedAt          time.Time         `bson:"created_at"`
	UpdatedAt          time.Time         `bson:"updated_at"`
}

func reminderFromDoc(doc reminderDoc) reminder.Reminder {
	tagIDs := doc.TagIDs
	if tagIDs == nil {
		tagIDs = []uuid.UUID{}
	}
	channelOverrides := doc.ChannelOverrides
	if channelOverrides == nil {
		channelOverrides = map[string]bool{}
	}
	return reminder.Reminder{
		ID:                 doc.ID,
		UserID:             doc.UserID,
		ListID:             doc.ListID,
		Title:              doc.Title,
		Description:        doc.Description,
		DueAt:              doc.DueAt,
		Timezone:           doc.Timezone,
		Status:             doc.Status,
		Priority:           doc.Priority,
		Pinned:             doc.Pinned,
		TagIDs:             tagIDs,
		RecurrenceRule:     doc.RecurrenceRule,
		SeriesID:           doc.SeriesID,
		RecurrenceUntil:    doc.RecurrenceUntil,
		ChannelOverrides:   channelOverrides,
		RequireAllSubtasks: doc.RequireAllSubtasks,
		AttachmentURL:      doc.AttachmentURL,
		AssigneeID:         doc.AssigneeID,
		GroupID:            doc.GroupID,
		SnoozedUntil:       doc.SnoozedUntil,
		PausedAt:           doc.PausedAt,
		ResumeAt:           doc.ResumeAt,
		CompletedAt:        doc.CompletedAt,
		CreatedAt:          doc.CreatedAt,
		UpdatedAt:          doc.UpdatedAt,
	}
}

func (db *DB) CreateReminder(ctx context.Context, userID uuid.UUID, in reminder.CreateInput) (reminder.Reminder, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return reminder.Reminder{}, fmt.Errorf("title is required")
	}
	if in.DueAt.IsZero() {
		return reminder.Reminder{}, fmt.Errorf("due_at is required")
	}
	priority := in.Priority
	if priority == "" {
		priority = reminder.PriorityNormal
	}
	tz := in.Timezone
	if tz == "" {
		tz = "Asia/Kolkata"
	}
	tagIDs := in.TagIDs
	if tagIDs == nil {
		tagIDs = []uuid.UUID{}
	}
	channelOverrides := in.ChannelOverrides
	if channelOverrides == nil {
		channelOverrides = map[string]bool{}
	}
	now := time.Now().UTC()
	doc := reminderDoc{
		ID:                 uuid.New(),
		UserID:             userID,
		ListID:             in.ListID,
		Title:              title,
		Description:        strings.TrimSpace(in.Description),
		DueAt:              in.DueAt.UTC(),
		Timezone:           tz,
		Status:             reminder.StatusScheduled,
		Priority:           priority,
		TagIDs:             tagIDs,
		RecurrenceRule:     strings.TrimSpace(in.RecurrenceRule),
		SeriesID:           in.SeriesID,
		RecurrenceUntil:    in.RecurrenceUntil,
		ChannelOverrides:   channelOverrides,
		RequireAllSubtasks: in.RequireAllSubtasks,
		AttachmentURL:      strings.TrimSpace(in.AttachmentURL),
		AssigneeID:         in.AssigneeID,
		GroupID:            in.GroupID,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if doc.RecurrenceUntil != nil {
		doc.RecurrenceUntil = ptrUTC(doc.RecurrenceUntil)
	}
	_, err := db.collection("reminders").InsertOne(ctx, doc)
	if err != nil {
		return reminder.Reminder{}, err
	}
	return reminderFromDoc(doc), nil
}

func ptrUTC(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}

func (db *DB) GetReminder(ctx context.Context, userID, id uuid.UUID) (reminder.Reminder, error) {
	var doc reminderDoc
	err := db.collection("reminders").FindOne(ctx, bson.M{"_id": id, "user_id": userID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return reminder.Reminder{}, ErrNotFound
	}
	if err != nil {
		return reminder.Reminder{}, err
	}
	return reminderFromDoc(doc), nil
}

func (db *DB) ListReminders(ctx context.Context, f reminder.ListFilter) ([]reminder.Reminder, error) {
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	filter := bson.M{"user_id": f.UserID}
	if f.Status != "" {
		filter["status"] = f.Status
	}
	if f.ListID != nil {
		filter["list_id"] = *f.ListID
	}
	if f.Priority != "" {
		filter["priority"] = f.Priority
	}
	if f.TagID != nil {
		filter["tag_ids"] = *f.TagID
	}
	if f.From != nil || f.To != nil {
		dueFilter := bson.M{}
		if f.From != nil {
			dueFilter["$gte"] = f.From.UTC()
		}
		if f.To != nil {
			dueFilter["$lte"] = f.To.UTC()
		}
		filter["due_at"] = dueFilter
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		filter["$or"] = []bson.M{
			{"title": bson.M{"$regex": q, "$options": "i"}},
			{"description": bson.M{"$regex": q, "$options": "i"}},
		}
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "due_at", Value: 1}}).
		SetLimit(int64(limit))
	cur, err := db.collection("reminders").Find(ctx, filter, opts)
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

func (db *DB) UpdateReminder(ctx context.Context, userID, id uuid.UUID, in reminder.UpdateInput) (reminder.Reminder, error) {
	cur, err := db.GetReminder(ctx, userID, id)
	if err != nil {
		return reminder.Reminder{}, err
	}
	title := cur.Title
	if in.Title != nil {
		title = strings.TrimSpace(*in.Title)
		if title == "" {
			return reminder.Reminder{}, fmt.Errorf("title is required")
		}
	}
	desc := cur.Description
	if in.Description != nil {
		desc = strings.TrimSpace(*in.Description)
	}
	dueAt := cur.DueAt
	if in.DueAt != nil {
		dueAt = in.DueAt.UTC()
	}
	tz := cur.Timezone
	if in.Timezone != nil {
		tz = strings.TrimSpace(*in.Timezone)
	}
	priority := cur.Priority
	if in.Priority != nil {
		priority = *in.Priority
	}
	status := cur.Status
	if in.Status != nil {
		status = *in.Status
	}
	listID := cur.ListID
	if in.ListID != nil {
		listID = in.ListID
	}
	tagIDs := cur.TagIDs
	if in.TagIDs != nil {
		tagIDs = *in.TagIDs
	}
	recurrenceRule := cur.RecurrenceRule
	if in.RecurrenceRule != nil {
		recurrenceRule = strings.TrimSpace(*in.RecurrenceRule)
	}
	seriesID := cur.SeriesID
	if in.SeriesID != nil {
		seriesID = in.SeriesID
	}
	recurrenceUntil := cur.RecurrenceUntil
	if in.RecurrenceUntil != nil {
		recurrenceUntil = ptrUTC(in.RecurrenceUntil)
	}
	channelOverrides := cur.ChannelOverrides
	if in.ChannelOverrides != nil {
		channelOverrides = *in.ChannelOverrides
	}
	requireAllSubtasks := cur.RequireAllSubtasks
	if in.RequireAllSubtasks != nil {
		requireAllSubtasks = *in.RequireAllSubtasks
	}
	attachmentURL := cur.AttachmentURL
	if in.AttachmentURL != nil {
		attachmentURL = strings.TrimSpace(*in.AttachmentURL)
	}
	assigneeID := cur.AssigneeID
	if in.AssigneeID != nil {
		assigneeID = in.AssigneeID
	}
	groupID := cur.GroupID
	if in.GroupID != nil {
		groupID = in.GroupID
	}
	completedAt := cur.CompletedAt
	if status == reminder.StatusDone && cur.CompletedAt == nil {
		now := time.Now().UTC()
		completedAt = &now
	}

	set := bson.M{
		"title":                 title,
		"description":           desc,
		"due_at":                dueAt,
		"timezone":              tz,
		"priority":              priority,
		"status":                status,
		"list_id":               listID,
		"tag_ids":               tagIDs,
		"recurrence_rule":       recurrenceRule,
		"series_id":             seriesID,
		"recurrence_until":      recurrenceUntil,
		"channel_overrides":     channelOverrides,
		"require_all_subtasks":  requireAllSubtasks,
		"attachment_url":        attachmentURL,
		"assignee_id":           assigneeID,
		"group_id":              groupID,
		"completed_at":          completedAt,
		"updated_at":            time.Now().UTC(),
	}
	res, err := db.collection("reminders").UpdateOne(ctx, bson.M{"_id": id, "user_id": userID}, bson.M{"$set": set})
	if err != nil {
		return reminder.Reminder{}, err
	}
	if res.MatchedCount == 0 {
		return reminder.Reminder{}, ErrNotFound
	}
	return db.GetReminder(ctx, userID, id)
}

func (db *DB) DeleteReminder(ctx context.Context, userID, id uuid.UUID) error {
	res, err := db.collection("reminders").DeleteOne(ctx, bson.M{"_id": id, "user_id": userID})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (db *DB) PauseReminder(ctx context.Context, userID, id uuid.UUID, resumeAt *time.Time) (reminder.Reminder, error) {
	now := time.Now().UTC()
	res, err := db.collection("reminders").UpdateOne(ctx, bson.M{
		"_id":     id,
		"user_id": userID,
		"status":  bson.M{"$in": []string{reminder.StatusScheduled, reminder.StatusPaused}},
	}, bson.M{"$set": bson.M{
		"status":     reminder.StatusPaused,
		"paused_at":  now,
		"resume_at":  resumeAt,
		"updated_at": now,
	}})
	if err != nil {
		return reminder.Reminder{}, err
	}
	if res.MatchedCount == 0 {
		return reminder.Reminder{}, ErrNotFound
	}
	return db.GetReminder(ctx, userID, id)
}

func (db *DB) ResumeReminder(ctx context.Context, userID, id uuid.UUID) (reminder.Reminder, error) {
	now := time.Now().UTC()
	res, err := db.collection("reminders").UpdateOne(ctx, bson.M{
		"_id":     id,
		"user_id": userID,
		"status":  reminder.StatusPaused,
	}, bson.M{"$set": bson.M{
		"status":     reminder.StatusScheduled,
		"paused_at":  nil,
		"resume_at":  nil,
		"updated_at": now,
	}})
	if err != nil {
		return reminder.Reminder{}, err
	}
	if res.MatchedCount == 0 {
		return reminder.Reminder{}, ErrNotFound
	}
	return db.GetReminder(ctx, userID, id)
}

func (db *DB) SnoozeReminder(ctx context.Context, userID, id uuid.UUID, until time.Time) (reminder.Reminder, error) {
	res, err := db.collection("reminders").UpdateOne(ctx, bson.M{"_id": id, "user_id": userID}, bson.M{"$set": bson.M{
		"snoozed_until": until.UTC(),
		"updated_at":    time.Now().UTC(),
	}})
	if err != nil {
		return reminder.Reminder{}, err
	}
	if res.MatchedCount == 0 {
		return reminder.Reminder{}, ErrNotFound
	}
	return db.GetReminder(ctx, userID, id)
}
