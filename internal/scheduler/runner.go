package scheduler

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/config"
	"github.com/shivam-20/day-remind/internal/jobs"
	"github.com/shivam-20/day-remind/internal/notify"
	"github.com/shivam-20/day-remind/internal/reminder"
	"github.com/shivam-20/day-remind/internal/storage"
)

type Scheduler struct {
	db             *storage.DB
	cfg            config.Config
	interval       time.Duration
	lastDigestHour int
	lastDigestDay  int
}

func New(db *storage.DB, cfg config.Config) *Scheduler {
	return &Scheduler{db: db, cfg: cfg, interval: 30 * time.Second, lastDigestHour: -1}
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
	s.maybeRunDigests(ctx, now)
	occurrences, err := s.db.ListDueOccurrences(ctx, now, 100)
	if err != nil {
		log.Printf("scheduler: list due occurrences: %v", err)
	} else {
		for _, occ := range occurrences {
			s.dispatchOccurrence(ctx, occ, deliveryCfg, now)
		}
	}
	items, err := s.db.ListDueReminders(ctx, now, 100)
	if err != nil {
		log.Printf("scheduler: list due: %v", err)
		return
	}
	for _, item := range items {
		s.dispatchReminder(ctx, item, deliveryCfg, now)
	}
}

func (s *Scheduler) maybeRunDigests(ctx context.Context, now time.Time) {
	hour := now.Hour()
	day := now.YearDay()
	if s.lastDigestHour == hour && s.lastDigestDay == day {
		return
	}
	s.lastDigestHour = hour
	s.lastDigestDay = day
	if err := jobs.RunMorningDigests(ctx, s.db, s.cfg); err != nil {
		log.Printf("scheduler: morning digests: %v", err)
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

func (s *Scheduler) reminderListPaused(ctx context.Context, item reminder.Reminder) bool {
	if item.ListID == nil {
		return false
	}
	list, err := s.db.GetListByID(ctx, item.UserID, *item.ListID)
	if err != nil {
		return false
	}
	return list.Paused
}

func (s *Scheduler) dispatchOccurrence(ctx context.Context, occ storage.ReminderOccurrence, deliveryCfg notify.DeliveryConfig, now time.Time) {
	item, err := s.db.GetReminder(ctx, occ.UserID, occ.ReminderID)
	if err != nil {
		_ = s.db.MarkOccurrenceFired(ctx, occ.ID)
		return
	}
	if item.Status != reminder.StatusScheduled {
		_ = s.db.MarkOccurrenceFired(ctx, occ.ID)
		return
	}
	if s.reminderListPaused(ctx, item) {
		return
	}
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
	prefix := occurrencePrefix(occ)
	s.deliverToChannels(ctx, item, state, deliveryCfg, item.DueAt, prefix)
	_ = s.db.MarkOccurrenceFired(ctx, occ.ID)
}

func (s *Scheduler) dispatchReminder(ctx context.Context, item reminder.Reminder, deliveryCfg notify.DeliveryConfig, now time.Time) {
	if s.reminderListPaused(ctx, item) {
		return
	}
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
	s.deliverToChannels(ctx, item, state, deliveryCfg, item.DueAt, "🔔 Now:")
}

func (s *Scheduler) deliverToChannels(ctx context.Context, item reminder.Reminder, state storage.UserNotifyState, deliveryCfg notify.DeliveryConfig, dueAt time.Time, prefix string) {
	avail := notify.ChannelAvailability(deliveryCfg)
	channels := orderedChannels(state, avail)
	if len(channels) == 0 {
		return
	}
	for _, ch := range channels {
		pref := prefForChannel(state.Prefs, ch)
		if pref == nil {
			continue
		}
		key := storage.DeliveryIdempotencyKey(item.ID, ch, dueAt)
		d, err := s.db.CreateDelivery(ctx, storage.Delivery{
			ReminderID:     item.ID,
			UserID:         item.UserID,
			Channel:        ch,
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
		if s.sendDelivery(ctx, d, item, *pref, deliveryCfg, prefix) {
			return
		}
	}
}

func orderedChannels(state storage.UserNotifyState, avail map[string]bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, ch := range state.EscalationOrder {
		if !avail[ch] || seen[ch] {
			continue
		}
		seen[ch] = true
		out = append(out, ch)
	}
	for _, pref := range state.Prefs {
		if !pref.Enabled || !avail[pref.Channel] || strings.TrimSpace(pref.Address) == "" || seen[pref.Channel] {
			continue
		}
		seen[pref.Channel] = true
		out = append(out, pref.Channel)
	}
	return out
}

func prefForChannel(prefs []storage.NotificationPref, channel string) *storage.NotificationPref {
	for _, p := range prefs {
		if p.Channel == channel && p.Enabled && strings.TrimSpace(p.Address) != "" {
			return &p
		}
	}
	return nil
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
	if !s.sendDelivery(ctx, d, item, pref, deliveryCfg, "🔔 Now:") {
		s.tryEscalation(ctx, d, item, state, deliveryCfg)
	}
}

func (s *Scheduler) tryEscalation(ctx context.Context, failed storage.Delivery, item reminder.Reminder, state storage.UserNotifyState, deliveryCfg notify.DeliveryConfig) {
	avail := notify.ChannelAvailability(deliveryCfg)
	order := orderedChannels(state, avail)
	idx := -1
	for i, ch := range order {
		if ch == failed.Channel {
			idx = i
			break
		}
	}
	if idx < 0 || idx+1 >= len(order) {
		return
	}
	nextCh := order[idx+1]
	pref := prefForChannel(state.Prefs, nextCh)
	if pref == nil {
		return
	}
	key := storage.DeliveryIdempotencyKey(item.ID, nextCh, item.DueAt) + ":esc"
	d, err := s.db.CreateDelivery(ctx, storage.Delivery{
		ReminderID:     item.ID,
		UserID:         item.UserID,
		Channel:        nextCh,
		Status:         storage.DeliveryStatusPending,
		IdempotencyKey: key,
	})
	if err != nil {
		return
	}
	s.sendDelivery(ctx, d, item, *pref, deliveryCfg, "🔔 Now:")
}

// sendDelivery returns true on success.
func (s *Scheduler) sendDelivery(ctx context.Context, d storage.Delivery, item reminder.Reminder, pref storage.NotificationPref, deliveryCfg notify.DeliveryConfig, prefix string) bool {
	subject := fmt.Sprintf("%s %s", strings.TrimSpace(prefix), item.Title)
	body := fmt.Sprintf("<p><strong>%s</strong></p><p>%s</p><p>Due: %s</p>",
		escapeHTML(item.Title), escapeHTML(item.Description), item.DueAt.UTC().Format(time.RFC3339))
	text := fmt.Sprintf("%s %s\n%s\nDue: %s", strings.TrimSpace(prefix), item.Title, item.Description, item.DueAt.UTC().Format(time.RFC822))

	var err error
	switch d.Channel {
	case storage.ChannelEmail:
		err = notify.SendEmail(ctx, deliveryCfg, pref.Address, subject, body)
	case storage.ChannelTelegram:
		var tb strings.Builder
		tb.WriteString(strings.TrimSpace(prefix))
		tb.WriteString(" <b>")
		tb.WriteString(notify.EscapeTelegramHTML(item.Title))
		tb.WriteString("</b>")
		if desc := strings.TrimSpace(item.Description); desc != "" {
			tb.WriteString("\n")
			tb.WriteString(notify.EscapeTelegramHTML(desc))
		}
		tb.WriteString("\nDue: ")
		tb.WriteString(item.DueAt.UTC().Format(time.RFC822))
		deepLink := ""
		if base := strings.TrimRight(strings.TrimSpace(s.cfg.WebhookBaseURL), "/"); base != "" {
			deepLink = base + "/#/reminders?open=" + item.ID.String()
		}
		err = notify.SendTelegramMessage(ctx, deliveryCfg, pref.Address, tb.String(), notify.TelegramReminderKeyboard(item.ID.String(), deepLink))
	case storage.ChannelWebPush:
		err = notify.SendWebPush(ctx, deliveryCfg, pref.Address, subject, text)
	case storage.ChannelSMS:
		err = notify.SendSMS(ctx, notify.SMSConfig{
			AccountSID: deliveryCfg.TwilioAccountSID,
			AuthToken:  deliveryCfg.TwilioAuthToken,
			From:       deliveryCfg.TwilioFrom,
		}, pref.Address, text)
	case storage.ChannelSlack:
		err = notify.SendSlack(ctx, pref.Address, text)
	default:
		err = fmt.Errorf("unknown channel")
	}
	if err != nil {
		_ = s.db.MarkDeliveryFailed(ctx, d.ID, err.Error())
		return false
	}
	_ = s.db.MarkDeliverySent(ctx, d.ID)
	return true
}

func occurrencePrefix(occ storage.ReminderOccurrence) string {
	if occ.Kind == storage.OccurrenceKindOffset && occ.OffsetMinutes > 0 {
		return formatOffsetPrefix(occ.OffsetMinutes)
	}
	return "🔔 Now:"
}

func formatOffsetPrefix(minutes int) string {
	if minutes >= 10080 && minutes%10080 == 0 {
		w := minutes / 10080
		if w == 1 {
			return "⏰ In 1 week:"
		}
		return fmt.Sprintf("⏰ In %d weeks:", w)
	}
	if minutes >= 1440 && minutes%1440 == 0 {
		d := minutes / 1440
		if d == 1 {
			return "⏰ In 1 day:"
		}
		return fmt.Sprintf("⏰ In %d days:", d)
	}
	if minutes >= 60 && minutes%60 == 0 {
		h := minutes / 60
		if h == 1 {
			return "⏰ In 1 hour:"
		}
		return fmt.Sprintf("⏰ In %d hours:", h)
	}
	if minutes == 1 {
		return "⏰ In 1 minute:"
	}
	return fmt.Sprintf("⏰ In %d minutes:", minutes)
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

// ScheduleNextRecurrenceIfNeeded is called when a reminder is marked done.
func ScheduleNextRecurrenceIfNeeded(ctx context.Context, db *storage.DB, userID, reminderID uuid.UUID) {
	item, err := db.GetReminder(ctx, userID, reminderID)
	if err != nil || strings.TrimSpace(item.RecurrenceRule) == "" {
		return
	}
	if _, err := db.ScheduleNextRecurrence(ctx, userID, item); err != nil {
		log.Printf("scheduler: schedule next recurrence %s: %v", reminderID, err)
	}
}

// MaterializeReminderOccurrences materializes occurrence rows after reminder creation.
func MaterializeReminderOccurrences(ctx context.Context, db *storage.DB, userID uuid.UUID, item reminder.Reminder) {
	if err := db.MaterializeOccurrencesForReminder(ctx, userID, item); err != nil {
		log.Printf("scheduler: materialize occurrences %s: %v", item.ID, err)
	}
}
