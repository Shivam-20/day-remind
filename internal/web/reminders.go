package web

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/reminder"
	"github.com/shivam-20/day-remind/internal/storage"
)

type createReminderRequest struct {
	Title       string  `json:"title"`
	Description string  `json:"description"`
	DueAt       string  `json:"due_at"`
	Timezone    string  `json:"timezone"`
	Priority    string  `json:"priority"`
	ListID      *string `json:"list_id"`
}

type snoozeRequest struct {
	Until  string `json:"until"`
	Preset string `json:"preset"`
}

func (s *Server) handleListReminders(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	f := reminder.ListFilter{UserID: userID, Status: r.URL.Query().Get("status"), Query: r.URL.Query().Get("q")}
	if raw := r.URL.Query().Get("from"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err == nil {
			f.From = &t
		}
	}
	if raw := r.URL.Query().Get("to"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err == nil {
			f.To = &t
		}
	}
	items, err := s.db.ListReminders(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list reminders")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, reminderPayload(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"reminders": out})
}

func (s *Server) handleCreateReminder(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	var body createReminderRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	dueAt, err := time.Parse(time.RFC3339, body.DueAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "due_at must be RFC3339")
		return
	}
	in := reminder.CreateInput{
		Title:       body.Title,
		Description: body.Description,
		DueAt:       dueAt,
		Timezone:    body.Timezone,
		Priority:    body.Priority,
	}
	if body.ListID != nil && *body.ListID != "" {
		id, err := uuid.Parse(*body.ListID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid list_id")
			return
		}
		in.ListID = &id
	}
	item, err := s.db.CreateReminder(r.Context(), userID, in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, reminderPayload(item))
}

func (s *Server) handleGetReminder(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	item, err := s.db.GetReminder(r.Context(), userID, id)
	if err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load reminder")
		return
	}
	writeJSON(w, http.StatusOK, reminderPayload(item))
}

func (s *Server) handleUpdateReminder(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body createReminderRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	in := reminder.UpdateInput{
		Title:       &body.Title,
		Description: &body.Description,
		Priority:    &body.Priority,
		Timezone:    &body.Timezone,
	}
	if body.DueAt != "" {
		dueAt, err := time.Parse(time.RFC3339, body.DueAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "due_at must be RFC3339")
			return
		}
		in.DueAt = &dueAt
	}
	item, err := s.db.UpdateReminder(r.Context(), userID, id, in)
	if err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, reminderPayload(item))
}

func (s *Server) handleDeleteReminder(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.db.DeleteReminder(r.Context(), userID, id); err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleDoneReminder(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	status := reminder.StatusDone
	item, err := s.db.UpdateReminder(r.Context(), userID, id, reminder.UpdateInput{Status: &status})
	if err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update")
		return
	}
	writeJSON(w, http.StatusOK, reminderPayload(item))
}

func (s *Server) handlePauseReminder(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body struct {
		ResumeAt *string `json:"resume_at"`
	}
	_ = decodeJSON(r, &body)
	var resumeAt *time.Time
	if body.ResumeAt != nil && *body.ResumeAt != "" {
		t, err := time.Parse(time.RFC3339, *body.ResumeAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "resume_at must be RFC3339")
			return
		}
		resumeAt = &t
	}
	item, err := s.db.PauseReminder(r.Context(), userID, id, resumeAt)
	if err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not pause")
		return
	}
	writeJSON(w, http.StatusOK, reminderPayload(item))
}

func (s *Server) handleResumeReminder(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	item, err := s.db.ResumeReminder(r.Context(), userID, id)
	if err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not resume")
		return
	}
	writeJSON(w, http.StatusOK, reminderPayload(item))
}

func (s *Server) handleSnoozeReminder(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body snoozeRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	var until time.Time
	if body.Preset != "" {
		cur, err := s.db.GetReminder(r.Context(), userID, id)
		if err == storage.ErrNotFound {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not load reminder")
			return
		}
		t, err := reminder.SnoozeUntil(body.Preset, time.Now().UTC(), cur.Timezone)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		until = t
	} else if body.Until != "" {
		t, err := time.Parse(time.RFC3339, body.Until)
		if err != nil {
			writeError(w, http.StatusBadRequest, "until must be RFC3339")
			return
		}
		until = t
	} else {
		writeError(w, http.StatusBadRequest, "preset or until required")
		return
	}
	item, err := s.db.SnoozeReminder(r.Context(), userID, id, until)
	if err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not snooze")
		return
	}
	writeJSON(w, http.StatusOK, reminderPayload(item))
}

func (s *Server) handleSnoozePresets(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"presets": reminder.SnoozePresets()})
}

func reminderPayload(r reminder.Reminder) map[string]any {
	out := map[string]any{
		"id":          r.ID.String(),
		"title":       r.Title,
		"description": r.Description,
		"due_at":      r.DueAt.UTC().Format(time.RFC3339),
		"timezone":    r.Timezone,
		"status":      r.Status,
		"priority":    r.Priority,
		"pinned":      r.Pinned,
		"created_at":  r.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":  r.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if r.ListID != nil {
		out["list_id"] = r.ListID.String()
	}
	if r.SnoozedUntil != nil {
		out["snoozed_until"] = r.SnoozedUntil.UTC().Format(time.RFC3339)
	}
	if r.PausedAt != nil {
		out["paused_at"] = r.PausedAt.UTC().Format(time.RFC3339)
	}
	if r.ResumeAt != nil {
		out["resume_at"] = r.ResumeAt.UTC().Format(time.RFC3339)
	}
	if r.CompletedAt != nil {
		out["completed_at"] = r.CompletedAt.UTC().Format(time.RFC3339)
	}
	return out
}
