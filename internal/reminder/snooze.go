package reminder

import (
	"fmt"
	"strings"
	"time"
)

func SnoozeUntil(preset string, now time.Time, tz string) (time.Time, error) {
	preset = strings.TrimSpace(strings.ToLower(preset))
	loc := time.UTC
	if tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	local := now.In(loc)
	switch preset {
	case "+15m", "15m":
		return now.Add(15 * time.Minute), nil
	case "+1h", "1h":
		return now.Add(time.Hour), nil
	case "+3h", "3h":
		return now.Add(3 * time.Hour), nil
	case "tomorrow_9am", "tomorrow9am":
		t := time.Date(local.Year(), local.Month(), local.Day()+1, 9, 0, 0, 0, loc)
		return t.UTC(), nil
	default:
		return time.Time{}, fmt.Errorf("unknown snooze preset")
	}
}

func SnoozePresets() []string {
	return []string{"+15m", "+1h", "+3h", "tomorrow_9am"}
}
