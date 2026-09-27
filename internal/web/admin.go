package web

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/auth"
	"github.com/shivam-20/day-remind/internal/reminder"
	"github.com/shivam-20/day-remind/internal/storage"
)

type adminCreateUserRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Timezone    string `json:"timezone"`
	IsAdmin     bool   `json:"is_admin"`
}

type adminUpdateUserRequest struct {
	Email               *string    `json:"email"`
	Password            *string    `json:"password"`
	DisplayName         *string    `json:"display_name"`
	Timezone            *string    `json:"timezone"`
	Locale              *string    `json:"locale"`
	IsAdmin             *bool      `json:"is_admin"`
	EmailVerified       *bool      `json:"email_verified"`
	NotificationsPaused *bool      `json:"notifications_paused"`
	PausedUntil         *time.Time `json:"notifications_paused_until"`
}

type adminCreateReminderRequest struct {
	UserID      string  `json:"user_id"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	DueAt       string  `json:"due_at"`
	Timezone    string  `json:"timezone"`
	Priority    string  `json:"priority"`
	ListID      *string `json:"list_id"`
}

func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.db.AdminStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load stats")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"users":     stats.Users,
		"reminders": stats.Reminders,
		"sessions":  stats.Sessions,
	})
}

func (s *Server) handleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	f := storage.UserListFilter{
		Query:  r.URL.Query().Get("q"),
		Limit:  queryInt(r, "limit", 100),
		Offset: queryInt(r, "offset", 0),
	}
	users, err := s.db.ListUsers(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list users")
		return
	}
	out := make([]map[string]any, 0, len(users))
	for _, user := range users {
		out = append(out, adminUserPayload(user))
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": out})
}

func (s *Server) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	var body adminCreateUserRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	user, err := s.db.CreateUserAdmin(r.Context(), body.Email, hash, body.DisplayName, body.Timezone, body.IsAdmin)
	if err == storage.ErrConflict {
		writeError(w, http.StatusConflict, "email already registered")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, adminUserPayload(user))
}

func (s *Server) handleAdminGetUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	user, err := s.db.GetUserByID(r.Context(), id)
	if err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load user")
		return
	}
	writeJSON(w, http.StatusOK, adminUserPayload(user))
}

func (s *Server) handleAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body adminUpdateUserRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	in := storage.AdminUserUpdate{
		Email:                    body.Email,
		DisplayName:              body.DisplayName,
		Timezone:                 body.Timezone,
		Locale:                   body.Locale,
		IsAdmin:                  body.IsAdmin,
		EmailVerified:            body.EmailVerified,
		NotificationsPaused:      body.NotificationsPaused,
		NotificationsPausedUntil: body.PausedUntil,
	}
	if body.Password != nil && *body.Password != "" {
		hash, err := auth.HashPassword(*body.Password)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		in.PasswordHash = &hash
	}
	user, err := s.db.UpdateUserAdmin(r.Context(), id, in)
	if err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err == storage.ErrConflict {
		writeError(w, http.StatusConflict, "email already registered")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, adminUserPayload(user))
}

func (s *Server) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if id == userIDFromContext(r.Context()) {
		writeError(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}
	if err := s.db.DeleteUser(r.Context(), id); err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err == storage.ErrForbidden {
		writeError(w, http.StatusForbidden, "this admin account is protected")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete user")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleAdminRevokeUserSessions(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if _, err := s.db.GetUserByID(r.Context(), id); err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load user")
		return
	}
	count, err := s.db.DeleteUserSessions(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not revoke sessions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revoked": count})
}

func (s *Server) handleAdminListReminders(w http.ResponseWriter, r *http.Request) {
	f := storage.AdminReminderFilter{
		Status: r.URL.Query().Get("status"),
		Query:  r.URL.Query().Get("q"),
		Limit:  queryInt(r, "limit", 100),
		Offset: queryInt(r, "offset", 0),
	}
	if raw := r.URL.Query().Get("user_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user_id")
			return
		}
		f.UserID = &id
	}
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
	items, err := s.db.AdminListReminders(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list reminders")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, adminReminderPayload(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"reminders": out})
}

func (s *Server) handleAdminCreateReminder(w http.ResponseWriter, r *http.Request) {
	var body adminCreateReminderRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	userID, err := uuid.Parse(body.UserID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user_id")
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
		listID, err := uuid.Parse(*body.ListID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid list_id")
			return
		}
		in.ListID = &listID
	}
	item, err := s.db.AdminCreateReminder(r.Context(), userID, in)
	if err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, adminReminderPayload(item))
}

func (s *Server) handleAdminGetReminder(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	item, err := s.db.AdminGetReminder(r.Context(), id)
	if err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load reminder")
		return
	}
	writeJSON(w, http.StatusOK, adminReminderPayload(item))
}

func (s *Server) handleAdminUpdateReminder(w http.ResponseWriter, r *http.Request) {
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
	item, err := s.db.AdminUpdateReminder(r.Context(), id, in)
	if err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, adminReminderPayload(item))
}

func (s *Server) handleAdminDeleteReminder(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.db.AdminDeleteReminder(r.Context(), id); err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleAdminDoneReminder(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	status := reminder.StatusDone
	item, err := s.db.AdminUpdateReminder(r.Context(), id, reminder.UpdateInput{Status: &status})
	if err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update")
		return
	}
	writeJSON(w, http.StatusOK, adminReminderPayload(item))
}

func (s *Server) handleAdminPauseReminder(w http.ResponseWriter, r *http.Request) {
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
	item, err := s.db.AdminPauseReminder(r.Context(), id, resumeAt)
	if err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not pause")
		return
	}
	writeJSON(w, http.StatusOK, adminReminderPayload(item))
}

func (s *Server) handleAdminResumeReminder(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	item, err := s.db.AdminResumeReminder(r.Context(), id)
	if err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not resume")
		return
	}
	writeJSON(w, http.StatusOK, adminReminderPayload(item))
}

func (s *Server) handleAdminSnoozeReminder(w http.ResponseWriter, r *http.Request) {
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
		cur, err := s.db.AdminGetReminder(r.Context(), id)
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
	item, err := s.db.AdminSnoozeReminder(r.Context(), id, until)
	if err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not snooze")
		return
	}
	writeJSON(w, http.StatusOK, adminReminderPayload(item))
}

func adminUserPayload(u storage.User) map[string]any {
	out := userPayload(u)
	out["is_admin"] = u.IsAdmin
	out["created_at"] = u.CreatedAt.UTC().Format(time.RFC3339)
	out["updated_at"] = u.UpdatedAt.UTC().Format(time.RFC3339)
	return out
}

func adminReminderPayload(r reminder.Reminder) map[string]any {
	out := reminderPayload(r)
	out["user_id"] = r.UserID.String()
	return out
}

func queryInt(r *http.Request, key string, fallback int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}
