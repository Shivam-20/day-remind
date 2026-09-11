package web

import (
	"net/http"
)

func (s *Server) handleDashboardStats(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	stats, err := s.db.GetDashboardStats(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load stats")
		return
	}
	nextFive := make([]map[string]any, 0, len(stats.NextFive))
	for _, item := range stats.NextFive {
		nextFive = append(nextFive, reminderPayload(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"overdue":     stats.Overdue,
		"today":       stats.Today,
		"upcoming_7d": stats.Upcoming7d,
		"done_count":  stats.DoneCount,
		"next_five":   nextFive,
	})
}
