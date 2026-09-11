package importdata

import (
	"strings"
	"testing"
	"time"
)

func TestParseCSV(t *testing.T) {
	raw := `title,description,due_at,tags
Pay rent,October rent,2026-10-01T10:00:00Z,"bills,home"`
	rows, err := ParseCSV(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows: %d", len(rows))
	}
	if rows[0].Title != "Pay rent" {
		t.Fatalf("title: %q", rows[0].Title)
	}
	if rows[0].DueAt != time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC) {
		t.Fatalf("due: %v", rows[0].DueAt)
	}
	if len(rows[0].Tags) != 2 {
		t.Fatalf("tags: %v", rows[0].Tags)
	}
}
