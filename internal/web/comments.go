package web

import (
	"net/http"
	"time"

	"github.com/shivam-20/day-remind/internal/storage"
)

type commentCreateRequest struct {
	Body string `json:"body"`
}

func (s *Server) handleListComments(w http.ResponseWriter, r *http.Request) {
	reminderID, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}
	userID := userIDFromContext(r.Context())
	if _, err := s.db.GetReminder(r.Context(), userID, reminderID); err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load reminder")
		return
	}
	items, err := s.db.ListComments(r.Context(), reminderID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list comments")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, commentPayload(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"comments": out})
}

func (s *Server) handleCreateComment(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	reminderID, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}
	if _, err := s.db.GetReminder(r.Context(), userID, reminderID); err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load reminder")
		return
	}
	var body commentCreateRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	item, err := s.db.CreateComment(r.Context(), userID, reminderID, body.Body)
	if writeStorageErr(w, err, "not found") {
		return
	}
	writeJSON(w, http.StatusCreated, commentPayload(item))
}

func commentPayload(c storage.Comment) map[string]any {
	return map[string]any{
		"id":          c.ID.String(),
		"reminder_id": c.ReminderID.String(),
		"user_id":     c.UserID.String(),
		"body":        c.Body,
		"created_at":  c.CreatedAt.UTC().Format(time.RFC3339),
	}
}
