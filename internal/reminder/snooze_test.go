package reminder

import (
	"testing"
	"time"
)

func TestSnoozeUntilPresets(t *testing.T) {
	now := time.Date(2026, 9, 11, 14, 30, 0, 0, time.UTC)
	until, err := SnoozeUntil("+15m", now, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if until.Sub(now) != 15*time.Minute {
		t.Fatalf("expected +15m, got %v", until.Sub(now))
	}
}
