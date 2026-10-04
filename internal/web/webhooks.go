package web

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/shivam-20/day-remind/internal/storage"
	"github.com/shivam-20/day-remind/internal/webhook"
)

const minWebhookSecretLen = 16

type webhookCreateRequest struct {
	URL    string   `json:"url"`
	Secret string   `json:"secret"`
	Events []string `json:"events"`
}

type webhookUpdateRequest struct {
	URL          *string  `json:"url"`
	Secret       *string  `json:"secret"`
	RotateSecret bool     `json:"rotate_secret"`
	Events       []string `json:"events"`
	Enabled      *bool    `json:"enabled"`
}

// webhookSecret returns a caller-chosen secret (validated) or a generated one.
func webhookSecret(custom string) (string, error) {
	custom = strings.TrimSpace(custom)
	if custom == "" {
		return webhook.GenerateSecret()
	}
	if len(custom) < minWebhookSecretLen {
		return "", fmt.Errorf("secret must be at least %d characters", minWebhookSecretLen)
	}
	return custom, nil
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
	writeJSON(w, http.StatusOK, map[string]any{"webhooks": out, "events": webhook.Events})
}

func (s *Server) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	var body webhookCreateRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	url, err := webhook.ValidateURL(body.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	events, err := webhook.ValidateEvents(body.Events)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	secret, err := webhookSecret(body.Secret)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	item, err := s.db.CreateWebhook(r.Context(), userID, url, secret, events)
	if writeStorageErr(w, err, "not found") {
		return
	}
	// The secret is only ever returned here and on rotation.
	out := webhookPayload(item)
	out["secret"] = item.Secret
	writeJSON(w, http.StatusCreated, out)
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
	in := storage.WebhookUpdate{Enabled: body.Enabled}
	if body.URL != nil {
		url, err := webhook.ValidateURL(*body.URL)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		in.URL = &url
	}
	if body.Events != nil {
		events, err := webhook.ValidateEvents(body.Events)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		in.Events = events
	}
	if body.RotateSecret || body.Secret != nil {
		custom := ""
		if body.Secret != nil {
			custom = *body.Secret
		}
		secret, err := webhookSecret(custom)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		in.Secret = &secret
	}
	item, err := s.db.UpdateWebhook(r.Context(), userID, id, in)
	if writeStorageErr(w, err, "not found") {
		return
	}
	out := webhookPayload(item)
	if in.Secret != nil {
		out["secret"] = item.Secret
	}
	writeJSON(w, http.StatusOK, out)
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

// handleTestWebhook sends a signed "ping" event right away and reports the result.
func (s *Server) handleTestWebhook(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}
	if s.hooks == nil {
		writeError(w, http.StatusServiceUnavailable, "webhooks are not enabled")
		return
	}
	item, err := s.db.GetWebhook(r.Context(), userID, id)
	if writeStorageErr(w, err, "not found") {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	code, sendErr := s.hooks.Ping(ctx, item)
	out := map[string]any{"ok": sendErr == nil, "status_code": code}
	if sendErr != nil {
		out["error"] = sendErr.Error()
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleListWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}
	if _, err := s.db.GetWebhook(r.Context(), userID, id); writeStorageErr(w, err, "not found") {
		return
	}
	items, err := s.db.ListWebhookDeliveries(r.Context(), userID, id, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list deliveries")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, d := range items {
		row := map[string]any{
			"id":            d.ID.String(),
			"event":         d.Event,
			"status":        d.Status,
			"attempt_count": d.AttemptCount,
			"created_at":    d.CreatedAt.UTC().Format(time.RFC3339),
			"updated_at":    d.UpdatedAt.UTC().Format(time.RFC3339),
		}
		if d.ResponseCode != 0 {
			row["response_code"] = d.ResponseCode
		}
		if d.LastError != "" {
			row["last_error"] = d.LastError
		}
		if d.Status == storage.WebhookDeliveryRetry {
			row["next_attempt_at"] = d.NextAttemptAt.UTC().Format(time.RFC3339)
		}
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"deliveries": out})
}

// webhookPayload never includes the signing secret.
func webhookPayload(h storage.Webhook) map[string]any {
	out := map[string]any{
		"id":                   h.ID.String(),
		"url":                  h.URL,
		"events":               h.Events,
		"enabled":              h.Enabled,
		"has_secret":           h.Secret != "",
		"consecutive_failures": h.ConsecutiveFailures,
		"created_at":           h.CreatedAt.UTC().Format(time.RFC3339),
	}
	if h.DisabledReason != "" {
		out["disabled_reason"] = h.DisabledReason
	}
	if h.LastDeliveryAt != nil {
		out["last_delivery_at"] = h.LastDeliveryAt.UTC().Format(time.RFC3339)
		out["last_status_code"] = h.LastStatusCode
		if h.LastError != "" {
			out["last_error"] = h.LastError
		}
	}
	return out
}
