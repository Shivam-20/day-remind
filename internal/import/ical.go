package importdata

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/reminder"
	"github.com/shivam-20/day-remind/internal/storage"
)

// ParseICal reads VEVENT components from an iCalendar file.
func ParseICal(r io.Reader) ([]storage.ReminderRecord, error) {
	scanner := bufio.NewScanner(r)
	var out []storage.ReminderRecord
	var inEvent bool
	var cur storage.ReminderRecord

	flush := func() {
		if cur.Title != "" && !cur.DueAt.IsZero() {
			if cur.ID == uuid.Nil {
				cur.ID = uuid.New()
			}
			if cur.Status == "" {
				cur.Status = reminder.StatusScheduled
			}
			if cur.Priority == "" {
				cur.Priority = reminder.PriorityNormal
			}
			if cur.Timezone == "" {
				cur.Timezone = "UTC"
			}
			out = append(out, cur)
		}
		cur = storage.ReminderRecord{}
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "BEGIN:VEVENT") {
			inEvent = true
			cur = storage.ReminderRecord{}
			continue
		}
		if strings.HasPrefix(line, "END:VEVENT") {
			inEvent = false
			flush()
			continue
		}
		if !inEvent {
			continue
		}
		key, value := splitICalLine(line)
		switch key {
		case "SUMMARY":
			cur.Title = icalUnescape(value)
		case "DESCRIPTION":
			cur.Description = icalUnescape(value)
		case "DTSTART":
			if t, err := parseICalTime(value); err == nil {
				cur.DueAt = t
			}
		case "RRULE":
			cur.RecurrenceRule = value
		case "STATUS":
			switch strings.ToUpper(value) {
			case "COMPLETED":
				cur.Status = reminder.StatusDone
			case "CANCELLED":
				cur.Status = reminder.StatusCancelled
			default:
				cur.Status = reminder.StatusScheduled
			}
		case "UID":
			if id, err := uuid.Parse(strings.Split(value, "@")[0]); err == nil {
				cur.ID = id
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read ical: %w", err)
	}
	return out, nil
}

func splitICalLine(line string) (string, string) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return line, ""
	}
	keyPart := line[:idx]
	value := line[idx+1:]
	if semi := strings.Index(keyPart, ";"); semi >= 0 {
		keyPart = keyPart[:semi]
	}
	return strings.ToUpper(keyPart), value
}

func parseICalTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	layouts := []string{
		"20060102T150405Z",
		"20060102T150405",
		"20060102",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid ical datetime: %q", raw)
}

// ImportICal parses an iCalendar file and creates reminders for the user.
func ImportICal(ctx context.Context, db *storage.DB, userID uuid.UUID, raw []byte) (int, error) {
	records, err := ParseICal(strings.NewReader(string(raw)))
	if err != nil {
		return 0, err
	}
	imported := 0
	for _, rec := range records {
		exists, err := db.ReminderExists(ctx, userID, rec.ID)
		if err != nil {
			return imported, err
		}
		if exists {
			continue
		}
		_, err = db.CreateReminder(ctx, userID, reminder.CreateInput{
			Title:          rec.Title,
			Description:    rec.Description,
			DueAt:          rec.DueAt,
			Timezone:       rec.Timezone,
			Priority:       rec.Priority,
			RecurrenceRule: rec.RecurrenceRule,
		})
		if err != nil {
			return imported, err
		}
		imported++
	}
	return imported, nil
}

func icalUnescape(s string) string {
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\,", ",")
	s = strings.ReplaceAll(s, "\\;", ";")
	s = strings.ReplaceAll(s, "\\\\", "\\")
	return s
}
