package web

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/storage"
)

func (s *Server) handleListReminderDeliveries(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if _, err := s.db.GetReminder(r.Context(), userID, id); err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load reminder")
		return
	}
	items, err := s.db.ListDeliveriesForReminder(r.Context(), userID, id, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list deliveries")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, d := range items {
		out = append(out, deliveryPayload(d))
	}
	writeJSON(w, http.StatusOK, map[string]any{"deliveries": out})
}

func (s *Server) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	items, err := s.db.ListRecentDeliveriesForUser(r.Context(), userID, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list notifications")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, d := range items {
		out = append(out, deliveryPayload(d))
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": out})
}

func deliveryPayload(d storage.Delivery) map[string]any {
	out := map[string]any{
		"id":          d.ID.String(),
		"reminder_id": d.ReminderID.String(),
		"channel":     d.Channel,
		"status":      d.Status,
		"attempts":    d.AttemptCount,
		"created_at":  d.CreatedAt.UTC().Format(time.RFC3339),
	}
	if d.LastError != "" {
		out["last_error"] = d.LastError
	}
	if d.SentAt != nil {
		out["sent_at"] = d.SentAt.UTC().Format(time.RFC3339)
	}
	return out
}
