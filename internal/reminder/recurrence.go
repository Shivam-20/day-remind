package reminder

import "time"

const (
	RecurDaily   = "daily"
	RecurWeekly  = "weekly"
	RecurMonthly = "monthly"
	RecurYearly  = "yearly"
)

func ValidRecurrence(s string) bool {
	switch s {
	case "", RecurDaily, RecurWeekly, RecurMonthly, RecurYearly:
		return true
	}
	return false
}

func NextDue(due time.Time, tz, recur string) (time.Time, bool) {
	if recur == "" || !ValidRecurrence(recur) {
		return time.Time{}, false
	}
	loc, err := time.LoadLocation(tz)
	if err != nil || loc == nil {
		loc = time.UTC
	}
	local := due.In(loc)
	var next time.Time
	switch recur {
	case RecurDaily:
		next = local.AddDate(0, 0, 1)
	case RecurWeekly:
		next = local.AddDate(0, 0, 7)
	case RecurMonthly:
		next = local.AddDate(0, 1, 0)
	case RecurYearly:
		next = local.AddDate(1, 0, 0)
	default:
		return time.Time{}, false
	}
	return next.UTC(), true
}

func NextDueAfter(due time.Time, tz, recur string, now time.Time) (time.Time, bool) {
	next, ok := NextDue(due, tz, recur)
	if !ok {
		return time.Time{}, false
	}
	limit := now.UTC()
	for !next.After(limit) {
		n2, ok := NextDue(next, tz, recur)
		if !ok || n2.Equal(next) {
			break
		}
		next = n2
	}
	return next, true
}
