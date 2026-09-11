package web

import (
	"net/http"

	"github.com/shivam-20/day-remind/internal/storage"
)

type shareCreateRequest struct {
	Email      string `json:"email"`
	Permission string `json:"permission"`
}

func (s *Server) handleCreateShare(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	reminderID, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}
	var body shareCreateRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	item, err := s.db.CreateShare(r.Context(), userID, reminderID, body.Email, body.Permission)
	if err == storage.ErrConflict {
		writeError(w, http.StatusConflict, "share already exists")
		return
	}
	if writeStorageErr(w, err, "not found") {
		return
	}
	writeJSON(w, http.StatusCreated, sharePayload(item))
}

func (s *Server) handleListShares(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	reminderID, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}
	items, err := s.db.ListSharesForReminder(r.Context(), userID, reminderID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list shares")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, sharePayload(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"shares": out})
}

func (s *Server) handleAcceptShare(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	shareID, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}
	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	item, err := s.db.AcceptShare(r.Context(), shareID, user.Email)
	if writeStorageErr(w, err, "not found") {
		return
	}
	writeJSON(w, http.StatusOK, sharePayload(item))
}

func sharePayload(sh storage.ReminderShare) map[string]any {
	return map[string]any{
		"id":                sh.ID.String(),
		"reminder_id":       sh.ReminderID.String(),
		"owner_id":          sh.OwnerID.String(),
		"shared_with_email": sh.SharedWithEmail,
		"permission":        sh.Permission,
		"accepted":          sh.Accepted,
	}
}
