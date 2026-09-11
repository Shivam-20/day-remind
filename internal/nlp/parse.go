package nlp

import (
	"regexp"
	"strings"
	"time"
)

// ParsedReminder is the structured output of the rule-based NL parser.
type ParsedReminder struct {
	Title          string
	DueAt          time.Time
	RecurrenceRule string
	Priority       string
	Tags           []string
}

var (
	reRemindMeTo   = regexp.MustCompile(`(?i)^(?:remind\s+me\s+to|remind\s+me)\s+(.+)$`)
	reEveryMonth   = regexp.MustCompile(`(?i)\bevery\s+month\b`)
	reEveryWeek    = regexp.MustCompile(`(?i)\bevery\s+week\b`)
	reEveryDay     = regexp.MustCompile(`(?i)\bevery\s+day\b`)
	reOnNth        = regexp.MustCompile(`(?i)\bon\s+(?:the\s+)?(\d{1,2})(?:st|nd|rd|th)?\b`)
	reAtTime       = regexp.MustCompile(`(?i)\bat\s+(\d{1,2})(?::(\d{2}))?\s*(am|pm)?\b`)
	reTomorrow     = regexp.MustCompile(`(?i)\btomorrow\b`)
	reToday        = regexp.MustCompile(`(?i)\btoday\b`)
	reNextWeek     = regexp.MustCompile(`(?i)\bnext\s+week\b`)
	rePriorityHigh = regexp.MustCompile(`(?i)\b(urgent|asap|important|high\s+priority)\b`)
	rePriorityLow  = regexp.MustCompile(`(?i)\b(low\s+priority|whenever)\b`)
	reTag          = regexp.MustCompile(`#([a-z0-9_-]+)`)
)

// Parse converts natural-language reminder text into structured fields.
func Parse(text string, now time.Time, loc *time.Location) ParsedReminder {
	text = strings.TrimSpace(text)
	if text == "" {
		return ParsedReminder{}
	}
	if loc == nil {
		loc = time.UTC
	}
	now = now.In(loc)

	title := text
	if m := reRemindMeTo.FindStringSubmatch(text); len(m) == 2 {
		title = strings.TrimSpace(m[1])
	}

	var due time.Time
	var recurrence string

	if reEveryMonth.MatchString(title) {
		recurrence = "FREQ=MONTHLY"
	}
	if reEveryWeek.MatchString(title) {
		recurrence = "FREQ=WEEKLY"
	}
	if reEveryDay.MatchString(title) {
		recurrence = "FREQ=DAILY"
	}

	hour, min, hasTime := extractTime(title)
	base := now
	if reTomorrow.MatchString(title) {
		base = time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, loc)
	} else if reToday.MatchString(title) {
		base = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	} else if reNextWeek.MatchString(title) {
		base = now.AddDate(0, 0, 7)
	} else if m := reOnNth.FindStringSubmatch(title); len(m) == 2 {
		day := atoi(m[1])
		if day >= 1 && day <= 31 {
			base = time.Date(now.Year(), now.Month(), day, 0, 0, 0, 0, loc)
			if base.Before(now) {
				base = base.AddDate(0, 1, 0)
			}
		}
	}

	if hasTime {
		due = time.Date(base.Year(), base.Month(), base.Day(), hour, min, 0, 0, loc)
	} else if !base.Equal(now) || reTomorrow.MatchString(title) || reToday.MatchString(title) {
		due = time.Date(base.Year(), base.Month(), base.Day(), 9, 0, 0, 0, loc)
	}

	title = cleanTitle(title)

	priority := "normal"
	if rePriorityHigh.MatchString(text) {
		priority = "high"
	} else if rePriorityLow.MatchString(text) {
		priority = "low"
	}

	var tags []string
	for _, m := range reTag.FindAllStringSubmatch(text, -1) {
		if len(m) == 2 {
			tags = append(tags, m[1])
		}
	}

	return ParsedReminder{
		Title:          title,
		DueAt:          due,
		RecurrenceRule: recurrence,
		Priority:       priority,
		Tags:           tags,
	}
}

func extractTime(s string) (hour, min int, ok bool) {
	m := reAtTime.FindStringSubmatch(s)
	if len(m) < 4 {
		return 0, 0, false
	}
	hour = atoi(m[1])
	min = 0
	if m[2] != "" {
		min = atoi(m[2])
	}
	ampm := strings.ToLower(m[3])
	switch ampm {
	case "pm":
		if hour < 12 {
			hour += 12
		}
	case "am":
		if hour == 12 {
			hour = 0
		}
	default:
		if hour >= 1 && hour <= 7 {
			hour += 12
		}
	}
	if hour < 0 || hour > 23 || min < 0 || min > 59 {
		return 0, 0, false
	}
	return hour, min, true
}

func cleanTitle(s string) string {
	patterns := []*regexp.Regexp{
		reTomorrow, reToday, reNextWeek, reEveryMonth, reEveryWeek, reEveryDay,
		reOnNth, reAtTime, rePriorityHigh, rePriorityLow, reTag,
	}
	out := s
	for _, p := range patterns {
		out = p.ReplaceAllString(out, "")
	}
	out = strings.TrimSpace(strings.Join(strings.Fields(out), " "))
	out = strings.Trim(out, ",.-")
	return out
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}
