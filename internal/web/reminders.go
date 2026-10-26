package web

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/auth"
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
	Recurrence  string  `json:"recurrence"`
	Hidden      bool    `json:"hidden"`
	// PrivacyPassword bootstraps the account privacy password on the first
	// hidden reminder. It is ignored once that password exists.
	PrivacyPassword string `json:"privacy_password"`
}

type updateReminderRequest struct {
	Title           *string `json:"title"`
	Description     *string `json:"description"`
	DueAt           *string `json:"due_at"`
	Timezone        *string `json:"timezone"`
	Priority        *string `json:"priority"`
	Status          *string `json:"status"`
	Pinned          *bool   `json:"pinned"`
	Hidden          *bool   `json:"hidden"`
	ListID          *string `json:"list_id"`
	Recurrence      *string `json:"recurrence"`
	PrivacyPassword string  `json:"privacy_password"`
}

type unlockRequest struct {
	Password string `json:"password"`
}

type snoozeRequest struct {
	Until string `json:"until"`
}

// writeStorageError maps a storage-layer error onto the right status code, so
// a validation failure is not reported as a server fault and an internal
// database error is not echoed back to the caller.
func writeStorageError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, storage.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, storage.ErrConflict):
		writeError(w, http.StatusConflict, "conflict")
	case errors.Is(err, storage.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, fallback)
	}
}

func (s *Server) handleListReminders(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	q := r.URL.Query()
	status := strings.TrimSpace(q.Get("status"))
	if status != "" && !reminder.ValidStatus(status) {
		writeError(w, http.StatusBadRequest, "unknown status filter")
		return
	}
	f := reminder.ListFilter{UserID: userID, Status: status, Query: q.Get("q")}
	// An unparseable bound used to be dropped silently, handing back an
	// unfiltered list that looked like a successful narrow search.
	if raw := q.Get("from"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "from must be RFC3339")
			return
		}
		f.From = &t
	}
	if raw := q.Get("to"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "to must be RFC3339")
			return
		}
		f.To = &t
	}
	if f.From != nil && f.To != nil && f.To.Before(*f.From) {
		writeError(w, http.StatusBadRequest, "to must not be before from")
		return
	}
	items, err := s.db.ListReminders(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list reminders")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if item.Hidden {
			out = append(out, maskedReminderPayload(item))
			continue
		}
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
		Recurrence:  body.Recurrence,
		Hidden:      body.Hidden,
	}
	if body.ListID != nil && *body.ListID != "" {
		id, err := uuid.Parse(*body.ListID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid list_id")
			return
		}
		in.ListID = &id
	}
	// Hiding needs an account privacy password to unlock with; the first
	// hidden reminder can create it inline.
	if body.Hidden {
		if !s.ensurePrivacyPassword(w, r, userID, body.PrivacyPassword) {
			return
		}
	}
	item, err := s.db.CreateReminder(r.Context(), userID, in)
	if err != nil {
		writeStorageError(w, err, "could not create reminder")
		return
	}
	reminderResponse(w, http.StatusCreated, item)
}

func (s *Server) handleGetReminder(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	item, err := s.db.GetReminder(r.Context(), userID, id)
	if err != nil {
		writeStorageError(w, err, "could not load reminder")
		return
	}
	reminderResponse(w, http.StatusOK, item)
}

func (s *Server) handleUpdateReminder(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body updateReminderRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	in := reminder.UpdateInput{
		Title:       body.Title,
		Description: body.Description,
		Timezone:    body.Timezone,
		Priority:    body.Priority,
		Status:      body.Status,
		Pinned:      body.Pinned,
		Hidden:      body.Hidden,
		Recurrence:  body.Recurrence,
	}
	if body.DueAt != nil {
		dueAt, err := time.Parse(time.RFC3339, *body.DueAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "due_at must be RFC3339")
			return
		}
		in.DueAt = &dueAt
	}
	if body.ListID != nil && *body.ListID != "" {
		listID, err := uuid.Parse(*body.ListID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid list_id")
			return
		}
		in.ListID = &listID
	}
	if body.Hidden != nil && !*body.Hidden {
		// Unhiding reveals content, so it must go through POST /unhide where
		// the privacy password is verified; PATCH would bypass it.
		writeError(w, http.StatusForbidden, "unhide requires the privacy password — use POST /unhide")
		return
	}
	if body.Hidden != nil && *body.Hidden {
		if !s.ensurePrivacyPassword(w, r, userID, body.PrivacyPassword) {
			return
		}
	}
	item, err := s.db.UpdateReminder(r.Context(), userID, id, in)
	if err != nil {
		writeStorageError(w, err, "could not update reminder")
		return
	}
	reminderResponse(w, http.StatusOK, item)
}

