package web

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/export"
	importdata "github.com/shivam-20/day-remind/internal/import"
	"github.com/shivam-20/day-remind/internal/reminder"
	"github.com/shivam-20/day-remind/internal/storage"
)

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	bundle, err := export.ExportUserData(r.Context(), s.db, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not export data")
		return
	}
	writeJSON(w, http.StatusOK, bundle)
}

func (s *Server) handleExportICal(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	records, err := s.db.ListUserReminderRecords(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not export calendar")
		return
	}
	body := export.ExportICal(records)
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename="+export.ExportICalFilename())
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

func (s *Server) handleImportJSONPreview(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	raw, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read body")
		return
	}
	result, err := importdata.PreviewImport(r.Context(), s.db, userID, raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleImportJSON(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	mode := r.URL.Query().Get("mode")
	raw, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read body")
		return
	}
	result, err := importdata.ImportUserData(r.Context(), s.db, userID, raw, mode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleImportCSV(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load user")
		return
	}
	rows, err := importdata.ParseCSV(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	records := importdata.CSVToReminderRecords(rows, user.Timezone)
	imported, warnings := s.importReminderRecords(r.Context(), userID, records)
	writeJSON(w, http.StatusOK, map[string]any{
		"reminders_imported": imported,
		"warnings":           warnings,
	})
}

func (s *Server) handleImportICal(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	records, err := importdata.ParseICal(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	imported, warnings := s.importReminderRecords(r.Context(), userID, records)
	writeJSON(w, http.StatusOK, map[string]any{
		"reminders_imported": imported,
		"warnings":           warnings,
	})
}

func (s *Server) importReminderRecords(ctx context.Context, userID uuid.UUID, records []storage.ReminderRecord) (int, []string) {
	var warnings []string
	imported := 0
	for _, rec := range records {
		if strings.TrimSpace(rec.Title) == "" {
			warnings = append(warnings, "skipped reminder with empty title")
			continue
		}
		if rec.Status == "" {
			rec.Status = reminder.StatusScheduled
		}
		if rec.Priority == "" {
			rec.Priority = reminder.PriorityNormal
		}
		if rec.Timezone == "" {
			rec.Timezone = "UTC"
		}
		if rec.ID == uuid.Nil {
			rec.ID = uuid.New()
		}
		if err := s.db.UpsertReminderRecord(ctx, userID, rec); err != nil {
			warnings = append(warnings, err.Error())
			continue
		}
		imported++
	}
	return imported, warnings
}
