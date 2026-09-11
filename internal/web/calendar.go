package web

import (
	"net/http"
	"time"

	"github.com/shivam-20/day-remind/internal/reminder"
)

func (s *Server) handleCalendar(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	fromRaw := r.URL.Query().Get("from")
	toRaw := r.URL.Query().Get("to")
	if fromRaw == "" || toRaw == "" {
		writeError(w, http.StatusBadRequest, "from and to are required")
		return
	}
	from, err := time.Parse(time.RFC3339, fromRaw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "from must be RFC3339")
		return
	}
	to, err := time.Parse(time.RFC3339, toRaw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "to must be RFC3339")
		return
	}

	items, err := s.db.ListReminders(r.Context(), reminder.ListFilter{
		UserID: userID,
		From:   &from,
		To:     &to,
		Limit:  500,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load calendar")
		return
	}

	events := make([]map[string]any, 0, len(items))
	for _, item := range items {
		events = append(events, calendarEventPayload(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func calendarEventPayload(r reminder.Reminder) map[string]any {
	end := r.DueAt.Add(30 * time.Minute)
	out := map[string]any{
		"id":          r.ID.String(),
		"reminder_id": r.ID.String(),
		"title":       r.Title,
		"start":       r.DueAt.UTC().Format(time.RFC3339),
		"end":         end.UTC().Format(time.RFC3339),
		"status":      r.Status,
		"priority":    r.Priority,
		"timezone":    r.Timezone,
	}
	if r.ListID != nil {
		out["list_id"] = r.ListID.String()
	}
	if len(r.TagIDs) > 0 {
		out["tag_ids"] = uuidStrings(r.TagIDs)
	}
	if r.RecurrenceRule != "" {
		out["recurrence_rule"] = r.RecurrenceRule
	}
	return out
}
