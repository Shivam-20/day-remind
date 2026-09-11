package importdata

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/export"
	"github.com/shivam-20/day-remind/internal/reminder"
	"github.com/shivam-20/day-remind/internal/storage"
)

// Import mode constants.
const (
	ModeMerge       = "merge"
	ModeMergeUpdate = "merge_update"
	ModeReplace     = "replace"
)

// PreviewResult summarizes an import without writing data.
type PreviewResult struct {
	RemindersNew  int      `json:"reminders_new"`
	RemindersSkip int      `json:"reminders_skip"`
	RemindersUpdate int    `json:"reminders_update"`
	Lists         int      `json:"lists"`
	Tags          int      `json:"tags"`
	Warnings      []string `json:"warnings"`
}

// ImportResult summarizes a completed import.
type ImportResult struct {
	RemindersImported int      `json:"reminders_imported"`
	ListsImported     int      `json:"lists_imported"`
	TagsImported      int      `json:"tags_imported"`
	Warnings          []string `json:"warnings"`
}

// PreviewImport parses JSON and returns counts without writing.
func PreviewImport(ctx context.Context, db *storage.DB, userID uuid.UUID, raw []byte) (PreviewResult, error) {
	bundle, err := parseBundle(raw)
	if err != nil {
		return PreviewResult{}, err
	}
	result := PreviewResult{
		Lists: len(bundle.Lists),
		Tags:  len(bundle.Tags),
	}
	for _, r := range bundle.Reminders {
		id, err := uuid.Parse(r.ID)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("invalid reminder id %q", r.ID))
			continue
		}
		exists, err := db.ReminderExists(ctx, userID, id)
		if err != nil {
			return PreviewResult{}, err
		}
		if exists {
			result.RemindersSkip++
		} else {
			result.RemindersNew++
		}
	}
	return result, nil
}

// ImportUserData imports a JSON backup using the given mode.
func ImportUserData(ctx context.Context, db *storage.DB, userID uuid.UUID, raw []byte, mode string) (ImportResult, error) {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" {
		mode = ModeMerge
	}
	switch mode {
	case ModeMerge, ModeMergeUpdate, ModeReplace:
	default:
		return ImportResult{}, fmt.Errorf("invalid import mode: %q", mode)
	}

	bundle, err := parseBundle(raw)
	if err != nil {
		return ImportResult{}, err
	}
	if bundle.SchemaVersion != "" && bundle.SchemaVersion != export.SchemaVersion {
		return ImportResult{}, fmt.Errorf("unsupported schema version: %s", bundle.SchemaVersion)
	}

	result := ImportResult{}
	if mode == ModeReplace {
		if err := db.DeleteUserExportData(ctx, userID); err != nil {
			return ImportResult{}, err
		}
	}

	for _, l := range bundle.Lists {
		id, err := uuid.Parse(l.ID)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("skip list %q: invalid id", l.Name))
			continue
		}
		if err := db.UpsertListRecord(ctx, userID, storage.ListRecord{
			ID: id, Name: l.Name, Color: l.Color, Paused: l.Paused,
		}); err != nil {
			return ImportResult{}, err
		}
		result.ListsImported++
	}

	for _, t := range bundle.Tags {
		id, err := uuid.Parse(t.ID)
		if err != nil {
			id = uuid.New()
		}
		if err := db.UpsertTagRecord(ctx, userID, storage.TagRecord{ID: id, Name: t.Name}); err != nil {
			return ImportResult{}, err
		}
		result.TagsImported++
	}

	for _, r := range bundle.Reminders {
		rec, warn := exportReminderToRecord(r)
		if warn != "" {
			result.Warnings = append(result.Warnings, warn)
			continue
		}
		exists, err := db.ReminderExists(ctx, userID, rec.ID)
		if err != nil {
			return ImportResult{}, err
		}
		if exists && mode == ModeMerge {
			continue
		}
		if err := db.UpsertReminderRecord(ctx, userID, rec); err != nil {
			return ImportResult{}, err
		}
		result.RemindersImported++
	}

	return result, nil
}

func parseBundle(raw []byte) (export.ExportBundle, error) {
	var bundle export.ExportBundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return export.ExportBundle{}, fmt.Errorf("parse export json: %w", err)
	}
	return bundle, nil
}

func exportReminderToRecord(r export.ExportReminder) (storage.ReminderRecord, string) {
	id, err := uuid.Parse(r.ID)
	if err != nil {
		id = uuid.New()
	}
	rec := storage.ReminderRecord{
		ID:               id,
		Title:            strings.TrimSpace(r.Title),
		Description:      r.Description,
		DueAt:            r.DueAt,
		Timezone:         r.Timezone,
		Status:           r.Status,
		Priority:         r.Priority,
		RecurrenceRule:   r.RecurrenceRule,
		ChannelOverrides: r.ChannelOverrides,
	}
	if rec.Title == "" {
		return storage.ReminderRecord{}, fmt.Sprintf("skip reminder %s: empty title", r.ID)
	}
	if rec.Status == "" {
		rec.Status = reminder.StatusScheduled
	}
	if rec.Priority == "" {
		rec.Priority = reminder.PriorityNormal
	}
	if rec.Timezone == "" {
		rec.Timezone = "UTC"
	}
	if r.ListID != nil && *r.ListID != "" {
		listID, err := uuid.Parse(*r.ListID)
		if err == nil {
			rec.ListID = &listID
		}
	}
	for _, tagID := range r.TagIDs {
		parsed, err := uuid.Parse(tagID)
		if err == nil {
			rec.TagIDs = append(rec.TagIDs, parsed)
		}
	}
	for _, st := range r.Subtasks {
		rec.Subtasks = append(rec.Subtasks, storage.SubtaskRecord{
			ID: uuid.New(), Title: st.Title, Done: st.Done,
		})
	}
	for _, off := range r.Offsets {
		rec.Offsets = append(rec.Offsets, storage.OffsetRecord{OffsetMinutes: off.OffsetMinutes})
	}
	return rec, ""
}
