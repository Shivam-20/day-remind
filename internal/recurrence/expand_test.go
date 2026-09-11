package recurrence

import (
	"testing"
	"time"
)

func TestNextOccurrence_Daily(t *testing.T) {
	rule := "DTSTART:20260101T090000Z\nRRULE:FREQ=DAILY;COUNT=5"
	after := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)

	next, err := NextOccurrence(rule, after, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 1, 2, 9, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("expected %v, got %v", want, next)
	}
}

func TestNextOccurrence_Weekly(t *testing.T) {
	rule := "DTSTART:20260105T100000Z\nRRULE:FREQ=WEEKLY;BYDAY=MO;COUNT=3"
	after := time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC)

	next, err := NextOccurrence(rule, after, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 1, 12, 10, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("expected %v, got %v", want, next)
	}
}

func TestNextOccurrence_EmptyRule(t *testing.T) {
	_, err := NextOccurrence("", time.Now(), "UTC")
	if err == nil {
		t.Fatal("expected error for empty rule")
	}
}

func TestNextOccurrence_NoMoreOccurrences(t *testing.T) {
	rule := "DTSTART:20260101T090000Z\nRRULE:FREQ=DAILY;COUNT=1"
	after := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	_, err := NextOccurrence(rule, after, "UTC")
	if err == nil {
		t.Fatal("expected error when no occurrence remains")
	}
}
