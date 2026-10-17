package storage

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/reminder"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type reminderDoc struct {
	ID           uuid.UUID  `bson:"_id"`
	UserID       uuid.UUID  `bson:"user_id"`
	ListID       *uuid.UUID `bson:"list_id,omitempty"`
	Title        string     `bson:"title"`
	Description  string     `bson:"description"`
	DueAt        time.Time  `bson:"due_at"`
	Timezone     string     `bson:"timezone"`
	Status       string     `bson:"status"`
	Priority     string     `bson:"priority"`
	Pinned       bool       `bson:"pinned"`
	SnoozedUntil *time.Time `bson:"snoozed_until,omitempty"`
	PausedAt     *time.Time `bson:"paused_at,omitempty"`
	ResumeAt     *time.Time `bson:"resume_at,omitempty"`
	CompletedAt  *time.Time `bson:"completed_at,omitempty"`
	// Delivery claims. A non-nil value means that notification has already
	// been dispatched and must not be sent again.
	SentDueAt     *time.Time `bson:"sent_due_at,omitempty"`
	SentAdvanceAt *time.Time `bson:"sent_advance_at,omitempty"`
	Recurrence    string     `bson:"recurrence,omitempty"`
	CreatedAt     time.Time  `bson:"created_at"`
	UpdatedAt     time.Time  `bson:"updated_at"`
}

func reminderFromDoc(doc reminderDoc) reminder.Reminder {
	return reminder.Reminder{
		ID:            doc.ID,
		UserID:        doc.UserID,
		ListID:        doc.ListID,
		Title:         doc.Title,
		Description:   doc.Description,
		DueAt:         doc.DueAt,
		Timezone:      doc.Timezone,
		Status:        doc.Status,
		Priority:      doc.Priority,
		Pinned:        doc.Pinned,
		SnoozedUntil:  doc.SnoozedUntil,
		PausedAt:      doc.PausedAt,
		ResumeAt:      doc.ResumeAt,
		CompletedAt:   doc.CompletedAt,
		SentDueAt:     doc.SentDueAt,
		SentAdvanceAt: doc.SentAdvanceAt,
		Recurrence:    doc.Recurrence,
		CreatedAt:     doc.CreatedAt,
		UpdatedAt:     doc.UpdatedAt,
	}
}

