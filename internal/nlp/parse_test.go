package nlp

import (
	"testing"
	"time"
)

func TestParseRemindMeTomorrow(t *testing.T) {
	loc := time.FixedZone("IST", 5*3600+30*60)
	now := time.Date(2026, 9, 10, 10, 0, 0, 0, loc)
	got := Parse("remind me to call dentist tomorrow at 3pm", now, loc)
	if got.Title != "call dentist" {
		t.Fatalf("title: got %q", got.Title)
	}
	if got.DueAt.Day() != 11 || got.DueAt.Hour() != 15 {
		t.Fatalf("due: got %v", got.DueAt)
	}
	if got.Priority != "normal" {
		t.Fatalf("priority: got %q", got.Priority)
	}
}

func TestParseMonthlyRent(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 9, 1, 8, 0, 0, 0, loc)
	got := Parse("pay rent on 1st at 10am every month", now, loc)
	if got.Title != "pay rent" {
		t.Fatalf("title: got %q", got.Title)
	}
	if got.RecurrenceRule != "FREQ=MONTHLY" {
		t.Fatalf("recurrence: got %q", got.RecurrenceRule)
	}
	if got.DueAt.Hour() != 10 {
		t.Fatalf("due hour: got %d", got.DueAt.Hour())
	}
}

func TestParseTagsAndPriority(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, loc)
	got := Parse("urgent: submit report #work tomorrow at 9am", now, loc)
	if got.Priority != "high" {
		t.Fatalf("priority: got %q", got.Priority)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "work" {
		t.Fatalf("tags: got %v", got.Tags)
	}
}
