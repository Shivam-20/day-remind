package web

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/storage"
)

type contextKey string

const userIDKey contextKey = "userID"

func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Primary path: day-remind's own session cookie (opaque token in Mongo).
		if token := sessionTokenFromRequest(r); token != "" {
			if userID, err := s.db.GetSessionUserID(r.Context(), token); err == nil {
				s.syncAdminFromHeaders(r, userID)
				ctx := context.WithValue(r.Context(), userIDKey, userID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		// Fallback: Caddy forward_auth identity headers (Launchpad SSO). Checked
		// before the JWT because /auth/verify reads the live role from the IdP,
		// whereas the cookie's role claim is frozen at sign-in.
		if userID, ok := s.userFromHeaders(r); ok {
			ctx := context.WithValue(r.Context(), userIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		// Fallback: shared gateway JWT (auth_token cookie) from the home page.
		if userID, ok := s.userFromGateway(r); ok {
			ctx := context.WithValue(r.Context(), userIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		writeError(w, http.StatusUnauthorized, "unauthorized")
	})
}

func (s *Server) requireAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(authHeader, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		if !strings.HasPrefix(token, "dr_") {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		user, err := s.db.GetUserByAPIKeyHash(r.Context(), storage.HashAPIKeyToken(token))
		if err == storage.ErrNotFound {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not authenticate")
			return
		}
		ctx := context.WithValue(r.Context(), userIDKey, user.ID)
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

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := userIDFromContext(r.Context())
		user, err := s.db.GetUserByID(r.Context(), userID)
		if err != nil || !user.IsAdmin {
			writeError(w, http.StatusForbidden, "admin required")
			return
		}
		next.ServeHTTP(w, r)
	})
}
