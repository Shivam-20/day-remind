package web

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/notify"
	"github.com/shivam-20/day-remind/internal/storage"
)

type mailTemplateRequest struct {
	Name              string `json:"name"`
	EmailSubject      string `json:"email_subject"`
	EmailHTML         string `json:"email_html"`
	EmailCSS          string `json:"email_css"`
	EmailText         string `json:"email_text"`
	TelegramText      string `json:"telegram_text"`
	TelegramParseMode string `json:"telegram_parse_mode"`
}

func (body mailTemplateRequest) set() notify.TemplateSet {
	return notify.TemplateSet{
		EmailSubject:      body.EmailSubject,
		EmailHTML:         body.EmailHTML,
		EmailCSS:          body.EmailCSS,
		EmailText:         body.EmailText,
		TelegramText:      body.TelegramText,
		TelegramParseMode: body.TelegramParseMode,
	}
}

func mailLibraryPayload(items []storage.MailTemplate, active *uuid.UUID) map[string]any {
	out := make([]map[string]any, 0, len(items))
	activeID := ""
	if active != nil {
		activeID = active.String()
	}
	for _, item := range items {
		row := templatePayload(item.Set)
		row["id"] = item.ID.String()
		row["name"] = item.Name
		row["active"] = active != nil && *active == item.ID
		out = append(out, row)
	}
	return map[string]any{"templates": out, "active_id": activeID}
}

func mailTemplateID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return uuid.UUID{}, false
	}
	return id, true
}

func decodeMailTemplate(w http.ResponseWriter, r *http.Request) (mailTemplateRequest, bool) {
	var body mailTemplateRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return mailTemplateRequest{}, false
	}
	return body, true
}

func (s *Server) handleListOwnMailTemplates(w http.ResponseWriter, r *http.Request) {
	user, err := s.db.GetUserByID(r.Context(), userIDFromContext(r.Context()))
	if err != nil {
		writeStorageError(w, err, "could not load mail templates")
		return
	}
	writeJSON(w, http.StatusOK, mailLibraryPayload(user.MailTemplates, user.ActiveMailTemplateID))
}

func (s *Server) handleCreateOwnMailTemplate(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeMailTemplate(w, r)
	if !ok {
		return
	}
	item, err := s.db.CreateUserMailTemplate(r.Context(), userIDFromContext(r.Context()), body.Name, body.set())
	if err != nil {
		writeStorageError(w, err, "could not save the mail template")
		return
	}
	row := templatePayload(item.Set)
	row["id"] = item.ID.String()
	row["name"] = item.Name
	writeJSON(w, http.StatusCreated, row)
}

func (s *Server) handleUpdateOwnMailTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := mailTemplateID(w, r)
	if !ok {
		return
	}
	body, ok := decodeMailTemplate(w, r)
	if !ok {
		return
	}
	item, err := s.db.UpdateUserMailTemplate(r.Context(), userIDFromContext(r.Context()), id, body.Name, body.set())
	if err != nil {
		writeStorageError(w, err, "could not save the mail template")
		return
	}
	row := templatePayload(item.Set)
	row["id"] = item.ID.String()
	row["name"] = item.Name
	writeJSON(w, http.StatusOK, row)
}

func (s *Server) handleDeleteOwnMailTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := mailTemplateID(w, r)
	if !ok {
		return
	}
	if err := s.db.DeleteUserMailTemplate(r.Context(), userIDFromContext(r.Context()), id); err != nil {
		writeStorageError(w, err, "could not delete the mail template")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUseOwnMailTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := mailTemplateID(w, r)
	if !ok {
		return
	}
	user, err := s.db.UseUserMailTemplate(r.Context(), userIDFromContext(r.Context()), id)
	if err != nil {
		writeStorageError(w, err, "could not use the mail template")
		return
	}
	writeJSON(w, http.StatusOK, mailLibraryPayload(user.MailTemplates, user.ActiveMailTemplateID))
}

func (s *Server) handleListAdminMailTemplates(w http.ResponseWriter, r *http.Request) {
	inst, err := s.db.GetInstanceSettings(r.Context())
	if err != nil {
		writeStorageError(w, err, "could not load mail templates")
		return
	}
	writeJSON(w, http.StatusOK, mailLibraryPayload(inst.MailTemplates, inst.ActiveMailTemplateID))
}

func (s *Server) handleCreateAdminMailTemplate(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeMailTemplate(w, r)
	if !ok {
		return
	}
	actor := adminFromContext(r.Context())
	item, err := s.db.CreateInstanceMailTemplate(r.Context(), actor.ID, body.Name, body.set())
	if err != nil {
		writeStorageError(w, err, "could not save the mail template")
		return
	}
	row := templatePayload(item.Set)
	row["id"] = item.ID.String()
	row["name"] = item.Name
	writeJSON(w, http.StatusCreated, row)
}

func (s *Server) handleUpdateAdminMailTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := mailTemplateID(w, r)
	if !ok {
		return
	}
	body, ok := decodeMailTemplate(w, r)
	if !ok {
		return
	}
	actor := adminFromContext(r.Context())
	item, err := s.db.UpdateInstanceMailTemplate(r.Context(), actor.ID, id, body.Name, body.set())
	if err != nil {
		writeStorageError(w, err, "could not save the mail template")
		return
	}
	row := templatePayload(item.Set)
	row["id"] = item.ID.String()
	row["name"] = item.Name
	writeJSON(w, http.StatusOK, row)
}

func (s *Server) handleDeleteAdminMailTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := mailTemplateID(w, r)
	if !ok {
		return
	}
	actor := adminFromContext(r.Context())
	if err := s.db.DeleteInstanceMailTemplate(r.Context(), actor.ID, id); err != nil {
		writeStorageError(w, err, "could not delete the mail template")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUseAdminMailTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := mailTemplateID(w, r)
	if !ok {
		return
	}
	actor := adminFromContext(r.Context())
	inst, err := s.db.UseInstanceMailTemplate(r.Context(), actor.ID, id)
	if err != nil {
		writeStorageError(w, err, "could not use the mail template")
		return
	}
	writeJSON(w, http.StatusOK, mailLibraryPayload(inst.MailTemplates, inst.ActiveMailTemplateID))
}
