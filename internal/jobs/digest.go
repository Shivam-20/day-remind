package jobs

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/shivam-20/day-remind/internal/config"
	"github.com/shivam-20/day-remind/internal/notify"
	"github.com/shivam-20/day-remind/internal/storage"
)

// RunMorningDigests sends daily summary emails/messages to users whose local
// morning digest time matches the current hour.
func RunMorningDigests(ctx context.Context, db *storage.DB, cfg config.Config) error {
	stored, err := db.GetDeliverySettings(ctx)
	if err != nil {
		return err
	}
	deliveryCfg := notify.EffectiveDelivery(cfg, stored)
	avail := notify.ChannelAvailability(deliveryCfg)

	users, err := db.ListDigestUsers(ctx)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	for _, u := range users {
		if err := sendUserDigest(ctx, db, deliveryCfg, avail, u, now); err != nil {
			log.Printf("digest: user %s: %v", u.ID, err)
		}
	}
	return nil
}

func sendUserDigest(ctx context.Context, db *storage.DB, deliveryCfg notify.DeliveryConfig, avail map[string]bool, u storage.DigestUser, now time.Time) error {
	loc, err := time.LoadLocation(u.Timezone)
	if err != nil || loc == nil {
		loc = time.UTC
	}
	localNow := now.In(loc)
	digestTime := strings.TrimSpace(u.MorningDigestTime)
	if digestTime == "" {
		digestTime = "08:00"
	}
	targetHour, targetMin := parseDigestClock(digestTime)
	if localNow.Hour() != targetHour {
		return nil
	}
	if localNow.Minute() < targetMin || localNow.Minute() >= targetMin+15 {
		return nil
	}

	dayStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)
	dayEnd := dayStart.Add(24 * time.Hour)
	docs, err := db.ListRemindersForDay(ctx, u.ID, dayStart, dayEnd)
	if err != nil {
		return err
	}
	if len(docs) == 0 {
		return nil
	}

	subject := fmt.Sprintf("Good morning — %d reminder(s) for today", len(docs))
	var lines []string
	lines = append(lines, fmt.Sprintf("You have %d reminder(s) today:", len(docs)))
	for _, doc := range docs {
		localDue := doc.DueAt.In(loc)
		lines = append(lines, fmt.Sprintf("• %s at %s", doc.Title, localDue.Format("15:04")))
	}
	text := strings.Join(lines, "\n")
	body := "<ul>"
	for _, doc := range docs {
		localDue := doc.DueAt.In(loc)
		body += fmt.Sprintf("<li><strong>%s</strong> at %s</li>", doc.Title, localDue.Format("15:04"))
	}
	body += "</ul>"

	for _, pref := range u.NotificationPrefs {
		if !pref.Enabled || !avail[pref.Channel] || strings.TrimSpace(pref.Address) == "" {
			continue
		}
		switch pref.Channel {
		case storage.ChannelEmail:
			if err := notify.SendEmail(ctx, deliveryCfg, pref.Address, subject, body); err != nil {
				return err
			}
		case storage.ChannelTelegram:
			if err := notify.SendTelegram(ctx, deliveryCfg, pref.Address, text); err != nil {
				return err
			}
		}
	}
	return nil
}

func parseDigestClock(raw string) (hour, min int) {
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		return 8, 0
	}
	fmt.Sscanf(parts[0], "%d", &hour)
	fmt.Sscanf(parts[1], "%d", &min)
	if hour < 0 || hour > 23 {
		hour = 8
	}
	if min < 0 || min > 59 {
		min = 0
	}
	return hour, min
}
