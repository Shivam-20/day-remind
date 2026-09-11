package importdata

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/export"
	"github.com/shivam-20/day-remind/internal/reminder"
)

func TestPreviewAndImportMerge(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	user := createTestUser(t, db)

	listID := uuid.New()
	tagID := uuid.New()
	reminderID := uuid.New()
	bundle := export.ExportBundle{
		SchemaVersion: export.SchemaVersion,
		ExportedAt:    time.Now().UTC(),
		App:           "day-remind",
		Lists: []export.ExportList{
			{ID: listID.String(), Name: "Personal", Color: "#3b82f6"},
		},
		Tags: []export.ExportTag{
			{ID: tagID.String(), Name: "bills"},
		},
		Reminders: []export.ExportReminder{
			{
				ID:       reminderID.String(),
				Title:    "Pay rent",
				DueAt:    time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC),
				Timezone: "UTC",
				Status:   reminder.StatusScheduled,
				Priority: reminder.PriorityHigh,
				ListID:   ptr(listID.String()),
				TagIDs:   []string{tagID.String()},
			},
		},
	}
	raw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}

	preview, err := PreviewImport(ctx, db, user.ID, raw)
	if err != nil {
		t.Fatal(err)
	}
	if preview.RemindersNew != 1 || preview.Lists != 1 || preview.Tags != 1 {
		t.Fatalf("preview: %+v", preview)
	}

	result, err := ImportUserData(ctx, db, user.ID, raw, ModeMerge)
	if err != nil {
		t.Fatal(err)
	}
	if result.RemindersImported != 1 {
		t.Fatalf("import: %+v", result)
	}

	preview2, err := PreviewImport(ctx, db, user.ID, raw)
	if err != nil {
		t.Fatal(err)
	}
	if preview2.RemindersSkip != 1 || preview2.RemindersNew != 0 {
		t.Fatalf("second preview: %+v", preview2)
	}
}

func ptr(s string) *string { return &s }
