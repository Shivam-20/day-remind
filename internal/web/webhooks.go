package web

import (
	"net/http"

	"github.com/shivam-20/day-remind/internal/storage"
)

type webhookCreateRequest struct {
	URL    string   `json:"url"`
	Secret string   `json:"secret"`
	Events []string `json:"events"`
}

type webhookUpdateRequest struct {
	URL     *string  `json:"url"`
	Secret  *string  `json:"secret"`
	Events  []string `json:"events"`
	Enabled *bool    `json:"enabled"`
}

func (s *Server) handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	items, err := s.db.ListWebhooks(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list webhooks")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, webhookPayload(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"webhooks": out})
}

func (s *Server) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	var body webhookCreateRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	item, err := s.db.CreateWebhook(r.Context(), userID, body.URL, body.Secret, body.Events)
	if writeStorageErr(w, err, "not found") {
		return
	}
	writeJSON(w, http.StatusCreated, webhookPayload(item))
}

func (s *Server) handleGetWebhook(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}
	item, err := s.db.GetWebhook(r.Context(), userID, id)
	if writeStorageErr(w, err, "not found") {
		return
	}
	writeJSON(w, http.StatusOK, webhookPayload(item))
}

func (s *Server) handleUpdateWebhook(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}
	var body webhookUpdateRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	item, err := s.db.UpdateWebhook(r.Context(), userID, id, storage.WebhookUpdate{
		URL: body.URL, Secret: body.Secret, Events: body.Events, Enabled: body.Enabled,
	})
	if writeStorageErr(w, err, "not found") {
		return
	}
	writeJSON(w, http.StatusOK, webhookPayload(item))
}

func (s *Server) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}
	if err := s.db.DeleteWebhook(r.Context(), userID, id); err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete webhook")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func webhookPayload(h storage.Webhook) map[string]any {
	return map[string]any{
		"id":      h.ID.String(),
		"url":     h.URL,
		"events":  h.Events,
		"enabled": h.Enabled,
	}
}
