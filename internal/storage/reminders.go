package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/reminder"
)

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
	id := uuid.New()
	now := time.Now().UTC()
	_, err := db.SQL.ExecContext(ctx, `
		INSERT INTO reminders (
			id, user_id, list_id, title, description, due_at, timezone,
			status, priority, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$10)`,
		id, userID, in.ListID, title, strings.TrimSpace(in.Description), in.DueAt.UTC(), tz,
		reminder.StatusScheduled, priority, now,
	)
	if err != nil {
		return reminder.Reminder{}, err
	}
	return db.GetReminder(ctx, userID, id)
}

func (db *DB) GetReminder(ctx context.Context, userID, id uuid.UUID) (reminder.Reminder, error) {
	return scanReminder(db.SQL.QueryRowContext(ctx, reminderSelectWhereID, id, userID))
}

func (db *DB) ListReminders(ctx context.Context, f reminder.ListFilter) ([]reminder.Reminder, error) {
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := reminderSelectBase + ` WHERE r.user_id = $1`
	args := []any{f.UserID}
	n := 2
	if f.Status != "" {
		query += fmt.Sprintf(` AND r.status = $%d`, n)
		args = append(args, f.Status)
		n++
	}
	if f.From != nil {
		query += fmt.Sprintf(` AND r.due_at >= $%d`, n)
		args = append(args, f.From.UTC())
		n++
	}
	if f.To != nil {
		query += fmt.Sprintf(` AND r.due_at <= $%d`, n)
		args = append(args, f.To.UTC())
		n++
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		query += fmt.Sprintf(` AND (r.title ILIKE $%d OR r.description ILIKE $%d)`, n, n)
		args = append(args, "%"+q+"%")
		n++
	}
	query += fmt.Sprintf(` ORDER BY r.due_at ASC LIMIT $%d`, n)
	args = append(args, limit)

	rows, err := db.SQL.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []reminder.Reminder
	for rows.Next() {
		r, err := scanReminderRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
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
	var completedAt *time.Time
	if status == reminder.StatusDone && cur.CompletedAt == nil {
		now := time.Now().UTC()
		completedAt = &now
	} else {
		completedAt = cur.CompletedAt
	}
	_, err = db.SQL.ExecContext(ctx, `
		UPDATE reminders SET
			title = $3, description = $4, due_at = $5, timezone = $6,
			priority = $7, status = $8, list_id = $9, completed_at = $10, updated_at = NOW()
		WHERE id = $1 AND user_id = $2`,
		id, userID, title, desc, dueAt, tz, priority, status, listID, completedAt,
	)
	if err != nil {
		return reminder.Reminder{}, err
	}
	return db.GetReminder(ctx, userID, id)
}

func (db *DB) DeleteReminder(ctx context.Context, userID, id uuid.UUID) error {
	res, err := db.SQL.ExecContext(ctx, `DELETE FROM reminders WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (db *DB) PauseReminder(ctx context.Context, userID, id uuid.UUID, resumeAt *time.Time) (reminder.Reminder, error) {
	now := time.Now().UTC()
	_, err := db.SQL.ExecContext(ctx, `
		UPDATE reminders SET status = $3, paused_at = $4, resume_at = $5, updated_at = NOW()
		WHERE id = $1 AND user_id = $2 AND status IN ('scheduled', 'paused')`,
		id, userID, reminder.StatusPaused, now, resumeAt)
	if err != nil {
		return reminder.Reminder{}, err
	}
	return db.GetReminder(ctx, userID, id)
}

func (db *DB) ResumeReminder(ctx context.Context, userID, id uuid.UUID) (reminder.Reminder, error) {
	_, err := db.SQL.ExecContext(ctx, `
		UPDATE reminders SET status = $3, paused_at = NULL, resume_at = NULL, updated_at = NOW()
		WHERE id = $1 AND user_id = $2 AND status = 'paused'`,
		id, userID, reminder.StatusScheduled)
	if err != nil {
		return reminder.Reminder{}, err
	}
	return db.GetReminder(ctx, userID, id)
}

func (db *DB) SnoozeReminder(ctx context.Context, userID, id uuid.UUID, until time.Time) (reminder.Reminder, error) {
	_, err := db.SQL.ExecContext(ctx, `
		UPDATE reminders SET snoozed_until = $3, updated_at = NOW()
		WHERE id = $1 AND user_id = $2`,
		id, userID, until.UTC())
	if err != nil {
		return reminder.Reminder{}, err
	}
	return db.GetReminder(ctx, userID, id)
}

const reminderSelectBase = `
	SELECT r.id, r.user_id, r.list_id, r.title, r.description, r.due_at, r.timezone,
	       r.status, r.priority, r.pinned, r.snoozed_until, r.paused_at, r.resume_at,
	       r.completed_at, r.created_at, r.updated_at
	FROM reminders r`

const reminderSelectWhereID = reminderSelectBase + ` WHERE r.id = $1 AND r.user_id = $2`

func scanReminder(row *sql.Row) (reminder.Reminder, error) {
	var r reminder.Reminder
	var listID sql.NullString
	var snoozed, paused, resume, completed sql.NullTime
	err := row.Scan(
		&r.ID, &r.UserID, &listID, &r.Title, &r.Description, &r.DueAt, &r.Timezone,
		&r.Status, &r.Priority, &r.Pinned, &snoozed, &paused, &resume, &completed,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return reminder.Reminder{}, ErrNotFound
	}
	if err != nil {
		return reminder.Reminder{}, err
	}
	if listID.Valid {
		id, err := uuid.Parse(listID.String)
		if err == nil {
			r.ListID = &id
		}
	}
	r.SnoozedUntil = nullTimePtr(snoozed)
	r.PausedAt = nullTimePtr(paused)
	r.ResumeAt = nullTimePtr(resume)
	r.CompletedAt = nullTimePtr(completed)
	return r, nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanReminderRow(row scannable) (reminder.Reminder, error) {
	var r reminder.Reminder
	var listID sql.NullString
	var snoozed, paused, resume, completed sql.NullTime
	err := row.Scan(
		&r.ID, &r.UserID, &listID, &r.Title, &r.Description, &r.DueAt, &r.Timezone,
		&r.Status, &r.Priority, &r.Pinned, &snoozed, &paused, &resume, &completed,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return reminder.Reminder{}, err
	}
	if listID.Valid {
		id, err := uuid.Parse(listID.String)
		if err == nil {
			r.ListID = &id
		}
	}
	r.SnoozedUntil = nullTimePtr(snoozed)
	r.PausedAt = nullTimePtr(paused)
	r.ResumeAt = nullTimePtr(resume)
	r.CompletedAt = nullTimePtr(completed)
	return r, nil
}

func nullTimePtr(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}
