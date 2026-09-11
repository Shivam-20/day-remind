package web

import (
	"net/http"
	"time"

	"github.com/shivam-20/day-remind/internal/storage"
)

type apiKeyCreateRequest struct {
	Name string `json:"name"`
}

func (s *Server) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	items, err := s.db.ListAPIKeys(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list api keys")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, apiKeyPayload(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"api_keys": out})
}

func (s *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	var body apiKeyCreateRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	item, raw, err := s.db.CreateAPIKey(r.Context(), userID, body.Name)
	if writeStorageErr(w, err, "not found") {
		return
	}
	out := apiKeyPayload(item)
	out["token"] = raw
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}
	if err := s.db.RevokeAPIKey(r.Context(), userID, id); err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not revoke api key")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func apiKeyPayload(k storage.APIKey) map[string]any {
	out := map[string]any{
		"id":         k.ID.String(),
		"name":       k.Name,
		"prefix":     k.Prefix,
		"created_at": k.CreatedAt.UTC().Format(time.RFC3339),
	}
	if k.LastUsedAt != nil {
		out["last_used_at"] = k.LastUsedAt.UTC().Format(time.RFC3339)
	}
	return out
}
