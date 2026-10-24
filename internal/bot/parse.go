package bot

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ParseDuration reads the shorthand people type in chat: 30m, 2h, 1d, 3w.
// It deliberately does not accept Go's raw duration syntax for days, which
// does not exist there.
func ParseDuration(s string) (time.Duration, error) {
	s = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(s, " ", "")))
	m := regexp.MustCompile(`^(\d+)(m|min|mins|minute|minutes|h|hr|hrs|hour|hours|d|day|days|w|week|weeks)$`).FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("unrecognised duration %q", s)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("duration must be a positive number")
	}
	var unit time.Duration
	switch m[2][:1] {
	case "m":
		unit = time.Minute
	case "h":
		unit = time.Hour
	case "d":
		unit = 24 * time.Hour
	case "w":
		unit = 7 * 24 * time.Hour
	}
	d := time.Duration(n) * unit
	// A year is far beyond anything a chat shorthand should express, and keeps
	// the value well inside what Mongo can store.
	if d > 365*24*time.Hour {
		return 0, fmt.Errorf("duration is too far out")
	}
	return d, nil
}

var (
	reIn        = regexp.MustCompile(`^in\s+(.+)$`)
	reClock     = regexp.MustCompile(`^(\d{1,2})(?::(\d{2}))?\s*(am|pm)?$`)
	reISODate   = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`)
	reDayMonth  = regexp.MustCompile(`^(\d{1,2})\s+([a-z]{3,9})$`)
	reWeekdayRe = regexp.MustCompile(`^(mon|tue|wed|thu|fri|sat|sun)[a-z]*$`)
)

// ParseWhen turns the phrasings people actually type into an absolute UTC time.
// now must already be expressed in loc, so "tomorrow" means the user's tomorrow
// rather than the server's.
//
// Understood forms:
//
//	in 2h, in 30m, in 3 days
//	tomorrow, tomorrow 9am, today 18:00, tonight
//	monday, next friday 14:30
//	2026-12-25, 2026-12-25 08:00
//	25 dec, 25 december 9am
//	9am, 18:30                      (today if still ahead, else tomorrow)
func ParseWhen(raw string, now time.Time, loc *time.Location) (time.Time, error) {
	s := strings.ToLower(strings.Join(strings.Fields(raw), " "))
	if s == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}

	// "in <duration>", including "in 3 days" with a space.
	if m := reIn.FindStringSubmatch(s); m != nil {
		d, err := ParseDuration(strings.ReplaceAll(m[1], " ", ""))
		if err != nil {
			return time.Time{}, err
		}
		return now.Add(d).UTC(), nil
	}

	// Split a trailing clock time off the date words.
	datePart, timePart := splitDateTime(s)
	hour, minute, hasTime := 0, 0, false
	if timePart != "" {
		h, mi, err := parseClock(timePart)
		if err != nil {
			return time.Time{}, err
		}
		hour, minute, hasTime = h, mi, true
	}

	// Default to 9am when only a date was given, which is what people expect
	// from "tomorrow" or "25 dec".
	if !hasTime {
		hour, minute = 9, 0
	}

	base := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, loc)

	switch {
	case datePart == "":
		// Bare clock time: today if it is still ahead, otherwise tomorrow.
		if !hasTime {
			return time.Time{}, fmt.Errorf("no date or time found")
		}
		if !base.After(now) {
			base = base.AddDate(0, 0, 1)
		}
		return base.UTC(), nil

	case datePart == "today":
		return base.UTC(), nil

	case datePart == "tonight":
		if !hasTime {
			base = time.Date(now.Year(), now.Month(), now.Day(), 20, 0, 0, 0, loc)
		}
		return base.UTC(), nil

	case datePart == "tomorrow":
		return base.AddDate(0, 0, 1).UTC(), nil

	case datePart == "day after tomorrow":
		return base.AddDate(0, 0, 2).UTC(), nil
	}

	// "next <weekday>" nudges past the coming one.
	next := false
	if rest, ok := strings.CutPrefix(datePart, "next "); ok {
		next = true
		datePart = rest
	}

	if m := reWeekdayRe.FindStringSubmatch(datePart); m != nil {
		target, ok := weekdays[m[1]]
		if !ok {
			return time.Time{}, fmt.Errorf("unknown weekday")
		}
		delta := (int(target) - int(base.Weekday()) + 7) % 7
		if delta == 0 && (!base.After(now) || next) {
			delta = 7
		} else if next && delta < 7 {
			delta += 7
		}
		return base.AddDate(0, 0, delta).UTC(), nil
	}

	if m := reISODate.FindStringSubmatch(datePart); m != nil {
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		d, _ := strconv.Atoi(m[3])
		if mo < 1 || mo > 12 || d < 1 || d > 31 {
			return time.Time{}, fmt.Errorf("invalid date")
		}
		return time.Date(y, time.Month(mo), d, hour, minute, 0, 0, loc).UTC(), nil
	}

	if m := reDayMonth.FindStringSubmatch(datePart); m != nil {
		d, _ := strconv.Atoi(m[1])
		mo, ok := months[m[2][:3]]
		if !ok || d < 1 || d > 31 {
			return time.Time{}, fmt.Errorf("unknown month")
		}
		t := time.Date(now.Year(), mo, d, hour, minute, 0, 0, loc)
		// A date already past this year means they mean next year.
		if t.Before(now) {
			t = t.AddDate(1, 0, 0)
		}
		return t.UTC(), nil
	}

	return time.Time{}, fmt.Errorf("unrecognised time %q", raw)
}

// splitDateTime peels a trailing clock time off the end of the phrase.
func splitDateTime(s string) (datePart, timePart string) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return "", ""
	}
	// Try the last one or two words as a clock ("9 pm" as well as "9pm").
	for _, take := range []int{1, 2} {
		if len(fields) < take {
			continue
		}
		candidate := strings.Join(fields[len(fields)-take:], "")
		if reClock.MatchString(candidate) {
			return strings.Join(fields[:len(fields)-take], " "), candidate
		}
	}
	return s, ""
}

func parseClock(s string) (hour, minute int, err error) {
	m := reClock.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, fmt.Errorf("unrecognised time %q", s)
	}
	hour, _ = strconv.Atoi(m[1])
	if m[2] != "" {
		minute, _ = strconv.Atoi(m[2])
	}
	switch m[3] {
	case "am":
		if hour == 12 {
			hour = 0
		}
	case "pm":
		if hour != 12 {
			hour += 12
		}
	}
	if hour > 23 || minute > 59 {
		return 0, 0, fmt.Errorf("invalid clock time %q", s)
	}
	return hour, minute, nil
}

var weekdays = map[string]time.Weekday{
	"mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday, "sun": time.Sunday,
}

var months = map[string]time.Month{
	"jan": time.January, "feb": time.February, "mar": time.March, "apr": time.April,
	"may": time.May, "jun": time.June, "jul": time.July, "aug": time.August,
	"sep": time.September, "oct": time.October, "nov": time.November, "dec": time.December,
}
