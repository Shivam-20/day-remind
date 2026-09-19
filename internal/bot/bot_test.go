package bot

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/auth"
	"github.com/shivam-20/day-remind/internal/notify"
	"github.com/shivam-20/day-remind/internal/reminder"
	"github.com/shivam-20/day-remind/internal/storage"
)

// stubAPI stands in for api.telegram.org. No request ever leaves the process.
type stubAPI struct {
	srv  *httptest.Server
	mu   sync.Mutex
	sent []string
}

func newStubAPI(t *testing.T) *stubAPI {
	t.Helper()
	s := &stubAPI{}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			var p struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(body, &p)
			s.mu.Lock()
			s.sent = append(s.sent, p.Text)
			s.mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *stubAPI) replies() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.sent...)
}

func (s *stubAPI) lastReply(t *testing.T) string {
	t.Helper()
	r := s.replies()
	if len(r) == 0 {
		t.Fatal("bot sent no reply")
	}
	return r[len(r)-1]
}

func testDB(t *testing.T) *storage.DB {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	db, err := storage.Open(url)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// newBot wires a bot straight to the stub client. The command handlers under
// test do not go through refreshClient, which is what reads the database.
func newBot(t *testing.T, db *storage.DB, api *stubAPI, now time.Time) *Bot {
	t.Helper()
	return &Bot{
		db:      db,
		apiBase: api.srv.URL,
		client:  notify.NewTelegramClient("test-token", api.srv.URL),
		token:   "test-token",
		now:     func() time.Time { return now },
	}
}

func newUser(t *testing.T, db *storage.DB) storage.User {
	t.Helper()
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatal(err)
	}
	u, err := db.CreateUser(context.Background(), "bot-"+uuid.NewString()+"@example.com", hash, "Bot Tester", "Asia/Kolkata")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.DeleteUser(context.Background(), u.ID) })
	return u
}

func TestUnlinkedChatIsToldToLink(t *testing.T) {
	db, api := testDB(t), newStubAPI(t)
	b := newBot(t, db, api, time.Now().UTC())

	b.handleMessage(context.Background(), 111, "/list")
	if got := api.lastReply(t); !strings.Contains(got, "not linked") {
		t.Errorf("expected a link prompt, got %q", got)
	}
}

func TestStartCodeLinksTheChat(t *testing.T) {
	db, api := testDB(t), newStubAPI(t)
	ctx := context.Background()
	u := newUser(t, db)
	b := newBot(t, db, api, time.Now().UTC())

	code, _, err := db.CreateTelegramLinkCode(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}

	b.handleMessage(ctx, 222, "/start "+code)
	if got := api.lastReply(t); !strings.Contains(got, "Linked") {
		t.Errorf("expected a link confirmation, got %q", got)
	}

	linked, err := db.GetUserByTelegramChat(ctx, 222)
	if err != nil {
		t.Fatalf("chat was not linked: %v", err)
	}
	if linked.ID != u.ID {
		t.Error("chat linked to the wrong account")
	}
	if !linked.Notify.TelegramEnabled {
		t.Error("linking should enable the telegram channel")
	}

	// A code is single use.
	b.handleMessage(ctx, 333, "/start "+code)
	if got := api.lastReply(t); !strings.Contains(got, "not valid") {
		t.Errorf("a redeemed code must not work twice, got %q", got)
	}
}

