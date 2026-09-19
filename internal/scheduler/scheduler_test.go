package scheduler

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/auth"
	"github.com/shivam-20/day-remind/internal/delivery"
	"github.com/shivam-20/day-remind/internal/notify"
	"github.com/shivam-20/day-remind/internal/reminder"
	"github.com/shivam-20/day-remind/internal/secret"
	"github.com/shivam-20/day-remind/internal/storage"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// fakeSender records what would have been delivered. Nothing leaves the process.
type fakeSender struct {
	name string
	mu   sync.Mutex
	sent []notify.Message
	fail error
}

func (f *fakeSender) Name() string { return f.name }
func (f *fakeSender) Send(_ context.Context, dest string, m notify.Message, r notify.Rendered) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return f.fail
	}
	f.sent = append(f.sent, m)
	return nil
}
func (f *fakeSender) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

// testDB gives each test its own database. Tick deliberately sweeps every
// notifiable user, so tests sharing one database would see each other's
// reminders — and anything left in a developer's local database as well.
func testDB(t *testing.T) *storage.DB {
	t.Helper()
	raw := os.Getenv("DATABASE_URL")
	if raw == "" {
		t.Skip("DATABASE_URL not set")
	}
	name := "day_remind_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse DATABASE_URL: %v", err)
	}
	u.Path = "/" + name

	db, err := storage.Open(u.String())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		// Drop through a separate client: the storage handle has no drop, and
		// leaving per-test databases behind would pile up fast.
		client, err := mongo.Connect(ctx, options.Client().ApplyURI(u.String()))
		if err == nil {
			_ = client.Database(name).Drop(ctx)
			_ = client.Disconnect(ctx)
		}
		_ = db.Close()
	})
	return db
}

