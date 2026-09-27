package web

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/notify"
	"github.com/shivam-20/day-remind/internal/storage"
)

type mailWordingRequest struct {
	Name         string `json:"name"`
	EmailSubject string `json:"email_subject"`
	EmailHTML    string `json:"email_html"`
	EmailCSS     string `json:"email_css"`
	EmailText    string `json:"email_text"`
	TelegramText string `json:"telegram_text"`
}

func mailWordingPayload(item storage.MailWording, active *uuid.UUID) map[string]any {
	return map[string]any{
		"id":            item.ID.String(),
		"name":          item.Name,
		"email_subject": item.EmailSubject,
		"email_html":    item.EmailHTML,
		"email_css":     item.EmailCSS,
		"email_text":    item.EmailText,
		"telegram_text": item.TelegramText,
		"active":        active != nil && *active == item.ID,
	}
}

func (s *Server) handleListMailWordings(w http.ResponseWriter, r *http.Request) {
	items, active, err := s.db.ListMailWordings(r.Context(), userIDFromContext(r.Context()))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load mail templates")
		return
	}
	out := make([]map[string]any, 0, len(items))
	activeID := ""
	if active != nil {
		activeID = active.String()
	}
	for _, item := range items {
		out = append(out, mailWordingPayload(item, active))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"templates": out,
		"active_id": activeID,
		"presets":   wordingPresetPayloads(),
		"variables": []map[string]string{
			{"Name": ".Title", "Description": "Reminder title"},
			{"Name": ".Description", "Description": "Reminder description (may be empty)"},
			{"Name": ".DueAt", "Description": "Due date and time, in the reminder's timezone"},
			{"Name": ".Timezone", "Description": "Reminder timezone"},
			{"Name": ".Priority", "Description": "low, normal, high or urgent"},
			{"Name": ".Advance", "Description": "True for an early warning"},
			{"Name": ".LeadTime", "Description": "How far ahead the warning is, e.g. \"1 hour\""},
			{"Name": ".UserName", "Description": "Recipient display name (may be empty)"},
			{"Name": ".ListName", "Description": "List name (may be empty)"},
			{"Name": ".Tags", "Description": "Comma-separated tag names (may be empty)"},
			{"Name": ".ReminderURL", "Description": "Link to open the reminder in the app (may be empty)"},
			{"Name": ".Today", "Description": "Today's date in the reminder's timezone"},
		},
	})
}

// wordingPresetPayloads exposes the built-in starter designs to the editor.
func wordingPresetPayloads() []map[string]any {
	presets := notify.WordingPresets()
	out := make([]map[string]any, 0, len(presets))
	for _, p := range presets {
		out = append(out, map[string]any{
			"name":          p.Name,
			"email_subject": p.EmailSubject,
			"email_html":    p.EmailHTML,
			"email_css":     p.EmailCSS,
			"email_text":    p.EmailText,
			"telegram_text": p.TelegramText,
		})
	}
	return out
}

func decodeMailWording(w http.ResponseWriter, r *http.Request) (mailWordingRequest, bool) {
	var body mailWordingRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return mailWordingRequest{}, false
	}
	return body, true
}

func (s *Server) handleCreateMailWording(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeMailWording(w, r)
	if !ok {
		return
	}
	if err := notify.ValidateWording(notify.Wording{
		Name: body.Name, EmailSubject: body.EmailSubject, EmailHTML: body.EmailHTML,
		EmailCSS: body.EmailCSS, EmailText: body.EmailText, TelegramText: body.TelegramText,
	}); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	item, err := s.db.CreateMailWording(r.Context(), userIDFromContext(r.Context()), body.Name, body.EmailSubject, body.EmailHTML, body.EmailCSS, body.EmailText, body.TelegramText)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, mailWordingPayload(item, nil))
}

func mailWordingID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return uuid.UUID{}, false
	}
	return id, true
}

func (s *Server) handleUpdateMailWording(w http.ResponseWriter, r *http.Request) {
	id, ok := mailWordingID(w, r)
	if !ok {
		return
	}
	body, ok := decodeMailWording(w, r)
	if !ok {
		return
	}
	if err := notify.ValidateWording(notify.Wording{
		Name: body.Name, EmailSubject: body.EmailSubject, EmailHTML: body.EmailHTML,
		EmailCSS: body.EmailCSS, EmailText: body.EmailText, TelegramText: body.TelegramText,
	}); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	item, err := s.db.UpdateMailWording(r.Context(), userIDFromContext(r.Context()), id, body.Name, body.EmailSubject, body.EmailHTML, body.EmailCSS, body.EmailText, body.TelegramText)
	if err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, mailWordingPayload(item, nil))
}

func (s *Server) handleDeleteMailWording(w http.ResponseWriter, r *http.Request) {
	id, ok := mailWordingID(w, r)
	if !ok {
		return
	}
	err := s.db.DeleteMailWording(r.Context(), userIDFromContext(r.Context()), id)
	if err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete the mail template")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUseMailWording(w http.ResponseWriter, r *http.Request) {
	id, ok := mailWordingID(w, r)
	if !ok {
		return
	}
	userID := userIDFromContext(r.Context())
	if err := s.db.UseMailWording(r.Context(), userID, id); err == storage.ErrNotFound {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not use the mail template")
		return
	}
	s.handleListMailWordings(w, r)
}

type generateWordingRequest struct {
	Brief string `json:"brief"`
}

// handleGenerateMailWording asks Gemini for a complete template from a short
// brief. The draft is returned, not saved — the client loads it into the
// editor so the user reviews it before saving.
func (s *Server) handleGenerateMailWording(w http.ResponseWriter, r *http.Request) {
	var body generateWordingRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	aiCfg, err := s.effectiveAI(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load ai settings")
		return
	}
	wording, err := notify.GenerateWordingWithAI(r.Context(), aiCfg, body.Brief)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":          wording.Name,
		"email_subject": wording.EmailSubject,
		"email_html":    wording.EmailHTML,
		"email_css":     wording.EmailCSS,
		"email_text":    wording.EmailText,
		"telegram_text": wording.TelegramText,
	})
}

func (s *Server) handlePreviewMailWording(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeMailWording(w, r)
	if !ok {
		return
	}
	wording := notify.Wording{
		EmailSubject: body.EmailSubject,
		EmailHTML:    body.EmailHTML,
		EmailCSS:     body.EmailCSS,
		EmailText:    body.EmailText,
		TelegramText: body.TelegramText,
	}
	if err := notify.ValidateWording(wording); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	out := map[string]any{}
	for _, advance := range []bool{false, true} {
		rendered := notify.RenderWording(wording, notify.SampleWordingData(advance))
		key := "due"
		if advance {
			key = "advance"
		}
		out[key] = map[string]any{
			"subject":  rendered.Subject,
			"html":     rendered.HTML,
			"text":     rendered.Text,
			"telegram": rendered.Telegram,
			"warnings": rendered.Problems,
		}
	}
	writeJSON(w, http.StatusOK, out)
}
