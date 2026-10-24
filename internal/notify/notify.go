// Package notify renders reminder notifications and delivers them over the
// configured channels. It knows nothing about storage or HTTP.
package notify

import (
	"context"
	"fmt"
	"time"
)

// Channel names as recorded in the delivery log.
const (
	ChannelEmail    = "email"
	ChannelTelegram = "telegram"
)

// Message is a rendered notification, independent of channel.
type Message struct {
	ReminderID  string
	Title       string
	Description string
	DueAt       time.Time
	Timezone    string
	Priority    string
	// Advance is true for the heads-up sent before the due time.
	Advance     bool
	LeadTime    time.Duration
	DisplayName string
}

// Sender delivers a rendered message to one address on one channel.
//
// Both the Message and the Rendered output are passed: the rendering is done
// once by the caller (which knows the recipient's merged template set), while
// the sender still needs the raw Message for channel details such as the
// reminder id behind Telegram's inline buttons.
type Sender interface {
	// Name is the channel name used in the delivery log.
	Name() string
	// Send delivers to dest: an email address, or a Telegram chat ID.
	Send(ctx context.Context, dest string, m Message, r Rendered) error
}

// localDue renders the due time in the reminder's own timezone, falling back to
// UTC when the stored zone is not installed on this machine.
func (m Message) localDue() string {
	loc, err := time.LoadLocation(m.Timezone)
	if err != nil || m.Timezone == "" {
		return m.DueAt.UTC().Format("Mon 2 Jan 2006, 15:04") + " UTC"
	}
	return m.DueAt.In(loc).Format("Mon 2 Jan 2006, 15:04") + " " + m.Timezone
}

// humanDuration renders a lead time the way a person would say it.
func humanDuration(d time.Duration) string {
	switch {
	case d >= 24*time.Hour:
		days := int(d.Hours() / 24)
		return plural(days, "day")
	case d >= time.Hour:
		return plural(int(d.Hours()), "hour")
	default:
		mins := int(d.Minutes())
		if mins < 1 {
			mins = 1
		}
		return plural(mins, "minute")
	}
}

func plural(n int, unit string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", unit)
	}
	return fmt.Sprintf("%d %ss", n, unit)
}
