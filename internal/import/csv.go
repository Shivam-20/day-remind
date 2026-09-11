package importdata

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/reminder"
	"github.com/shivam-20/day-remind/internal/storage"
)

// CSVReminder is a parsed row from a bulk-import CSV file.
type CSVReminder struct {
	Title       string
	Description string
	DueAt       time.Time
	Tags        []string
}

// ParseCSV reads reminder rows from CSV with columns: title, description, due_at, tags.
func ParseCSV(r io.Reader) ([]CSVReminder, error) {
	reader := csv.NewReader(r)
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read csv: %w", err)
	}
	if len(records) == 0 {
		return nil, nil
	}

	start := 0
	header := normalizeCSVHeader(records[0])
	if hasCSVHeader(header) {
		start = 1
	}

	var out []CSVReminder
	for i := start; i < len(records); i++ {
		row := records[i]
		if len(row) == 0 || strings.TrimSpace(row[0]) == "" {
			continue
		}
		title := strings.TrimSpace(row[0])
		desc := ""
		if len(row) > 1 {
			desc = strings.TrimSpace(row[1])
		}
		dueRaw := ""
		if len(row) > 2 {
			dueRaw = strings.TrimSpace(row[2])
		}
		tagsRaw := ""
		if len(row) > 3 {
			tagsRaw = strings.TrimSpace(row[3])
		}
		due, err := parseCSVTime(dueRaw)
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", i+1, err)
		}
		var tags []string
		for _, part := range strings.Split(tagsRaw, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				tags = append(tags, part)
			}
		}
		out = append(out, CSVReminder{
			Title: title, Description: desc, DueAt: due, Tags: tags,
		})
	}
	return out, nil
}

// CSVToReminderRecords converts parsed CSV rows into storage reminder records.
func CSVToReminderRecords(rows []CSVReminder, timezone string) []storage.ReminderRecord {
	if timezone == "" {
		timezone = "UTC"
	}
	out := make([]storage.ReminderRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, storage.ReminderRecord{
			ID:          uuid.New(),
			Title:       row.Title,
			Description: row.Description,
			DueAt:       row.DueAt,
			Timezone:    timezone,
			Status:      reminder.StatusScheduled,
			Priority:    reminder.PriorityNormal,
		})
	}
	return out
}

func normalizeCSVHeader(row []string) []string {
	out := make([]string, len(row))
	for i, col := range row {
		out[i] = strings.ToLower(strings.TrimSpace(col))
	}
	return out
}

func hasCSVHeader(header []string) bool {
	if len(header) == 0 {
		return false
	}
	return header[0] == "title"
}

func parseCSVTime(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, fmt.Errorf("due_at is required")
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid due_at: %q", raw)
}
