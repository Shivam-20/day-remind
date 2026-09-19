package web

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/storage"
)

var errLastAdmin = errors.New("this is the only admin account; promote another admin first")

// adminUserPayload reports on a user's activity without exposing any reminder
// content. Titles and descriptions are private to their owner.
func (s *Server) adminUserPayload(u storage.AdminUser) map[string]any {
	out := map[string]any{
		"id":                   u.ID.String(),
		"email":                u.Email,
		"display_name":         u.DisplayName,
		"role":                 u.Role,
		"timezone":             u.Timezone,
		"notifications_paused": u.NotificationsPaused,
		"pinned_admin":         s.cfg.IsAdminEmail(u.Email),
		"created_at":           u.CreatedAt.UTC().Format(time.RFC3339),
		"stats": map[string]any{
			"total":     u.Stats.Total,
			"scheduled": u.Stats.Scheduled,
			"paused":    u.Stats.Paused,
			"done":      u.Stats.Done,
			"cancelled": u.Stats.Cancelled,
			"overdue":   u.Stats.Overdue,
		},
	}
	if u.NotificationsPausedUntil != nil {
		out["notifications_paused_until"] = u.NotificationsPausedUntil.UTC().Format(time.RFC3339)
	}
	if u.Stats.NextDueAt != nil {
		out["next_due_at"] = u.Stats.NextDueAt.UTC().Format(time.RFC3339)
	}
	return out
}

func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.db.InstanceStatsFor(r.Context())
	if err != nil {
		writeStorageError(w, err, "could not load stats")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"users":             st.Users,
		"admins":            st.Admins,
		"users_paused":      st.UsersPaused,
		"reminders":         st.Reminders,
		"reminders_open":    st.RemindersOpen,
		"reminders_done":    st.RemindersDone,
		"reminders_overdue": st.RemindersDueNow,
		"active_sessions":   st.ActiveSessions,
	})
}

func (s *Server) handleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.db.ListUsersForAdmin(r.Context())
	if err != nil {
		writeStorageError(w, err, "could not list users")
		return
	}
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		out = append(out, s.adminUserPayload(u))
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": out})
}

// adminTarget resolves the {id} path parameter to a user, and reports whether
// that user is the admin making the request.
func (s *Server) adminTarget(w http.ResponseWriter, r *http.Request) (storage.User, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return storage.User{}, false
	}
	user, err := s.db.GetUserByID(r.Context(), id)
	if err != nil {
		writeStorageError(w, err, "could not load user")
		return storage.User{}, false
	}
	return user, true
}

func (s *Server) handleAdminSetRole(w http.ResponseWriter, r *http.Request) {
	actor := adminFromContext(r.Context())
	target, ok := s.adminTarget(w, r)
	if !ok {
		return
	}
	var body struct {
		Role string `json:"role"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !storage.ValidRole(body.Role) {
		writeError(w, http.StatusBadRequest, "role must be user or admin")
		return
	}
	if body.Role == storage.RoleUser {
		// ADMIN_EMAILS wins on the next login, so allowing the demotion here
		// would only produce a role that silently comes back.
		if s.cfg.IsAdminEmail(target.Email) {
			writeError(w, http.StatusConflict, "this address is pinned to admin by ADMIN_EMAILS")
			return
		}
		if err := s.guardLastAdmin(r.Context(), target); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
	}
	updated, err := s.db.SetUserRole(r.Context(), target.ID, body.Role)
	if err != nil {
		writeStorageError(w, err, "could not change role")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":           updated.ID.String(),
		"email":        updated.Email,
		"role":         updated.Role,
		"self":         updated.ID == actor.ID,
		"demoted_self": updated.ID == actor.ID && updated.Role != storage.RoleAdmin,
	})
}

func (s *Server) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	actor := adminFromContext(r.Context())
	target, ok := s.adminTarget(w, r)
	if !ok {
		return
	}
	// Deleting yourself would end the session performing the delete.
	if target.ID == actor.ID {
		writeError(w, http.StatusConflict, "you cannot delete your own account here")
		return
	}
	if target.Role == storage.RoleAdmin {
		if err := s.guardLastAdmin(r.Context(), target); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
	}
	if err := s.db.DeleteUser(r.Context(), target.ID); err != nil {
		writeStorageError(w, err, "could not delete user")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "email": target.Email})
}

func (s *Server) handleAdminPauseUser(w http.ResponseWriter, r *http.Request) {
	target, ok := s.adminTarget(w, r)
	if !ok {
		return
	}
	var body struct {
		Until *time.Time `json:"until"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.Until != nil && body.Until.Before(time.Now().UTC()) {
		writeError(w, http.StatusBadRequest, "until must be in the future")
		return
	}
	if err := s.db.SetNotificationsPaused(r.Context(), target.ID, true, body.Until); err != nil {
		writeStorageError(w, err, "could not pause notifications")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "paused", "id": target.ID.String()})
}

func (s *Server) handleAdminResumeUser(w http.ResponseWriter, r *http.Request) {
	target, ok := s.adminTarget(w, r)
	if !ok {
		return
	}
	if err := s.db.SetNotificationsPaused(r.Context(), target.ID, false, nil); err != nil {
		writeStorageError(w, err, "could not resume notifications")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "active", "id": target.ID.String()})
}

// guardLastAdmin refuses an action that would remove the final admin and leave
// the panel unreachable by anyone.
func (s *Server) guardLastAdmin(ctx context.Context, target storage.User) error {
	if target.Role != storage.RoleAdmin {
		return nil
	}
	n, err := s.db.CountAdmins(ctx)
	if err != nil {
		return err
	}
	if n <= 1 {
		return errLastAdmin
	}
	return nil
}
