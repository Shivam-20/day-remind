package web

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/auth"
	"github.com/shivam-20/day-remind/internal/storage"
)

const sessionCookieName = "dr_session"

type signupRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Timezone    string `json:"timezone"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	var body signupRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	user, err := s.db.CreateUser(r.Context(), body.Email, hash, body.DisplayName, body.Timezone)
	if errors.Is(err, storage.ErrConflict) {
		writeError(w, http.StatusConflict, "email already registered")
		return
	}
	if err != nil {
		writeStorageError(w, err, "could not create user")
		return
	}
	user = s.syncAdminRole(r.Context(), user)
	if err := s.setSession(w, r, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}
	writeJSON(w, http.StatusCreated, userPayload(user))
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body loginRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	user, err := s.db.GetUserByEmail(r.Context(), body.Email)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "could not sign in")
		return
	}
	if err != nil || !auth.CheckPassword(user.PasswordHash, body.Password) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	user = s.syncAdminRole(r.Context(), user)
	if err := s.setSession(w, r, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}
	writeJSON(w, http.StatusOK, userPayload(user))
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	token := sessionTokenFromRequest(r)
	if token != "" {
		_ = s.db.DeleteSession(r.Context(), token)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	user, err := s.db.GetUserByID(r.Context(), userID)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load profile")
		return
	}
	writeJSON(w, http.StatusOK, userPayload(user))
}

func (s *Server) handlePauseNotifications(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
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
	if err := s.db.SetNotificationsPaused(r.Context(), userID, true, body.Until); err != nil {
		writeError(w, http.StatusInternalServerError, "could not pause notifications")
		return
	}
	s.handleMe(w, r)
}

func (s *Server) handleResumeNotifications(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	if err := s.db.SetNotificationsPaused(r.Context(), userID, false, nil); err != nil {
		writeError(w, http.StatusInternalServerError, "could not resume notifications")
		return
	}
	s.handleMe(w, r)
}

func (s *Server) setSession(w http.ResponseWriter, r *http.Request, userID uuid.UUID) error {
	token, err := s.db.CreateSession(r.Context(), userID)
	if err != nil {
		return err
	}
	secure := strings.HasPrefix(r.Header.Get("X-Forwarded-Proto"), "https") || r.TLS != nil
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int((30 * 24 * time.Hour).Seconds()),
	})
	return nil
}

func userPayload(u storage.User) map[string]any {
	out := map[string]any{
		"id":                   u.ID.String(),
		"email":                u.Email,
		"role":                 u.Role,
		"display_name":         u.DisplayName,
		"timezone":             u.Timezone,
		"locale":               u.Locale,
		"notifications_paused": u.NotificationsPaused,
	}
	if u.NotificationsPausedUntil != nil {
		out["notifications_paused_until"] = u.NotificationsPausedUntil.UTC().Format(time.RFC3339)
	}
	return out
}

// syncAdminRole promotes a user listed in ADMIN_EMAILS. It runs on signup and
// login so the allowlist can be changed without touching the database, and it
// only ever grants: removing an address from the list leaves the stored role
// alone, so an admin is never silently demoted mid-session by a config edit.
func (s *Server) syncAdminRole(ctx context.Context, user storage.User) storage.User {
	if !s.cfg.IsAdminEmail(user.Email) || user.Role == storage.RoleAdmin {
		return user
	}
	updated, err := s.db.SetUserRole(ctx, user.ID, storage.RoleAdmin)
	if err != nil {
		// A failed promotion must not block sign-in; the user simply lands
		// without the panel and the next login retries.
		log.Printf("admin bootstrap: could not promote %s: %v", user.Email, err)
		return user
	}
	return updated
}
