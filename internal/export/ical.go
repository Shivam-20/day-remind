package export

import (
	"fmt"
	"strings"
	"time"

	"github.com/shivam-20/day-remind/internal/storage"
)

// ExportICal renders reminders as an iCalendar VCALENDAR document.
func ExportICal(reminders []storage.ReminderRecord) string {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\n")
	b.WriteString("VERSION:2.0\r\n")
	b.WriteString("PRODID:-//Day Remind//EN\r\n")
	b.WriteString("CALSCALE:GREGORIAN\r\n")
	for _, r := range reminders {
		writeVEVENT(&b, r)
	}
	b.WriteString("END:VCALENDAR\r\n")
	return b.String()
}

func writeVEVENT(b *strings.Builder, r storage.ReminderRecord) {
	uid := r.ID.String() + "@dayremind.app"
	dtStart := r.DueAt.UTC().Format("20060102T150405Z")
	dtStamp := time.Now().UTC().Format("20060102T150405Z")
	b.WriteString("BEGIN:VEVENT\r\n")
	b.WriteString("UID:" + uid + "\r\n")
	b.WriteString("DTSTAMP:" + dtStamp + "\r\n")
	b.WriteString("DTSTART:" + dtStart + "\r\n")
	b.WriteString("SUMMARY:" + icalEscape(r.Title) + "\r\n")
	if r.Description != "" {
		b.WriteString("DESCRIPTION:" + icalEscape(r.Description) + "\r\n")
	}
	if r.RecurrenceRule != "" {
		b.WriteString("RRULE:" + icalEscape(r.RecurrenceRule) + "\r\n")
	}
	switch r.Status {
	case "done":
		b.WriteString("STATUS:COMPLETED\r\n")
	case "cancelled":
		b.WriteString("STATUS:CANCELLED\r\n")
	default:
		b.WriteString("STATUS:CONFIRMED\r\n")
	}
	b.WriteString("END:VEVENT\r\n")
}

func icalEscape(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, ";", "\\;")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

// ExportICalFilename returns a suggested download filename.
func ExportICalFilename() string {
	return fmt.Sprintf("day-remind-%s.ics", time.Now().UTC().Format("20060102"))
}