func (db *DB) CreateReminder(ctx context.Context, userID uuid.UUID, in reminder.CreateInput) (reminder.Reminder, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return reminder.Reminder{}, fmt.Errorf("%w: title is required", ErrInvalid)
	}
	if in.DueAt.IsZero() {
		return reminder.Reminder{}, fmt.Errorf("%w: due_at is required", ErrInvalid)
	}
	priority := in.Priority
	if priority == "" {
		priority = reminder.PriorityNormal
	}
	if !reminder.ValidPriority(priority) {
		return reminder.Reminder{}, fmt.Errorf("%w: unknown priority %q", ErrInvalid, priority)
	}
	recur := strings.TrimSpace(in.Recurrence)
	if !reminder.ValidRecurrence(recur) {
		return reminder.Reminder{}, fmt.Errorf("%w: unknown recurrence %q", ErrInvalid, recur)
	}
	tz := strings.TrimSpace(in.Timezone)
	if tz == "" {
		tz = defaultTimezone
	}
	now := time.Now().UTC()
	doc := reminderDoc{
		ID:          uuid.New(),
		UserID:      userID,
		ListID:      in.ListID,
		Title:       title,
		Description: strings.TrimSpace(in.Description),
		DueAt:       in.DueAt.UTC(),
		Timezone:    tz,
		Status:      reminder.StatusScheduled,
		Priority:    priority,
		Recurrence:  recur,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, err := db.collection("reminders").InsertOne(ctx, doc)
	if err != nil {
		return reminder.Reminder{}, err
	}
	return reminderFromDoc(doc), nil
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
		// Treat the query as literal text: an unescaped user string is a valid
		// regex, so "(((" would error and "(a+)+$" would pin the server.
		pattern := regexp.QuoteMeta(q)
		filter["$or"] = []bson.M{
			{"title": bson.M{"$regex": pattern, "$options": "i"}},
			{"description": bson.M{"$regex": pattern, "$options": "i"}},
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
			return reminder.Reminder{}, fmt.Errorf("%w: title is required", ErrInvalid)
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
		if tz == "" {
			tz = defaultTimezone
		}
	}
	priority := cur.Priority
	if in.Priority != nil {
		priority = *in.Priority
		if !reminder.ValidPriority(priority) {
			return reminder.Reminder{}, fmt.Errorf("%w: unknown priority %q", ErrInvalid, priority)
		}
	}
	status := cur.Status
	if in.Status != nil {
		status = *in.Status
		if !reminder.ValidStatus(status) {
			return reminder.Reminder{}, fmt.Errorf("%w: unknown status %q", ErrInvalid, status)
		}
	}
	recur := cur.Recurrence
	if in.Recurrence != nil {
		recur = strings.TrimSpace(*in.Recurrence)
		if !reminder.ValidRecurrence(recur) {
			return reminder.Reminder{}, fmt.Errorf("%w: unknown recurrence %q", ErrInvalid, recur)
		}
	}
	if status == reminder.StatusDone {
		if next, ok := reminder.NextDueAfter(dueAt, tz, recur, time.Now().UTC()); ok {
			dueAt = next
			status = reminder.StatusScheduled
		}
	}
	listID := cur.ListID
	if in.ListID != nil {
		listID = in.ListID
	}
	pinned := cur.Pinned
	if in.Pinned != nil {
		pinned = *in.Pinned
	}
	// completed_at tracks status: stamp it on the way into done, clear it on the
	// way back out so a reopened reminder does not keep a stale completion time.
	completedAt := cur.CompletedAt
	switch {
	case status == reminder.StatusDone && completedAt == nil:
		now := time.Now().UTC()
		completedAt = &now
	case status != reminder.StatusDone:
		completedAt = nil
	}

	set := bson.M{
		"title":        title,
		"description":  desc,
		"due_at":       dueAt,
		"timezone":     tz,
		"priority":     priority,
		"status":       status,
		"pinned":       pinned,
		"recurrence":   recur,
		"list_id":      listID,
		"completed_at": completedAt,
		"updated_at":   time.Now().UTC(),
	}
	// Moving the due date re-arms delivery: the claims refer to the old time.
	if !dueAt.Equal(cur.DueAt) {
		set["sent_due_at"] = nil
		set["sent_advance_at"] = nil
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
	res, err := db.collection("reminders").UpdateOne(ctx, bson.M{
		"_id":     id,
		"user_id": userID,
		"status":  bson.M{"$in": []string{reminder.StatusScheduled, reminder.StatusPaused}},
	}, bson.M{"$set": bson.M{
		"snoozed_until":   until.UTC(),
		"sent_due_at":     nil,
		"sent_advance_at": nil,
		"updated_at":      time.Now().UTC(),
	}})
	if err != nil {
		return reminder.Reminder{}, err
	}
	if res.MatchedCount == 0 {
		return reminder.Reminder{}, ErrNotFound
	}
	return db.GetReminder(ctx, userID, id)
}

// DueKind distinguishes the two notifications a reminder can produce.
type DueKind string

const (
	KindDue     DueKind = "due"
	KindAdvance DueKind = "advance"
)

func (k DueKind) field() string {
	if k == KindAdvance {
		return "sent_advance_at"
	}
	return "sent_due_at"
}

// FireTime is when a reminder should actually notify. A snooze pushes the
// reminder out, so it wins over the original due date.
func FireTime(r reminder.Reminder) time.Time {
	if r.SnoozedUntil != nil && r.SnoozedUntil.After(r.DueAt) {
		return *r.SnoozedUntil
	}
	return r.DueAt
}

// ListPendingReminders returns scheduled reminders that could fire before the
// horizon and have not yet been claimed for this kind of notification. The
// caller filters per-user; the scheduler sweeps everyone in one query.
func (db *DB) ListPendingReminders(ctx context.Context, kind DueKind, horizon time.Time, limit int64) ([]reminder.Reminder, error) {
	if limit <= 0 {
		limit = 500
	}
	filter := bson.M{
		"status":     reminder.StatusScheduled,
		kind.field(): bson.M{"$eq": nil},
		// A snooze can push a reminder past the horizon; the caller rechecks
		// the effective fire time, so this bound only narrows the candidates.
		"$or": []bson.M{
			{"due_at": bson.M{"$lte": horizon}},
			{"snoozed_until": bson.M{"$lte": horizon, "$ne": nil}},
		},
	}
	cur, err := db.collection("reminders").Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "due_at", Value: 1}}).SetLimit(limit))
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

// ClaimReminderNotification atomically marks one notification as dispatched.
// It reports false when another worker (or an earlier tick) already claimed it,
// which is what keeps a reminder from being delivered twice.
func (db *DB) ClaimReminderNotification(ctx context.Context, id uuid.UUID, kind DueKind) (bool, error) {
	res, err := db.collection("reminders").UpdateOne(ctx,
		bson.M{"_id": id, kind.field(): bson.M{"$eq": nil}},
		bson.M{"$set": bson.M{kind.field(): time.Now().UTC()}},
	)
	if err != nil {
		return false, err
	}
	return res.ModifiedCount == 1, nil
}

// ReleaseReminderNotification undoes a claim so a failed send is retried on a
// later tick rather than being silently dropped.
func (db *DB) ReleaseReminderNotification(ctx context.Context, id uuid.UUID, kind DueKind) error {
	_, err := db.collection("reminders").UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{kind.field(): nil}},
	)
	return err
}
