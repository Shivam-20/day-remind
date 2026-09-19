// Package scheduler sweeps for reminders that are due and dispatches them over
// each user's enabled channels.
package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/delivery"
	"github.com/shivam-20/day-remind/internal/notify"
	"github.com/shivam-20/day-remind/internal/reminder"
	"github.com/shivam-20/day-remind/internal/storage"
)

// Scheduler polls for due reminders on an interval. One instance runs inside
// the server process; the claim mechanism in storage keeps it correct even if
// more than one process is running.
type Scheduler struct {
	db       *storage.DB
	interval time.Duration
	resolver *delivery.Resolver

	// now is injectable so tests can control time.
	now func() time.Time
	// overrideSenders replaces the resolved transport by channel name. Tests
	// set it so delivery can be exercised without a network; it is nil in
	// production and the resolver's own senders are used.
	overrideSenders map[string]notify.Sender
}

func New(db *storage.DB, interval time.Duration, resolver *delivery.Resolver) *Scheduler {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &Scheduler{db: db, interval: interval, resolver: resolver, now: func() time.Time { return time.Now().UTC() }}
}

// Run blocks until ctx is cancelled, sweeping on each tick. It sweeps once
// immediately so a restart delivers anything that came due while down.
func (s *Scheduler) Run(ctx context.Context) {
	// Channels are configured in the database and can be turned on at any
	// time, so the sweep always runs; it simply finds no targets until
	// something is configured.
	log.Printf("scheduler: started, sweeping every %s", s.interval)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		if n, err := s.Tick(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("scheduler: sweep failed: %v", err)
		} else if n > 0 {
			log.Printf("scheduler: dispatched %d notification(s)", n)
		}
		select {
		case <-ctx.Done():
			log.Println("scheduler: stopped")
			return
		case <-ticker.C:
		}
	}
}

// Tick performs one sweep and reports how many notifications were dispatched.
func (s *Scheduler) Tick(ctx context.Context) (int, error) {
	now := s.now()

	// One read of the instance settings per sweep, not per user.
	snap, err := s.resolver.Snapshot(ctx)
	if err != nil {
		return 0, err
	}

	users, err := s.db.ListNotifiableUsers(ctx)
	if err != nil {
		return 0, err
	}
	if len(users) == 0 {
		return 0, nil
	}

	byID := make(map[uuid.UUID]storage.User, len(users))
	maxAdvance := time.Duration(0)
	for _, u := range users {
		byID[u.ID] = u
		if d := time.Duration(u.Notify.AdvanceMinutes) * time.Minute; d > maxAdvance {
			maxAdvance = d
		}
	}

	sent := 0

	// Reminders that are due right now.
	due, err := s.db.ListPendingReminders(ctx, storage.KindDue, now, 500)
	if err != nil {
		return sent, err
	}
	for _, r := range due {
		u, ok := byID[r.UserID]
		if !ok {
			continue
		}
		// A snooze can hold a reminder back past its due date.
		if storage.FireTime(r).After(now) {
			continue
		}
		if s.dispatch(ctx, snap, u, r, storage.KindDue, 0) {
			sent++
		}
	}

	// The advance heads-up, for users who asked for one.
	if maxAdvance > 0 {
		ahead, err := s.db.ListPendingReminders(ctx, storage.KindAdvance, now.Add(maxAdvance), 500)
		if err != nil {
			return sent, err
		}
		for _, r := range ahead {
			u, ok := byID[r.UserID]
			if !ok || u.Notify.AdvanceMinutes <= 0 {
				continue
			}
			lead := time.Duration(u.Notify.AdvanceMinutes) * time.Minute
			fire := storage.FireTime(r)
			// Only inside this user's own window, and not once already due —
			// at that point the due notification covers it.
			if fire.After(now.Add(lead)) || !fire.After(now) {
				continue
			}
			if s.dispatch(ctx, snap, u, r, storage.KindAdvance, lead) {
				sent++
			}
		}
	}

	return sent, nil
}

// dispatch claims a notification and sends it on every enabled channel. It
// reports whether anything was actually delivered.
func (s *Scheduler) dispatch(ctx context.Context, snap *delivery.Snapshot, u storage.User, r reminder.Reminder, kind storage.DueKind, lead time.Duration) bool {
	if u.NotificationsCurrentlyPaused(s.now()) {
		return false
	}
	targets := snap.For(u)
	if len(targets) == 0 {
		return false
	}
	if s.overrideSenders != nil {
		for i := range targets {
			if sender, ok := s.overrideSenders[targets[i].Channel]; ok {
				targets[i].Sender = sender
			}
		}
	}

	// Claim first: if this fails, another worker or an earlier tick already
	// owns this notification and we must not send a duplicate.
	claimed, err := s.db.ClaimReminderNotification(ctx, r.ID, kind)
	if err != nil {
		log.Printf("scheduler: claim %s: %v", r.ID, err)
		return false
	}
	if !claimed {
		return false
	}

	msg := notify.Message{
		ReminderID:  r.ID.String(),
		Title:       r.Title,
		Description: r.Description,
		DueAt:       storage.FireTime(r),
		Timezone:    r.Timezone,
		Priority:    r.Priority,
		Advance:     kind == storage.KindAdvance,
		LeadTime:    lead,
		DisplayName: u.DisplayName,
	}

	var wg sync.WaitGroup
	results := make([]bool, len(targets))
	for i, t := range targets {
		wg.Add(1)
		go func(i int, t delivery.Target) {
			defer wg.Done()

			// Rendered per target: two channels can have different templates,
			// and a template problem on one must not affect the other.
			rendered, problems := notify.Render(t.Templates, msg)
			for _, p := range problems {
				log.Printf("scheduler: template problem for %s (%s), using the default: %v", u.Email, t.Channel, p)
			}

			sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			err := t.Sender.Send(sendCtx, t.Dest, msg, rendered)
			entry := storage.Delivery{
				UserID: u.ID, ReminderID: r.ID,
				Channel: t.Channel, Kind: string(kind),
				Status: storage.DeliverySent,
			}
			if err != nil {
				entry.Status = storage.DeliveryFailed
				entry.Detail = err.Error()
				log.Printf("scheduler: %s to %s failed: %v", t.Channel, u.Email, err)
			} else {
				results[i] = true
			}
			if logErr := s.db.LogDelivery(ctx, entry); logErr != nil {
				log.Printf("scheduler: could not record delivery: %v", logErr)
			}
		}(i, t)
	}
	wg.Wait()

	for _, ok := range results {
		if ok {
			return true
		}
	}

	// Every channel failed, so release the claim and let a later tick retry
	// rather than losing the notification entirely.
	if err := s.db.ReleaseReminderNotification(ctx, r.ID, kind); err != nil {
		log.Printf("scheduler: could not release claim on %s: %v", r.ID, err)
	}
	return false
}