// newUser creates an isolated user and removes it (and its reminders) after.
func newUser(t *testing.T, db *storage.DB, prefs storage.NotifyPrefs, chatID *int64) storage.User {
	t.Helper()
	ctx := context.Background()
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatal(err)
	}
	u, err := db.CreateUser(ctx, "sched-"+uuid.NewString()+"@example.com", hash, "Sched", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if prefs.EmailAddress == "" {
		prefs.EmailAddress = u.Email
	}
	if _, err := db.SetNotifyPrefs(ctx, u.ID, prefs); err != nil {
		t.Fatal(err)
	}
	if chatID != nil {
		code, _, err := db.CreateTelegramLinkCode(ctx, u.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.RedeemTelegramLinkCode(ctx, code, *chatID); err != nil {
			t.Fatal(err)
		}
		// Redeeming enables telegram; reapply the caller's intent.
		if _, err := db.SetNotifyPrefs(ctx, u.ID, prefs); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.GetUserByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func mkReminder(t *testing.T, db *storage.DB, userID uuid.UUID, title string, due time.Time) reminder.Reminder {
	t.Helper()
	r, err := db.CreateReminder(context.Background(), userID, reminder.CreateInput{
		Title: title, DueAt: due, Timezone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

const testSessionKey = "scheduler-test-session-key-32-chars-min"

// configureInstance switches on the channels the fake senders stand in for, so
// the resolver produces a target for them.
func configureInstance(t *testing.T, db *storage.DB, email, telegram bool) {
	t.Helper()
	var inst storage.InstanceSettings
	if email {
		inst.SMTP = storage.SMTPSettings{
			Enabled: true, Host: "smtp.invalid", Port: 587,
			From: "reminders@example.com", TLS: storage.TLSStartTLS,
		}
	}
	if telegram {
		box, err := secret.NewBox(testSessionKey)
		if err != nil {
			t.Fatal(err)
		}
		enc, err := box.Encrypt("test-bot-token")
		if err != nil {
			t.Fatal(err)
		}
		inst.Telegram = storage.TelegramSettings{Enabled: true, BotTokenEnc: enc}
	}
	if err := db.SaveInstanceSettings(context.Background(), inst, uuid.Nil); err != nil {
		t.Fatal(err)
	}
}

// stubResolver swaps the real transports for the fakes while keeping the
// resolver's own precedence logic in play.
type stubResolver struct {
	*delivery.Resolver
	senders map[string]notify.Sender
}

// at builds a scheduler whose clock is pinned, so tests are not racing wall
// time, and whose senders are fakes so nothing leaves the process.
func at(t *testing.T, db *storage.DB, senders map[string]notify.Sender, now time.Time) *Scheduler {
	t.Helper()
	box, err := secret.NewBox(testSessionKey)
	if err != nil {
		t.Fatal(err)
	}
	configureInstance(t, db, senders[notify.ChannelEmail] != nil, senders[notify.ChannelTelegram] != nil)

	s := New(db, time.Minute, delivery.NewResolver(db, box, "http://127.0.0.1:1"))
	s.now = func() time.Time { return now }
	s.overrideSenders = senders
	return s
}

func TestDueReminderIsDeliveredExactlyOnce(t *testing.T) {
	db := testDB(t)
	email := &fakeSender{name: notify.ChannelEmail}
	senders := map[string]notify.Sender{notify.ChannelEmail: email}

	now := time.Now().UTC()
	u := newUser(t, db, storage.NotifyPrefs{EmailEnabled: true}, nil)
	mkReminder(t, db, u.ID, "due now", now.Add(-time.Minute))

	s := at(t, db, senders, now)
	if _, err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if email.count() != 1 {
		t.Fatalf("expected 1 delivery, got %d", email.count())
	}

	// The whole point of the claim: a second sweep must not resend.
	for i := 0; i < 3; i++ {
		if _, err := s.Tick(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if email.count() != 1 {
		t.Errorf("reminder was delivered %d times, want exactly 1", email.count())
	}
}

func TestFutureReminderIsNotDelivered(t *testing.T) {
	db := testDB(t)
	email := &fakeSender{name: notify.ChannelEmail}
	now := time.Now().UTC()

	u := newUser(t, db, storage.NotifyPrefs{EmailEnabled: true}, nil)
	mkReminder(t, db, u.ID, "later", now.Add(2*time.Hour))

	if _, err := at(t, db, map[string]notify.Sender{notify.ChannelEmail: email}, now).Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if email.count() != 0 {
		t.Errorf("a reminder due in 2h should not fire, got %d deliveries", email.count())
	}
}

func TestAdvanceNoticeFiresThenDueFires(t *testing.T) {
	db := testDB(t)
	email := &fakeSender{name: notify.ChannelEmail}
	senders := map[string]notify.Sender{notify.ChannelEmail: email}

	now := time.Now().UTC()
	u := newUser(t, db, storage.NotifyPrefs{EmailEnabled: true, AdvanceMinutes: 60}, nil)
	mkReminder(t, db, u.ID, "meeting", now.Add(30*time.Minute))

	// Inside the 60m window: the advance notice fires, the due one does not.
	if _, err := at(t, db, senders, now).Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if email.count() != 1 {
		t.Fatalf("expected the advance notice, got %d deliveries", email.count())
	}
	if !email.sent[0].Advance {
		t.Error("first delivery should be flagged as an advance notice")
	}
	if email.sent[0].LeadTime != time.Hour {
		t.Errorf("lead time = %v, want 1h", email.sent[0].LeadTime)
	}

	// A second sweep in the same window must not repeat the advance notice.
	if _, err := at(t, db, senders, now.Add(5*time.Minute)).Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if email.count() != 1 {
		t.Fatalf("advance notice repeated: %d deliveries", email.count())
	}

	// Once the due time passes, the due notification fires — and only once.
	later := now.Add(31 * time.Minute)
	if _, err := at(t, db, senders, later).Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := at(t, db, senders, later.Add(time.Minute)).Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if email.count() != 2 {
		t.Fatalf("expected advance + due = 2 deliveries, got %d", email.count())
	}
	if email.sent[1].Advance {
		t.Error("second delivery should be the due notification, not an advance one")
	}
}

func TestVacationModeSuppressesDelivery(t *testing.T) {
	db := testDB(t)
	email := &fakeSender{name: notify.ChannelEmail}
	senders := map[string]notify.Sender{notify.ChannelEmail: email}
	ctx := context.Background()
	now := time.Now().UTC()

	u := newUser(t, db, storage.NotifyPrefs{EmailEnabled: true}, nil)
	r := mkReminder(t, db, u.ID, "quiet please", now.Add(-time.Minute))

	if err := db.SetNotificationsPaused(ctx, u.ID, true, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := at(t, db, senders, now).Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if email.count() != 0 {
		t.Fatalf("vacation mode should suppress delivery, got %d", email.count())
	}

	// Suppressed, not consumed: resuming must still deliver it.
	if err := db.SetNotificationsPaused(ctx, u.ID, false, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := at(t, db, senders, now).Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if email.count() != 1 {
		t.Errorf("expected delivery after resuming, got %d", email.count())
	}
	_ = r
}

func TestExpiredVacationWindowResumesDelivery(t *testing.T) {
	db := testDB(t)
	email := &fakeSender{name: notify.ChannelEmail}
	ctx := context.Background()
	now := time.Now().UTC()

	u := newUser(t, db, storage.NotifyPrefs{EmailEnabled: true}, nil)
	mkReminder(t, db, u.ID, "after the holiday", now.Add(-time.Minute))

	// Paused until a moment that has already passed.
	past := now.Add(-time.Hour)
	if err := db.SetNotificationsPaused(ctx, u.ID, true, &past); err != nil {
		t.Fatal(err)
	}
	if _, err := at(t, db, map[string]notify.Sender{notify.ChannelEmail: email}, now).Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if email.count() != 1 {
		t.Errorf("an expired pause window should not suppress delivery, got %d", email.count())
	}
}

func TestPausedAndDoneRemindersNeverFire(t *testing.T) {
	db := testDB(t)
	email := &fakeSender{name: notify.ChannelEmail}
	ctx := context.Background()
	now := time.Now().UTC()

	u := newUser(t, db, storage.NotifyPrefs{EmailEnabled: true}, nil)
	paused := mkReminder(t, db, u.ID, "paused", now.Add(-time.Hour))
	finished := mkReminder(t, db, u.ID, "done", now.Add(-time.Hour))

	if _, err := db.PauseReminder(ctx, u.ID, paused.ID, nil); err != nil {
		t.Fatal(err)
	}
	doneStatus := reminder.StatusDone
	if _, err := db.UpdateReminder(ctx, u.ID, finished.ID, reminder.UpdateInput{Status: &doneStatus}); err != nil {
		t.Fatal(err)
	}

	if _, err := at(t, db, map[string]notify.Sender{notify.ChannelEmail: email}, now).Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if email.count() != 0 {
		t.Errorf("paused and done reminders must not fire, got %d deliveries", email.count())
	}
}

func TestSnoozeDefersThenRearms(t *testing.T) {
	db := testDB(t)
	email := &fakeSender{name: notify.ChannelEmail}
	senders := map[string]notify.Sender{notify.ChannelEmail: email}
	ctx := context.Background()
	now := time.Now().UTC()

	u := newUser(t, db, storage.NotifyPrefs{EmailEnabled: true}, nil)
	r := mkReminder(t, db, u.ID, "snoozed", now.Add(-time.Minute))

	// Deliver once, then snooze it an hour out.
	if _, err := at(t, db, senders, now).Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if email.count() != 1 {
		t.Fatalf("expected the first delivery, got %d", email.count())
	}
	if _, err := db.SnoozeReminder(ctx, u.ID, r.ID, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	// Still snoozed: nothing.
	if _, err := at(t, db, senders, now.Add(30*time.Minute)).Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if email.count() != 1 {
		t.Fatalf("a snoozed reminder must stay quiet, got %d", email.count())
	}

	// Past the snooze: it fires again.
	if _, err := at(t, db, senders, now.Add(61*time.Minute)).Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if email.count() != 2 {
		t.Errorf("expected the snoozed reminder to fire again, got %d", email.count())
	}
}

func TestFailedSendIsRetriedOnTheNextSweep(t *testing.T) {
	db := testDB(t)
	email := &fakeSender{name: notify.ChannelEmail, fail: fmt.Errorf("smtp unavailable")}
	senders := map[string]notify.Sender{notify.ChannelEmail: email}
	ctx := context.Background()
	now := time.Now().UTC()

	u := newUser(t, db, storage.NotifyPrefs{EmailEnabled: true}, nil)
	mkReminder(t, db, u.ID, "flaky", now.Add(-time.Minute))

	if _, err := at(t, db, senders, now).Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if email.count() != 0 {
		t.Fatalf("send failed, nothing should be recorded as sent")
	}

	// The claim must have been released, so a recovered channel delivers it.
	email.mu.Lock()
	email.fail = nil
	email.mu.Unlock()

	if _, err := at(t, db, senders, now.Add(time.Minute)).Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if email.count() != 1 {
		t.Errorf("a failed delivery should be retried, got %d", email.count())
	}
}

func TestDeliversToBothChannels(t *testing.T) {
	db := testDB(t)
	email := &fakeSender{name: notify.ChannelEmail}
	tg := &fakeSender{name: notify.ChannelTelegram}
	senders := map[string]notify.Sender{notify.ChannelEmail: email, notify.ChannelTelegram: tg}

	now := time.Now().UTC()
	chat := int64(987654321)
	u := newUser(t, db, storage.NotifyPrefs{EmailEnabled: true, TelegramEnabled: true}, &chat)
	if u.TelegramChatID == nil {
		t.Fatal("telegram chat was not linked")
	}
	mkReminder(t, db, u.ID, "both channels", now.Add(-time.Minute))

	if _, err := at(t, db, senders, now).Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if email.count() != 1 || tg.count() != 1 {
		t.Errorf("expected 1 delivery per channel, got email=%d telegram=%d", email.count(), tg.count())
	}
}

func TestUsersDoNotReceiveEachOthersReminders(t *testing.T) {
	db := testDB(t)
	email := &fakeSender{name: notify.ChannelEmail}
	now := time.Now().UTC()

	a := newUser(t, db, storage.NotifyPrefs{EmailEnabled: true}, nil)
	b := newUser(t, db, storage.NotifyPrefs{EmailEnabled: true}, nil)
	mkReminder(t, db, a.ID, "belongs to A", now.Add(-time.Minute))

	if _, err := at(t, db, map[string]notify.Sender{notify.ChannelEmail: email}, now).Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if email.count() != 1 {
		t.Fatalf("expected exactly one delivery, got %d", email.count())
	}
	if email.sent[0].Title != "belongs to A" {
		t.Errorf("wrong reminder delivered: %q", email.sent[0].Title)
	}
	_ = b
}
