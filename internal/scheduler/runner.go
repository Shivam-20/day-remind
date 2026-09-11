package scheduler

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/shivam-20/day-remind/internal/config"
	"github.com/shivam-20/day-remind/internal/notify"
	"github.com/shivam-20/day-remind/internal/reminder"
	"github.com/shivam-20/day-remind/internal/storage"
)

type Scheduler struct {
	db       *storage.DB
	cfg      config.Config
	interval time.Duration
}

func New(db *storage.DB, cfg config.Config) *Scheduler {
	return &Scheduler{db: db, cfg: cfg, interval: 30 * time.Second}
}

func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	log.Printf("scheduler: started (every %s)", s.interval)
	for {
		select {
		case <-ctx.Done():
			log.Printf("scheduler: stopped")
			return
		case t := <-ticker.C:
			s.tick(ctx, t.UTC())
		}
	}
}

func (s *Scheduler) tick(ctx context.Context, now time.Time) {
	deliveryCfg, err := s.loadDelivery(ctx)
	if err != nil {
		log.Printf("scheduler: delivery config: %v", err)
		return
	}
	s.processRetries(ctx, deliveryCfg)
	items, err := s.db.ListDueReminders(ctx, now, 100)
	if err != nil {
		log.Printf("scheduler: list due: %v", err)
		return
	}
	for _, item := range items {
		s.dispatchReminder(ctx, item, deliveryCfg, now)
	}
}

func (s *Scheduler) loadDelivery(ctx context.Context) (notify.DeliveryConfig, error) {
	stored, err := s.db.GetDeliverySettings(ctx)
	if err != nil {
		return notify.DeliveryConfig{}, err
	}
	return notify.EffectiveDelivery(s.cfg, stored), nil
}

func (s *Scheduler) processRetries(ctx context.Context, deliveryCfg notify.DeliveryConfig) {
	items, err := s.db.ListRetryableDeliveries(ctx, 50)
	if err != nil {
		log.Printf("scheduler: list retries: %v", err)
		return
	}
	for _, d := range items {
		s.retryDelivery(ctx, d, deliveryCfg)
	}
}

func (s *Scheduler) dispatchReminder(ctx context.Context, item reminder.Reminder, deliveryCfg notify.DeliveryConfig, now time.Time) {
	state, err := s.db.GetUserNotifyState(ctx, item.UserID)
	if err != nil {
		return
	}
	if blockedByVacation(state, now) {
		return
	}
	if inQuietHours(state, item, now) {
		return
	}
	avail := notify.ChannelAvailability(deliveryCfg)
	for _, pref := range state.Prefs {
		if !pref.Enabled || !avail[pref.Channel] || strings.TrimSpace(pref.Address) == "" {
			continue
		}
		key := storage.DeliveryIdempotencyKey(item.ID, pref.Channel, item.DueAt)
		d, err := s.db.CreateDelivery(ctx, storage.Delivery{
			ReminderID:     item.ID,
			UserID:         item.UserID,
			Channel:        pref.Channel,
			Status:         storage.DeliveryStatusPending,
			IdempotencyKey: key,
		})
		if err == storage.ErrConflict {
			continue
		}
		if err != nil {
			log.Printf("scheduler: create delivery: %v", err)
			continue
		}
		s.sendDelivery(ctx, d, item, pref, deliveryCfg)
	}
}

func (s *Scheduler) retryDelivery(ctx context.Context, d storage.Delivery, deliveryCfg notify.DeliveryConfig) {
	item, err := s.db.GetReminder(ctx, d.UserID, d.ReminderID)
	if err != nil {
		return
	}
	state, err := s.db.GetUserNotifyState(ctx, d.UserID)
	if err != nil {
		return
	}
	var pref storage.NotificationPref
	for _, p := range state.Prefs {
		if p.Channel == d.Channel {
			pref = p
			break
		}
	}
	s.sendDelivery(ctx, d, item, pref, deliveryCfg)
}

func (s *Scheduler) sendDelivery(ctx context.Context, d storage.Delivery, item reminder.Reminder, pref storage.NotificationPref, deliveryCfg notify.DeliveryConfig) {
	subject := fmt.Sprintf("Reminder: %s", item.Title)
	body := fmt.Sprintf("<p><strong>%s</strong></p><p>%s</p><p>Due: %s</p>",
		escapeHTML(item.Title), escapeHTML(item.Description), item.DueAt.UTC().Format(time.RFC3339))
	text := fmt.Sprintf("Reminder: %s\n%s\nDue: %s", item.Title, item.Description, item.DueAt.UTC().Format(time.RFC822))

	var err error
	switch d.Channel {
	case storage.ChannelEmail:
		err = notify.SendEmail(ctx, deliveryCfg, pref.Address, subject, body)
	case storage.ChannelTelegram:
		err = notify.SendTelegram(ctx, deliveryCfg, pref.Address, text)
	default:
		err = fmt.Errorf("unknown channel")
	}
	if err != nil {
		_ = s.db.MarkDeliveryFailed(ctx, d.ID, err.Error())
		return
	}
	_ = s.db.MarkDeliverySent(ctx, d.ID)
}

func blockedByVacation(state storage.UserNotifyState, now time.Time) bool {
	if !state.NotificationsPaused {
		return false
	}
	if state.NotificationsPausedUntil != nil && now.After(state.NotificationsPausedUntil.UTC()) {
		return false
	}
	return true
}

func inQuietHours(state storage.UserNotifyState, item reminder.Reminder, now time.Time) bool {
	if item.Priority == reminder.PriorityUrgent {
		return false
	}
	start := strings.TrimSpace(state.QuietStart)
	end := strings.TrimSpace(state.QuietEnd)
	if start == "" || end == "" {
		return false
	}
	loc, err := time.LoadLocation(item.Timezone)
	if err != nil || loc == nil {
		loc = time.UTC
	}
	localNow := now.In(loc)
	return clockInRange(localNow, start, end)
}

func clockInRange(t time.Time, start, end string) bool {
	sh, sm := parseClock(start)
	eh, em := parseClock(end)
	if sh < 0 {
		return false
	}
	cur := t.Hour()*60 + t.Minute()
	s := sh*60 + sm
	e := eh*60 + em
	if s <= e {
		return cur >= s && cur < e
	}
	return cur >= s || cur < e
}

func parseClock(raw string) (hour, min int) {
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		return -1, -1
	}
	fmt.Sscanf(parts[0], "%d", &hour)
	fmt.Sscanf(parts[1], "%d", &min)
	if hour < 0 || hour > 23 || min < 0 || min > 59 {
		return -1, -1
	}
	return hour, min
}

func escapeHTML(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}
