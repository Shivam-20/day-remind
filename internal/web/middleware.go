package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/storage"
)

type contextKey string

const userIDKey contextKey = "userID"
const adminUserKey contextKey = "adminUser"

func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := sessionTokenFromRequest(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		userID, err := s.db.GetSessionUserID(r.Context(), token)
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if err != nil {
			// A database outage is not an authentication failure; reporting it
			// as one would log every user out instead of surfacing the fault.
			writeError(w, http.StatusServiceUnavailable, "service unavailable")
			return
		}
		ctx := context.WithValue(r.Context(), userIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func userIDFromContext(ctx context.Context) uuid.UUID {
	v, _ := ctx.Value(userIDKey).(uuid.UUID)
	return v
}

func sessionTokenFromRequest(r *http.Request) string {
	if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
		return c.Value
	}
	return ""
}

// requireAdmin runs after requireSession and rejects anyone whose stored role
// is not admin. The role is read from the database on every request rather
// than trusted from the session, so a demotion takes effect immediately.
func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := s.db.GetUserByID(r.Context(), userIDFromContext(r.Context()))
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "service unavailable")
			return
		}
		if user.Role != storage.RoleAdmin {
			// Deliberately 404, not 403: a non-admin is not told the panel exists.
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), adminUserKey, user)))
	})
}

func adminFromContext(ctx context.Context) storage.User {
	v, _ := ctx.Value(adminUserKey).(storage.User)
	return v
}
