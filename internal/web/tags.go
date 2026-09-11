package web

import (
	"net/http"
	"time"

	"github.com/shivam-20/day-remind/internal/storage"
)

type tagCreateRequest struct {
	Name string `json:"name"`
}

type tagUpdateRequest struct {
	Name *string `json:"name"`
}

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	items, err := s.db.ListTags(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list tags")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, tagPayload(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": out})
}

func (s *Server) handleCreateTag(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	var body tagCreateRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	item, err := s.db.CreateTag(r.Context(), userID, storage.TagCreateInput{Name: body.Name})
	if err == storage.ErrConflict {
		writeError(w, http.StatusConflict, "tag already exists")
		return
	}
	if writeStorageErr(w, err, "not found") {
		return
	}
	writeJSON(w, http.StatusCreated, tagPayload(item))
}

func (s *Server) handleUpdateTag(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}
	var body tagUpdateRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	item, err := s.db.UpdateTag(r.Context(), userID, id, storage.TagUpdateInput{Name: body.Name})
	if err == storage.ErrConflict {
		writeError(w, http.StatusConflict, "tag already exists")
		return
	}
	if writeStorageErr(w, err, "not found") {
		return
	}
	writeJSON(w, http.StatusOK, tagPayload(item))
}

func (s *Server) handleDeleteTag(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}
	if err := s.db.DeleteTag(r.Context(), userID, id); err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete tag")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func tagPayload(t storage.Tag) map[string]any {
	return map[string]any{
		"id":         t.ID.String(),
		"name":       t.Name,
		"created_at": t.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at": t.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
