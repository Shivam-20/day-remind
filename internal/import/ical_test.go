package importdata

import (
	"strings"
	"testing"
	"time"
)

func TestParseICal(t *testing.T) {
	raw := `BEGIN:VCALENDAR
BEGIN:VEVENT
UID:abc@dayremind.app
SUMMARY:Team standup
DTSTART:20260911T090000Z
DESCRIPTION:Daily sync
END:VEVENT
END:VCALENDAR`
	rows, err := ParseICal(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows: %d", len(rows))
	}
	if rows[0].Title != "Team standup" {
		t.Fatalf("title: %q", rows[0].Title)
	}
	if rows[0].DueAt != time.Date(2026, 9, 11, 9, 0, 0, 0, time.UTC) {
		t.Fatalf("due: %v", rows[0].DueAt)
	}
}
