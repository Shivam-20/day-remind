package web

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shivam-20/day-remind/internal/reminder"
	"github.com/shivam-20/day-remind/internal/storage"
)

type emailIngestRequest struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
	Text    string `json:"text"`
	From    string `json:"from"`
}

func (s *Server) handleEmailIngest(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(chi.URLParam(r, "token"))
	if token == "" {
		token = strings.TrimSpace(r.Header.Get("X-Ingest-Token"))
	}
	if token == "" {
		writeError(w, http.StatusUnauthorized, "token required")
		return
	}
	user, err := s.db.GetUserByEmailIngestToken(r.Context(), token)
	if err == storage.ErrNotFound {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not resolve token")
		return
	}

	var body emailIngestRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	title := strings.TrimSpace(body.Subject)
	if title == "" {
		title = "Email reminder"
	}
	desc := strings.TrimSpace(body.Body)
	if desc == "" {
		desc = strings.TrimSpace(body.Text)
	}

	loc, err := time.LoadLocation(user.Timezone)
	if err != nil || loc == nil {
		loc = time.UTC
	}
	localNow := time.Now().UTC().In(loc)
	dueAt := time.Date(localNow.Year(), localNow.Month(), localNow.Day()+1, 9, 0, 0, 0, loc)

	item, err := s.db.CreateReminder(r.Context(), user.ID, reminder.CreateInput{
		Title:       title,
		Description: desc,
		DueAt:       dueAt,
		Timezone:    user.Timezone,
		Priority:    reminder.PriorityNormal,
	})
	if writeStorageErr(w, err, "not found") {
		return
	}
	writeJSON(w, http.StatusCreated, reminderPayload(item))
}
