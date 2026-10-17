package web

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/shivam-20/day-remind/internal/auth"
	"github.com/shivam-20/day-remind/internal/reminder"
)

const maxDisplayNameLen = 80

type updateMeRequest struct {
	DisplayName *string `json:"display_name"`
	Timezone    *string `json:"timezone"`
}

func (s *Server) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	var body updateMeRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.DisplayName != nil && len(strings.TrimSpace(*body.DisplayName)) > maxDisplayNameLen {
		writeError(w, http.StatusBadRequest, "display name is too long")
		return
	}
	// An explicit timezone must be real: silently storing garbage would show
	// every future reminder at a made-up time.
	if body.Timezone != nil {
		tz := strings.TrimSpace(*body.Timezone)
		if tz != "" {
			if _, err := time.LoadLocation(tz); err != nil {
				writeError(w, http.StatusBadRequest, "unknown timezone")
				return
			}
		}
	}
	user, err := s.db.SetUserProfile(r.Context(), userID, body.DisplayName, body.Timezone)
	if err != nil {
		writeStorageError(w, err, "could not save profile")
		return
	}
	writeJSON(w, http.StatusOK, userPayload(user))
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	var body changePasswordRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil {
		writeStorageError(w, err, "could not load profile")
		return
	}
	if !auth.CheckPassword(user.PasswordHash, body.CurrentPassword) {
		writeError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}
	hash, err := auth.HashPassword(body.NewPassword)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.db.SetUserPassword(r.Context(), userID, hash); err != nil {
		writeStorageError(w, err, "could not save password")
		return
	}
	// A password change invalidates every other session, so a stolen cookie
	// stops working the moment the owner rotates the password.
	if err := s.db.DeleteOtherSessions(r.Context(), userID, sessionTokenFromRequest(r)); err != nil {
		// The password itself is changed; failing to revoke old sessions must
		// not read as a failed change, but it is worth surfacing.
		writeError(w, http.StatusInternalServerError, "password changed, but could not sign out other sessions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---------------------------------------------------------------- export

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil {
		writeStorageError(w, err, "could not load profile")
		return
	}
	items, err := s.db.ListRemindersForExport(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load reminders")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, reminderPayload(item))
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="day-remind-%s.json"`, time.Now().UTC().Format("20060102")))
	writeJSON(w, http.StatusOK, map[string]any{
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"user": map[string]any{
			"email":        user.Email,
			"display_name": user.DisplayName,
			"timezone":     user.Timezone,
		},
		"reminders": out,
	})
}

func (s *Server) handleExportICal(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	items, err := s.db.ListRemindersForExport(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load reminders")
		return
	}

	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\n")
	b.WriteString("VERSION:2.0\r\n")
	b.WriteString("PRODID:-//Day Remind//Export//EN\r\n")
	b.WriteString("CALSCALE:GREGORIAN\r\n")
	stamp := time.Now().UTC().Format("20060102T150405Z")
	for _, item := range items {
		if item.Status == reminder.StatusCancelled {
			continue
		}
		start := item.DueAt.UTC()
		b.WriteString("BEGIN:VEVENT\r\n")
		fmt.Fprintf(&b, "UID:%s@day-remind\r\n", item.ID)
		fmt.Fprintf(&b, "DTSTAMP:%s\r\n", stamp)
		fmt.Fprintf(&b, "DTSTART:%s\r\n", start.Format("20060102T150405Z"))
		fmt.Fprintf(&b, "DTEND:%s\r\n", start.Add(30*time.Minute).Format("20060102T150405Z"))
		fmt.Fprintf(&b, "SUMMARY:%s\r\n", icalEscape(item.Title))
		if item.Description != "" {
			fmt.Fprintf(&b, "DESCRIPTION:%s\r\n", icalEscape(item.Description))
		}
		if rule, ok := icalRRule(item.Recurrence); ok {
			fmt.Fprintf(&b, "RRULE:%s\r\n", rule)
		}
		if item.Status == reminder.StatusDone {
			b.WriteString("STATUS:COMPLETED\r\n")
			if item.CompletedAt != nil {
				fmt.Fprintf(&b, "COMPLETED:%s\r\n", item.CompletedAt.UTC().Format("20060102T150405Z"))
			}
		}
		b.WriteString("END:VEVENT\r\n")
	}
	b.WriteString("END:VCALENDAR\r\n")

	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="day-remind-%s.ics"`, time.Now().UTC().Format("20060102")))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.String()))
}

func icalEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\n", `\n`)
	return r.Replace(strings.TrimSpace(s))
}

func icalRRule(recurrence string) (string, bool) {
	switch recurrence {
	case reminder.RecurDaily:
		return "FREQ=DAILY", true
	case reminder.RecurWeekly:
		return "FREQ=WEEKLY", true
	case reminder.RecurMonthly:
		return "FREQ=MONTHLY", true
	case reminder.RecurYearly:
		return "FREQ=YEARLY", true
	}
	return "", false
}
