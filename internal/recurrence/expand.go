package recurrence

import (
	"fmt"
	"strings"
	"time"

	"github.com/teambition/rrule-go"
)

// NextOccurrence returns the first occurrence strictly after after according to rule.
// tz is an IANA timezone used when the rule string omits DTSTART timezone context.
func NextOccurrence(rule string, after time.Time, tz string) (time.Time, error) {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return time.Time{}, fmt.Errorf("recurrence rule is required")
	}

	r, err := rrule.StrToRRule(rule)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse recurrence rule: %w", err)
	}

	loc := time.UTC
	if tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}

	ref := after.UTC()
	if loc != time.UTC {
		ref = after.In(loc)
	}

	next := r.After(ref, false)
	if next.IsZero() {
		return time.Time{}, fmt.Errorf("no occurrence after %s", after.UTC().Format(time.RFC3339))
	}
	return next.UTC(), nil
}
