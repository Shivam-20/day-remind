package web

import (
	"encoding/json"
	"net/http"
	"strings"
)

type pushSubscribeRequest struct {
	Endpoint string          `json:"endpoint"`
	Keys     json.RawMessage `json:"keys"`
}

func (s *Server) handlePushSubscribe(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	var body pushSubscribeRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	keys := strings.TrimSpace(string(body.Keys))
	if keys == "" || keys == "null" {
		writeError(w, http.StatusBadRequest, "keys are required")
		return
	}
	item, err := s.db.SavePushSubscription(r.Context(), userID, body.Endpoint, keys)
	if writeStorageErr(w, err, "not found") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"endpoint": item.Endpoint,
		"status":   "subscribed",
	})
}
