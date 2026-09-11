package web

import (
	"fmt"
	"net/http"
)

type digestSettingsRequest struct {
	Enabled *bool   `json:"morning_digest_enabled"`
	Time    *string `json:"morning_digest_time"`
}

func (s *Server) handleUpdateDigestSettings(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	settings, err := s.db.LoadUserSettings(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load settings")
		return
	}
	var body digestSettingsRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	enabled := settings.MorningDigestEnabled
	digestTime := settings.MorningDigestTime
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	if body.Time != nil {
		digestTime = *body.Time
	}
	if err := s.db.UpdateMorningDigest(r.Context(), userID, enabled, digestTime); err != nil {
		writeError(w, http.StatusInternalServerError, "could not update settings")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"morning_digest_enabled": enabled,
		"morning_digest_time":    digestTime,
	})
}

func (s *Server) handleGetEmailIngestAddress(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	token, err := s.db.EnsureEmailIngestToken(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load ingest token")
		return
	}
	address := fmt.Sprintf("remind+%s@ingest.dayremind.app", token)
	writeJSON(w, http.StatusOK, map[string]any{
		"address": address,
		"token":   token,
	})
}
