package reminder

import (
	"testing"
	"time"
)

func TestNextDueDailyKeepsLocalWallClock(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatal(err)
	}
	due := time.Date(2026, 10, 17, 18, 0, 0, 0, loc)
	next, ok := NextDue(due.UTC(), "Asia/Kolkata", RecurDaily)
	if !ok {
		t.Fatal("expected next due")
	}
	got := next.In(loc)
	if got.Day() != 18 || got.Hour() != 18 || got.Minute() != 0 {
		t.Fatalf("got %s", got)
	}
}

func TestNextDueAfterSkipsPastOccurrences(t *testing.T) {
	due := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	next, ok := NextDueAfter(due, "UTC", RecurDaily, now)
	if !ok {
		t.Fatal("expected next due")
	}
	if !next.After(now) {
		t.Fatalf("next %s is not after now %s", next, now)
	}
	want := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("got %s want %s", next, want)
	}
}

func TestNextDueEmptyDoesNotRepeat(t *testing.T) {
	if _, ok := NextDue(time.Now(), "UTC", ""); ok {
		t.Fatal("empty recurrence must not repeat")
	}
}
