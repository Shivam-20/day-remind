package web

import (
	"net/http"
	"strings"
	"time"

	"github.com/shivam-20/day-remind/internal/nlp"
)

type parseReminderRequest struct {
	Text     string `json:"text"`
	Timezone string `json:"timezone"`
}

func (s *Server) handleParseReminder(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	var body parseReminderRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(body.Text) == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load user")
		return
	}
	tz := body.Timezone
	if tz == "" {
		tz = user.Timezone
	}
	loc, err := time.LoadLocation(tz)
	if err != nil || loc == nil {
		loc = time.UTC
	}
	parsed := nlp.Parse(body.Text, time.Now().UTC(), loc)
	out := map[string]any{
		"title":    parsed.Title,
		"priority": parsed.Priority,
		"tags":     parsed.Tags,
	}
	if !parsed.DueAt.IsZero() {
		out["due_at"] = parsed.DueAt.UTC().Format(time.RFC3339)
	}
	if parsed.RecurrenceRule != "" {
		out["recurrence_rule"] = parsed.RecurrenceRule
	}
	writeJSON(w, http.StatusOK, out)
}