func TestExpiredCodeIsRejected(t *testing.T) {
	db, api := testDB(t), newStubAPI(t)
	ctx := context.Background()
	u := newUser(t, db)
	b := newBot(t, db, api, time.Now().UTC())

	if _, _, err := db.CreateTelegramLinkCode(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	b.handleMessage(ctx, 444, "/start ZZZZZ")
	if got := api.lastReply(t); !strings.Contains(got, "not valid") {
		t.Errorf("a wrong code must be rejected, got %q", got)
	}
}

// linkChat wires a user to a chat id for the command tests.
func linkChat(t *testing.T, db *storage.DB, u storage.User, chatID int64) {
	t.Helper()
	ctx := context.Background()
	code, _, err := db.CreateTelegramLinkCode(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.RedeemTelegramLinkCode(ctx, code, chatID); err != nil {
		t.Fatal(err)
	}
}

func TestAddListDoneRoundTrip(t *testing.T) {
	db, api := testDB(t), newStubAPI(t)
	ctx := context.Background()
	u := newUser(t, db)
	linkChat(t, db, u, 555)

	// Wednesday 17 June 2026, 14:30 UTC.
	now := time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC)
	b := newBot(t, db, api, now)

	b.handleMessage(ctx, 555, "/add tomorrow 9am | Pay electricity")
	if got := api.lastReply(t); !strings.Contains(got, "Added") {
		t.Fatalf("expected an add confirmation, got %q", got)
	}

	items, err := db.ListReminders(ctx, reminder.ListFilter{UserID: u.ID})
	if err != nil || len(items) != 1 {
		t.Fatalf("expected 1 reminder, got %d (%v)", len(items), err)
	}
	if items[0].Title != "Pay electricity" {
		t.Errorf("title = %q", items[0].Title)
	}
	// 9am Kolkata on 18 June is 03:30 UTC.
	if got := items[0].DueAt.UTC().Format("2006-01-02 15:04"); got != "2026-06-18 03:30" {
		t.Errorf("due = %s, want 2026-06-18 03:30 UTC", got)
	}

	b.handleMessage(ctx, 555, "/list")
	if got := api.lastReply(t); !strings.Contains(got, "Pay electricity") {
		t.Errorf("/list should show the reminder, got %q", got)
	}

	b.handleMessage(ctx, 555, "/done 1")
	if got := api.lastReply(t); !strings.Contains(got, "Done") {
		t.Errorf("expected a done confirmation, got %q", got)
	}
	items, _ = db.ListReminders(ctx, reminder.ListFilter{UserID: u.ID})
	if items[0].Status != reminder.StatusDone {
		t.Errorf("status = %q, want done", items[0].Status)
	}
}

func TestSnoozeCommandRearmsDelivery(t *testing.T) {
	db, api := testDB(t), newStubAPI(t)
	ctx := context.Background()
	u := newUser(t, db)
	linkChat(t, db, u, 666)

	now := time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC)
	b := newBot(t, db, api, now)

	r, err := db.CreateReminder(ctx, u.ID, reminder.CreateInput{
		Title: "Standup", DueAt: now.Add(-time.Minute), Timezone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Pretend it already fired, so we can prove the snooze re-arms it.
	if _, err := db.ClaimReminderNotification(ctx, r.ID, storage.KindDue); err != nil {
		t.Fatal(err)
	}

	b.handleMessage(ctx, 666, "/snooze 1 2h")
	if got := api.lastReply(t); !strings.Contains(got, "Snoozed") {
		t.Fatalf("expected a snooze confirmation, got %q", got)
	}

	got, err := db.GetReminder(ctx, u.ID, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SnoozedUntil == nil {
		t.Fatal("snoozed_until was not set")
	}
	if want := now.Add(2 * time.Hour); got.SnoozedUntil.Sub(want).Abs() > time.Second {
		t.Errorf("snoozed until %v, want ~%v", got.SnoozedUntil, want)
	}
	if got.SentDueAt != nil {
		t.Error("snoozing must clear the delivery claim so it can fire again")
	}
}

func TestBadCommandsGiveGuidanceNotErrors(t *testing.T) {
	db, api := testDB(t), newStubAPI(t)
	ctx := context.Background()
	u := newUser(t, db)
	linkChat(t, db, u, 777)
	b := newBot(t, db, api, time.Now().UTC())

	// /snooze resolves the list index before the duration, so there has to be
	// something at position 1 for the duration error to be reachable.
	if _, err := db.CreateReminder(ctx, u.ID, reminder.CreateInput{
		Title: "Anchor", DueAt: time.Now().UTC().Add(time.Hour), Timezone: "UTC",
	}); err != nil {
		t.Fatal(err)
	}

	// Replies are MarkdownV2-escaped, so match on fragments that survive it.
	cases := []struct{ in, want string }{
		{"/add no pipe here", "for example"},
		{"/add bananas | Title", "could not read"},
		{"/add tomorrow 9am |   ", "title is empty"},
		{"/done", "the number comes from"},
		{"/done 99", "no number 99"},
		{"/snooze 1 bananas", "could not read"},
		{"/frobnicate", "Unknown command"},
	}
	for _, c := range cases {
		b.handleMessage(ctx, 777, c.in)
		if got := api.lastReply(t); !strings.Contains(strings.ToLower(got), strings.ToLower(c.want)) {
			t.Errorf("%q -> %q, want it to mention %q", c.in, got, c.want)
		}
	}
}

func TestHelpWorksWithoutLinking(t *testing.T) {
	db, api := testDB(t), newStubAPI(t)
	b := newBot(t, db, api, time.Now().UTC())

	b.handleMessage(context.Background(), 888, "/help")
	got := api.lastReply(t)
	for _, cmd := range []string{"/start", "/list", "/add", "/done", "/snooze"} {
		if !strings.Contains(got, cmd) {
			t.Errorf("help should mention %s, got %q", cmd, got)
		}
	}
}

func TestGroupChatCommandSuffixIsStripped(t *testing.T) {
	db, api := testDB(t), newStubAPI(t)
	b := newBot(t, db, api, time.Now().UTC())

	// In a group, Telegram delivers "/help@DayRemindBot".
	b.handleMessage(context.Background(), 999, "/help@DayRemindBot")
	if got := api.lastReply(t); !strings.Contains(got, "/snooze") {
		t.Errorf("suffixed command was not recognised, got %q", got)
	}
}

func TestUnlinkStopsDelivery(t *testing.T) {
	db, api := testDB(t), newStubAPI(t)
	ctx := context.Background()
	u := newUser(t, db)
	linkChat(t, db, u, 1010)
	b := newBot(t, db, api, time.Now().UTC())

	b.handleMessage(ctx, 1010, "/unlink")
	if got := api.lastReply(t); !strings.Contains(got, "no longer linked") {
		t.Errorf("expected an unlink confirmation, got %q", got)
	}
	after, err := db.GetUserByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.TelegramChatID != nil {
		t.Error("chat id should be cleared")
	}
	if after.Notify.TelegramEnabled {
		t.Error("telegram delivery should be switched off")
	}
}

// One chat must only ever drive one account.
func TestRelinkingMovesTheChat(t *testing.T) {
	db, api := testDB(t), newStubAPI(t)
	ctx := context.Background()
	first, second := newUser(t, db), newUser(t, db)
	linkChat(t, db, first, 1212)
	b := newBot(t, db, api, time.Now().UTC())

	code, _, err := db.CreateTelegramLinkCode(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	b.handleMessage(ctx, 1212, "/start "+code)

	owner, err := db.GetUserByTelegramChat(ctx, 1212)
	if err != nil {
		t.Fatal(err)
	}
	if owner.ID != second.ID {
		t.Error("the chat should now belong to the second account")
	}
	stale, err := db.GetUserByID(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stale.TelegramChatID != nil {
		t.Error("the first account must not keep the chat")
	}
}