func (s *Server) handleDeleteReminder(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.db.DeleteReminder(r.Context(), userID, id); err != nil {
		writeStorageError(w, err, "could not delete reminder")
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
	if err != nil {
		writeStorageError(w, err, "could not update reminder")
		return
	}
	reminderResponse(w, http.StatusOK, item)
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
	if err != nil {
		writeStorageError(w, err, "could not pause reminder")
		return
	}
	reminderResponse(w, http.StatusOK, item)
}

func (s *Server) handleResumeReminder(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	item, err := s.db.ResumeReminder(r.Context(), userID, id)
	if err != nil {
		writeStorageError(w, err, "could not resume reminder")
		return
	}
	reminderResponse(w, http.StatusOK, item)
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
	until, err := time.Parse(time.RFC3339, body.Until)
	if err != nil {
		writeError(w, http.StatusBadRequest, "until must be RFC3339")
		return
	}
	item, err := s.db.SnoozeReminder(r.Context(), userID, id, until)
	if err != nil {
		writeStorageError(w, err, "could not snooze reminder")
		return
	}
	reminderResponse(w, http.StatusOK, item)
}

// ---------------------------------------------------------------- privacy / unlock

const hiddenTitlePlaceholder = "Hidden reminder"

// ensurePrivacyPassword guarantees the account has a privacy password before a
// reminder is hidden, creating one from the offered value when none exists.
// An existing password is never replaced here; that happens in Settings.
// It writes the error response itself and reports false when the caller must stop.
func (s *Server) ensurePrivacyPassword(w http.ResponseWriter, r *http.Request, userID uuid.UUID, offered string) bool {
	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil {
		writeStorageError(w, err, "could not load profile")
		return false
	}
	if user.HideHash != "" {
		return true
	}
	hash, err := auth.HashPassword(offered)
	if err != nil {
		writeError(w, http.StatusBadRequest, "privacy password must be at least 8 characters")
		return false
	}
	if err := s.db.SetUserHideHash(r.Context(), userID, hash); err != nil {
		writeStorageError(w, err, "could not save privacy password")
		return false
	}
	return true
}

// verifyPrivacyPassword checks the request password against the account
// privacy password. It writes the error response itself and reports false when
// the caller must stop.
func (s *Server) verifyPrivacyPassword(w http.ResponseWriter, r *http.Request, userID, id uuid.UUID, password string) (reminder.Reminder, bool) {
	item, err := s.db.GetReminder(r.Context(), userID, id)
	if err != nil {
		writeStorageError(w, err, "could not load reminder")
		return reminder.Reminder{}, false
	}
	if !item.Hidden {
		// Unlocking an already visible reminder is a no-op, not an error.
		return item, true
	}
	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil {
		writeStorageError(w, err, "could not load profile")
		return reminder.Reminder{}, false
	}
	if user.HideHash == "" {
		// Defensive: a hidden reminder without a privacy password could never
		// be unlocked, so refuse rather than leak the content.
		writeError(w, http.StatusConflict, "privacy password is not configured")
		return reminder.Reminder{}, false
	}
	if !auth.CheckPassword(user.HideHash, password) {
		writeError(w, http.StatusUnauthorized, "wrong privacy password")
		return reminder.Reminder{}, false
	}
	return item, true
}

func (s *Server) handleUnlockReminder(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body unlockRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	item, ok := s.verifyPrivacyPassword(w, r, userID, id, body.Password)
	if !ok {
		return
	}
	// The unlock response is the only read that carries the real content.
	writeJSON(w, http.StatusOK, reminderPayload(item))
}

func (s *Server) handleUnhideReminder(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body unlockRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if _, ok := s.verifyPrivacyPassword(w, r, userID, id, body.Password); !ok {
		return
	}
	hidden := false
	item, err := s.db.UpdateReminder(r.Context(), userID, id, reminder.UpdateInput{Hidden: &hidden})
	if err != nil {
		writeStorageError(w, err, "could not update reminder")
		return
	}
	writeJSON(w, http.StatusOK, reminderPayload(item))
}

// reminderOrLock loads the parent reminder of a subtask/comment route and
// refuses hidden ones: their attachments count as content, so they unlock with
// the same privacy password as the reminder itself.
func (s *Server) reminderOrLock(w http.ResponseWriter, r *http.Request) (reminder.Reminder, bool) {
	userID := userIDFromContext(r.Context())
	id, ok := parseURLUUID(w, r, "id")
	if !ok {
		return reminder.Reminder{}, false
	}
	item, err := s.db.GetReminder(r.Context(), userID, id)
	if err != nil {
		writeStorageError(w, err, "not found")
		return reminder.Reminder{}, false
	}
	if item.Hidden {
		writeError(w, http.StatusLocked, "reminder is locked")
		return reminder.Reminder{}, false
	}
	return item, true
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
		"recurrence":  r.Recurrence,
		"pinned":      r.Pinned,
		"hidden":      r.Hidden,
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

// maskedReminderPayload is what every ordinary read of a hidden reminder
// returns: metadata stays (due time, status, priority), content does not. Only
// the unlock endpoint hands back the real title and description.
func maskedReminderPayload(r reminder.Reminder) map[string]any {
	out := reminderPayload(r)
	out["title"] = hiddenTitlePlaceholder
	out["description"] = ""
	return out
}

// reminderResponse writes a reminder payload, masking hidden reminders so a
// mutation response cannot leak content the caller has not unlocked.
func reminderResponse(w http.ResponseWriter, code int, r reminder.Reminder) {
	if r.Hidden {
		writeJSON(w, code, maskedReminderPayload(r))
		return
	}
	writeJSON(w, code, reminderPayload(r))
}
