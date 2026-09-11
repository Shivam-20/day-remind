package web

import (
	"net/http"
	"strings"
	"time"

	"github.com/shivam-20/day-remind/internal/reminder"
	"github.com/shivam-20/day-remind/internal/storage"
)

type templateCreateRequest struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
	Offsets     []int  `json:"offsets"`
}

type templateUseRequest struct {
	DueAt     string            `json:"due_at"`
	Timezone  string            `json:"timezone"`
	ListID    *string           `json:"list_id"`
	Variables map[string]string `json:"variables"`
}

func (s *Server) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	items, err := s.db.ListTemplates(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list templates")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, templatePayload(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": out})
}

func (s *Server) handleCreateTemplate(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	var body templateCreateRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	item, err := s.db.CreateTemplate(r.Context(), userID, body.Name, body.Title, body.Description, body.Priority, body.Offsets)
	if writeStorageErr(w, err, "not found") {
		return
	}
	writeJSON(w, http.StatusCreated, templatePayload(item))
}

func (s *Server) handleUseTemplate(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	templateID, ok := parseURLUUID(w, r, "id")
	if !ok {
		return
	}
	var body templateUseRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.DueAt == "" {
		writeError(w, http.StatusBadRequest, "due_at is required")
		return
	}
	dueAt, err := time.Parse(time.RFC3339, body.DueAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "due_at must be RFC3339")
		return
	}

	tmpl, err := s.db.GetTemplate(r.Context(), userID, templateID)
	if err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load template")
		return
	}

	title := applyTemplateVars(tmpl.Title, body.Variables)
	desc := applyTemplateVars(tmpl.Description, body.Variables)
	in := reminder.CreateInput{
		Title:       title,
		Description: desc,
		DueAt:       dueAt,
		Timezone:    body.Timezone,
		Priority:    tmpl.Priority,
	}
	if body.ListID != nil && *body.ListID != "" {
		listID, err := parseOptionalUUID(*body.ListID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid list_id")
			return
		}
		in.ListID = listID
	}

	item, err := s.db.CreateReminder(r.Context(), userID, in)
	if writeStorageErr(w, err, "not found") {
		return
	}
	for _, minutes := range tmpl.Offsets {
		_, _ = s.db.CreateOffset(r.Context(), userID, item.ID, storage.OffsetCreateInput{
			OffsetMinutes: minutes,
		})
	}
	writeJSON(w, http.StatusCreated, reminderPayload(item))
}

func applyTemplateVars(text string, vars map[string]string) string {
	out := text
	for key, val := range vars {
		out = strings.ReplaceAll(out, "{{"+key+"}}", val)
	}
	return out
}

func templatePayload(t storage.Template) map[string]any {
	out := map[string]any{
		"id":          t.ID.String(),
		"name":        t.Name,
		"title":       t.Title,
		"description": t.Description,
		"priority":    t.Priority,
		"offsets":     t.Offsets,
		"is_system":   t.IsSystem,
	}
	if t.UserID != nil {
		out["user_id"] = t.UserID.String()
	}
	return out
}
