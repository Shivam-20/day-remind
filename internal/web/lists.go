package web

import (
	"net/http"
	"time"

	"github.com/shivam-20/day-remind/internal/storage"
)

type listCreateRequest struct {
	Name      string `json:"name"`
	Color     string `json:"color"`
	SortOrder int    `json:"sort_order"`
}

type listUpdateRequest struct {
	Name      *string `json:"name"`
	Color     *string `json:"color"`
	SortOrder *int    `json:"sort_order"`
}

func (s *Server) handleListLists(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	items, err := s.db.ListLists(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list lists")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, listPayload(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"lists": out})
}

func (s *Server) handleCreateList(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	var body listCreateRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	item, err := s.db.CreateList(r.Context(), userID, storage.ListCreateInput{
		Name: body.Name, Color: body.Color, SortOrder: body.SortOrder,
	})
	if writeStorageErr(w, err, "not found") {
		return
	}
	writeJSON(w, http.StatusCreated, listPayload(item))
}

func (s *Server) handleGetList(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}
	item, err := s.db.GetListByID(r.Context(), userID, id)
	if err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load list")
		return
	}
	writeJSON(w, http.StatusOK, listPayload(item))
}

func (s *Server) handleUpdateList(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}
	var body listUpdateRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	item, err := s.db.UpdateList(r.Context(), userID, id, storage.ListUpdateInput{
		Name: body.Name, Color: body.Color, SortOrder: body.SortOrder,
	})
	if writeStorageErr(w, err, "not found") {
		return
	}
	writeJSON(w, http.StatusOK, listPayload(item))
}

func (s *Server) handleDeleteList(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}
	if err := s.db.DeleteList(r.Context(), userID, id); err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete list")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handlePauseList(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}
	item, err := s.db.PauseList(r.Context(), userID, id)
	if writeStorageErr(w, err, "not found") {
		return
	}
	writeJSON(w, http.StatusOK, listPayload(item))
}

func (s *Server) handleResumeList(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	id, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}
	item, err := s.db.ResumeList(r.Context(), userID, id)
	if writeStorageErr(w, err, "not found") {
		return
	}
	writeJSON(w, http.StatusOK, listPayload(item))
}

func listPayload(l storage.List) map[string]any {
	return map[string]any{
		"id":         l.ID.String(),
		"name":       l.Name,
		"color":      l.Color,
		"sort_order": l.SortOrder,
		"paused":     l.Paused,
		"created_at": l.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at": l.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
