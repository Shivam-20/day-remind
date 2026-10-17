package web

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/storage"
)

type subtaskCreateRequest struct {
	Title     string `json:"title"`
	SortOrder int    `json:"sort_order"`
}

type subtaskUpdateRequest struct {
	Title     *string `json:"title"`
	Done      *bool   `json:"done"`
	SortOrder *int    `json:"sort_order"`
}

type subtaskReorderRequest struct {
	Order []string `json:"order"`
}

func (s *Server) handleListSubtasks(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	reminderID, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}
	items, err := s.db.ListSubtasks(r.Context(), userID, reminderID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list subtasks")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, subtaskPayload(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"subtasks": out})
}

func (s *Server) handleCreateSubtask(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	reminderID, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}
	var body subtaskCreateRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	item, err := s.db.CreateSubtask(r.Context(), userID, reminderID, storage.SubtaskCreateInput{
		Title: body.Title, SortOrder: body.SortOrder,
	})
	if writeStorageErr(w, err, "not found") {
		return
	}
	writeJSON(w, http.StatusCreated, subtaskPayload(item))
}

func (s *Server) handleUpdateSubtask(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	subID, ok := parseURLUUID(w, r, "subId")
	if !ok {
		return
	}
	var body subtaskUpdateRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	item, err := s.db.UpdateSubtask(r.Context(), userID, subID, storage.SubtaskUpdateInput{
		Title: body.Title, Done: body.Done, SortOrder: body.SortOrder,
	})
	if writeStorageErr(w, err, "not found") {
		return
	}
	writeJSON(w, http.StatusOK, subtaskPayload(item))
}

func (s *Server) handleDeleteSubtask(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	subID, ok := parseURLUUID(w, r, "subId")
	if !ok {
		return
	}
	if err := s.db.DeleteSubtask(r.Context(), userID, subID); err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete subtask")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleReorderSubtasks(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	reminderID, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}
	var body subtaskReorderRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(body.Order) == 0 {
		writeError(w, http.StatusBadRequest, "order is required")
		return
	}
	ordered := make([]uuid.UUID, 0, len(body.Order))
	for _, raw := range body.Order {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid order id")
			return
		}
		ordered = append(ordered, id)
	}
	if err := s.db.ReorderSubtasks(r.Context(), userID, reminderID, ordered); err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not reorder subtasks")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func subtaskPayload(st storage.Subtask) map[string]any {
	return map[string]any{
		"id":          st.ID.String(),
		"reminder_id": st.ReminderID.String(),
		"title":       st.Title,
		"done":        st.Done,
		"sort_order":  st.SortOrder,
		"created_at":  st.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":  st.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
