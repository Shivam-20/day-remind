package scheduler

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/reminder"
	"github.com/shivam-20/day-remind/internal/storage"
)

func TestOverrideAllows(t *testing.T) {
	open := reminder.Reminder{}
	if !overrideAllows(open, "email") || !overrideAllows(open, "telegram") {
		t.Fatal("empty overrides should allow account channels")
	}
	picked := reminder.Reminder{ChannelOverrides: map[string]bool{"email": true, "telegram": false}}
	if !overrideAllows(picked, "email") || overrideAllows(picked, "telegram") {
		t.Fatal("saved overrides should allow only selected channels")
	}
}

func TestFormatOffsetLead(t *testing.T) {
	cases := []struct {
		minutes int
		want    string
	}{
		{1, "1 minute"},
		{15, "15 minutes"},
		{60, "1 hour"},
		{90, "90 minutes"},
		{120, "2 hours"},
		{1440, "1 day"},
		{2880, "2 days"},
		{10080, "1 week"},
		{20160, "2 weeks"},
	}
	for _, c := range cases {
		if got := formatOffsetLead(c.minutes); got != c.want {
			t.Errorf("formatOffsetLead(%d) = %q, want %q", c.minutes, got, c.want)
		}
	}
}

func TestOccurrenceDeliveryContext(t *testing.T) {
	offset := storage.ReminderOccurrence{
		ID: uuid.New(), Kind: storage.OccurrenceKindOffset, OffsetMinutes: 120,
		FireAt: time.Now().UTC(),
	}
	dctx := occurrenceDeliveryContext(offset)
	if !dctx.Advance || dctx.Lead != "2 hours" {
		t.Fatalf("offset context: %+v", dctx)
	}

	due := storage.ReminderOccurrence{ID: uuid.New(), Kind: storage.OccurrenceKindMain, FireAt: time.Now().UTC()}
	dctx = occurrenceDeliveryContext(due)
	if dctx.Advance || dctx.Lead != "" {
		t.Fatalf("due context: %+v", dctx)
	}
}
