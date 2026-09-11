package export

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/storage"
)

const SchemaVersion = "day_remind_export_v1"

// ExportBundle is the canonical JSON backup format for Day Remind.
type ExportBundle struct {
	SchemaVersion      string                   `json:"schema_version"`
	ExportedAt         time.Time                `json:"exported_at"`
	App                string                   `json:"app"`
	User               ExportUser               `json:"user"`
	Lists              []ExportList             `json:"lists"`
	Tags               []ExportTag              `json:"tags"`
	Templates          []map[string]interface{} `json:"templates"`
	Reminders          []ExportReminder         `json:"reminders"`
	NotificationPrefs  []ExportNotificationPref `json:"notification_prefs"`
	Settings           ExportSettings           `json:"settings"`
}

type ExportUser struct {
	Email       string `json:"email"`
	Timezone    string `json:"timezone"`
	DisplayName string `json:"display_name"`
}

type ExportList struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Color  string `json:"color"`
	Paused bool   `json:"paused"`
}

type ExportTag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ExportSubtask struct {
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

type ExportOffset struct {
	OffsetMinutes int `json:"offset_minutes"`
}

type ExportReminder struct {
	ID               string          `json:"id"`
	Title            string          `json:"title"`
	Description      string          `json:"description"`
	DueAt            time.Time       `json:"due_at"`
	Timezone         string          `json:"timezone"`
	Status           string          `json:"status"`
	Priority         string          `json:"priority"`
	ListID           *string         `json:"list_id,omitempty"`
	TagIDs           []string        `json:"tag_ids,omitempty"`
	RecurrenceRule   string          `json:"recurrence_rule,omitempty"`
	Offsets          []ExportOffset  `json:"offsets,omitempty"`
	Subtasks         []ExportSubtask `json:"subtasks,omitempty"`
	ChannelOverrides map[string]bool `json:"channel_overrides,omitempty"`
}

type ExportNotificationPref struct {
	Channel string `json:"channel"`
	Enabled bool   `json:"enabled"`
}

type ExportSettings struct {
	QuietHours           QuietHours `json:"quiet_hours"`
	MorningDigestEnabled bool       `json:"morning_digest_enabled"`
	MorningDigestTime    string     `json:"morning_digest_time,omitempty"`
}

type QuietHours struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// ExportUserData builds a full export bundle for a user.
func ExportUserData(ctx context.Context, db *storage.DB, userID uuid.UUID) (ExportBundle, error) {
	user, err := db.GetUserByID(ctx, userID)
	if err != nil {
		return ExportBundle{}, err
	}
	settings, err := db.LoadUserSettings(ctx, userID)
	if err != nil {
		return ExportBundle{}, err
	}
	prefs, err := db.GetUserNotificationPrefs(ctx, userID)
	if err != nil {
		return ExportBundle{}, err
	}
	lists, err := db.ListUserLists(ctx, userID)
	if err != nil {
		return ExportBundle{}, err
	}
	tags, err := db.ListUserTags(ctx, userID)
	if err != nil {
		return ExportBundle{}, err
	}
	reminders, err := db.ListUserReminderRecords(ctx, userID)
	if err != nil {
		return ExportBundle{}, err
	}

	bundle := ExportBundle{
		SchemaVersion: SchemaVersion,
		ExportedAt:    time.Now().UTC(),
		App:           "day-remind",
		User: ExportUser{
			Email:       user.Email,
			Timezone:    user.Timezone,
			DisplayName: user.DisplayName,
		},
		Templates: []map[string]interface{}{},
		Settings: ExportSettings{
			QuietHours: QuietHours{
				Start: settings.QuietStart,
				End:   settings.QuietEnd,
			},
			MorningDigestEnabled: settings.MorningDigestEnabled,
			MorningDigestTime:    settings.MorningDigestTime,
		},
	}
	for _, l := range lists {
		bundle.Lists = append(bundle.Lists, ExportList{
			ID: l.ID.String(), Name: l.Name, Color: l.Color, Paused: l.Paused,
		})
	}
	for _, t := range tags {
		bundle.Tags = append(bundle.Tags, ExportTag{ID: t.ID.String(), Name: t.Name})
	}
	for _, p := range prefs {
		bundle.NotificationPrefs = append(bundle.NotificationPrefs, ExportNotificationPref{
			Channel: p.Channel, Enabled: p.Enabled,
		})
	}
	for _, r := range reminders {
		bundle.Reminders = append(bundle.Reminders, reminderToExport(r))
	}
	return bundle, nil
}

func reminderToExport(r storage.ReminderRecord) ExportReminder {
	out := ExportReminder{
		ID:               r.ID.String(),
		Title:            r.Title,
		Description:      r.Description,
		DueAt:            r.DueAt,
		Timezone:         r.Timezone,
		Status:           r.Status,
		Priority:         r.Priority,
		RecurrenceRule:   r.RecurrenceRule,
		ChannelOverrides: r.ChannelOverrides,
	}
	if r.ListID != nil {
		s := r.ListID.String()
		out.ListID = &s
	}
	for _, id := range r.TagIDs {
		out.TagIDs = append(out.TagIDs, id.String())
	}
	for _, st := range r.Subtasks {
		out.Subtasks = append(out.Subtasks, ExportSubtask{Title: st.Title, Done: st.Done})
	}
	for _, off := range r.Offsets {
		out.Offsets = append(out.Offsets, ExportOffset{OffsetMinutes: off.OffsetMinutes})
	}
	return out
}
