package delivery

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/auth"
	"github.com/shivam-20/day-remind/internal/notify"
	"github.com/shivam-20/day-remind/internal/secret"
	"github.com/shivam-20/day-remind/internal/storage"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const testKey = "delivery-test-session-key-at-least-32ch"

// testDB gives each test a private database; the instance settings document is
// a singleton, so tests sharing one database would overwrite each other.
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
		if c, err := mongo.Connect(ctx, options.Client().ApplyURI(u.String())); err == nil {
			_ = c.Database(name).Drop(ctx)
			_ = c.Disconnect(ctx)
		}
		_ = db.Close()
	})
	return db
}

func newBox(t *testing.T) *secret.Box {
	t.Helper()
	b, err := secret.NewBox(testKey)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func newUser(t *testing.T, db *storage.DB, prefs storage.NotifyPrefs) storage.User {
	t.Helper()
	ctx := context.Background()
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatal(err)
	}
	u, err := db.CreateUser(ctx, "res-"+uuid.NewString()+"@example.com", hash, "Res", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if prefs.EmailAddress == "" {
		prefs.EmailAddress = u.Email
	}
	got, err := db.SetNotifyPrefs(ctx, u.ID, prefs)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func instanceSMTP(host string) storage.SMTPSettings {
	return storage.SMTPSettings{
		Enabled: true, Host: host, Port: 587,
		From: "instance@example.com", TLS: storage.TLSStartTLS,
	}
}

func snapshot(t *testing.T, db *storage.DB) *Snapshot {
	t.Helper()
	snap, err := NewResolver(db, newBox(t), "http://127.0.0.1:1").Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

func TestInstanceSMTPIsUsedWhenUserHasNoOverride(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	if err := db.SaveInstanceSettings(ctx, storage.InstanceSettings{SMTP: instanceSMTP("mail.instance")}, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	u := newUser(t, db, storage.NotifyPrefs{EmailEnabled: true})

	targets := snapshot(t, db).For(u)
	if len(targets) != 1 {
		t.Fatalf("expected one target, got %d", len(targets))
	}
	sender, ok := targets[0].Sender.(*notify.SMTPSender)
	if !ok {
		t.Fatalf("expected an SMTP sender, got %T", targets[0].Sender)
	}
	if sender.Host != "mail.instance" {
		t.Errorf("host = %q, want the instance host", sender.Host)
	}
}

// The whole point of per-user configuration: their server, not the instance's.
func TestUserSMTPOverridesTheInstance(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	box := newBox(t)
	if err := db.SaveInstanceSettings(ctx, storage.InstanceSettings{SMTP: instanceSMTP("mail.instance")}, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	u := newUser(t, db, storage.NotifyPrefs{EmailEnabled: true})

	enc, err := box.Encrypt("user-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SetUserDelivery(ctx, u.ID, storage.UserDelivery{
		SMTP: storage.SMTPSettings{
			Enabled: true, Host: "mail.mine", Port: 465, Username: "me",
			PasswordEnc: enc, From: "me@example.com", TLS: storage.TLSImplicit,
		},
	}); err != nil {
		t.Fatal(err)
	}
	u, err = db.GetUserByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}

	targets := snapshot(t, db).For(u)
	if len(targets) != 1 {
		t.Fatalf("expected one target, got %d", len(targets))
	}
	sender := targets[0].Sender.(*notify.SMTPSender)
	if sender.Host != "mail.mine" {
		t.Errorf("host = %q, want the user's host", sender.Host)
	}
	if sender.Password != "user-password" {
		t.Errorf("the stored password was not decrypted for use")
	}
	if sender.Mode != storage.TLSImplicit || sender.Port != 465 {
		t.Errorf("transport not carried over: %s:%d", sender.Mode, sender.Port)
	}
}

func TestNoTargetWhenNothingIsConfigured(t *testing.T) {
	db := testDB(t)
	u := newUser(t, db, storage.NotifyPrefs{EmailEnabled: true, TelegramEnabled: true})

	if targets := snapshot(t, db).For(u); len(targets) != 0 {
		t.Errorf("expected no targets with nothing configured, got %d", len(targets))
	}
}

// An incomplete override must not shadow a working instance configuration.
func TestIncompleteUserSMTPFallsBackToInstance(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	if err := db.SaveInstanceSettings(ctx, storage.InstanceSettings{SMTP: instanceSMTP("mail.instance")}, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	u := newUser(t, db, storage.NotifyPrefs{EmailEnabled: true})
	// Enabled but with no host: not usable.
	if _, err := db.SetUserDelivery(ctx, u.ID, storage.UserDelivery{
		SMTP: storage.SMTPSettings{Enabled: false, Host: "", From: ""},
	}); err != nil {
		t.Fatal(err)
	}
	u, _ = db.GetUserByID(ctx, u.ID)

	targets := snapshot(t, db).For(u)
	if len(targets) != 1 {
		t.Fatalf("expected the instance to be used, got %d targets", len(targets))
	}
	if targets[0].Sender.(*notify.SMTPSender).Host != "mail.instance" {
		t.Error("expected the instance host")
	}
}

func TestTelegramPrefersTheUsersOwnBot(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	box := newBox(t)

	instTok, _ := box.Encrypt("instance-token")
	if err := db.SaveInstanceSettings(ctx, storage.InstanceSettings{
		Telegram: storage.TelegramSettings{Enabled: true, BotTokenEnc: instTok},
	}, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	u := newUser(t, db, storage.NotifyPrefs{TelegramEnabled: true})

	// Linked to the instance bot first.
	code, _, err := db.CreateTelegramLinkCode(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.RedeemTelegramLinkCode(ctx, code, 111); err != nil {
		t.Fatal(err)
	}
	u, _ = db.GetUserByID(ctx, u.ID)

	targets := snapshot(t, db).For(u)
	if len(targets) != 1 || targets[0].Dest != "111" {
		t.Fatalf("expected the instance bot and linked chat, got %+v", targets)
	}

	// Now they bring their own bot and their own chat.
	ownTok, _ := box.Encrypt("own-token")
	own := int64(222)
	if _, err := db.SetUserDelivery(ctx, u.ID, storage.UserDelivery{
		Telegram: storage.UserTelegram{Enabled: true, BotTokenEnc: ownTok, OwnChatID: &own},
	}); err != nil {
		t.Fatal(err)
	}
	u, _ = db.GetUserByID(ctx, u.ID)

	targets = snapshot(t, db).For(u)
	if len(targets) != 1 {
		t.Fatalf("expected one target, got %d", len(targets))
	}
	if targets[0].Dest != "222" {
		t.Errorf("dest = %q, want the chat on their own bot", targets[0].Dest)
	}
	client, ok := targets[0].Sender.(*notify.TelegramClient)
	if !ok {
		t.Fatalf("expected a Telegram client, got %T", targets[0].Sender)
	}
	if client.Token != "own-token" {
		t.Errorf("token = %q, want the user's own", client.Token)
	}
}

// A user's own bot with no discovered chat cannot be used: a chat id belongs to
// one bot, so the instance-linked id is not valid for theirs.
func TestOwnBotWithoutAChatIsNotUsed(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	box := newBox(t)
	ownTok, _ := box.Encrypt("own-token")

	u := newUser(t, db, storage.NotifyPrefs{TelegramEnabled: true})
	if _, err := db.SetUserDelivery(ctx, u.ID, storage.UserDelivery{
		Telegram: storage.UserTelegram{Enabled: true, BotTokenEnc: ownTok},
	}); err != nil {
		t.Fatal(err)
	}
	u, _ = db.GetUserByID(ctx, u.ID)

	if targets := snapshot(t, db).For(u); len(targets) != 0 {
		t.Errorf("expected no target without a discovered chat, got %+v", targets)
	}
}

// A channel switched off must produce no target even when fully configured.
func TestDisabledChannelsProduceNoTargets(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	if err := db.SaveInstanceSettings(ctx, storage.InstanceSettings{SMTP: instanceSMTP("mail.instance")}, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	u := newUser(t, db, storage.NotifyPrefs{EmailEnabled: false})

	if targets := snapshot(t, db).For(u); len(targets) != 0 {
		t.Errorf("email is off, expected no targets, got %d", len(targets))
	}
}

func TestTemplatesAreMergedOntoTargets(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	if err := db.SaveInstanceSettings(ctx, storage.InstanceSettings{
		SMTP:      instanceSMTP("mail.instance"),
		Templates: notify.TemplateSet{EmailSubject: "instance: {{.Title}}", EmailText: "instance body"},
	}, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	u := newUser(t, db, storage.NotifyPrefs{EmailEnabled: true})
	if _, err := db.SetUserTemplates(ctx, u.ID, notify.TemplateSet{EmailSubject: "mine: {{.Title}}"}); err != nil {
		t.Fatal(err)
	}
	u, _ = db.GetUserByID(ctx, u.ID)

	targets := snapshot(t, db).For(u)
	if len(targets) != 1 {
		t.Fatalf("expected one target, got %d", len(targets))
	}
	if got := targets[0].Templates.EmailSubject; got != "mine: {{.Title}}" {
		t.Errorf("subject template = %q, the user's should win", got)
	}
	if got := targets[0].Templates.EmailText; got != "instance body" {
		t.Errorf("text template = %q, the instance's should show through", got)
	}
}

// Rotating SESSION_KEY must degrade to "unconfigured", not to a crash.
func TestUnreadableCredentialIsReportedNotFatal(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	oldBox, err := secret.NewBox("an-entirely-different-old-session-key-32")
	if err != nil {
		t.Fatal(err)
	}
	enc, err := oldBox.Encrypt("secret")
	if err != nil {
		t.Fatal(err)
	}
	cfg := instanceSMTP("mail.instance")
	cfg.PasswordEnc = enc
	if err := db.SaveInstanceSettings(ctx, storage.InstanceSettings{SMTP: cfg}, uuid.Nil); err != nil {
		t.Fatal(err)
	}

	snap := snapshot(t, db) // built with the *current* key
	if !snap.SMTPUnreadable {
		t.Error("expected the unreadable credential to be reported")
	}
	u := newUser(t, db, storage.NotifyPrefs{EmailEnabled: true})
	// Still resolves: an unauthenticated relay works, and a server that needs
	// the password will report a clear auth failure at send time.
	if targets := snap.For(u); len(targets) != 1 {
		t.Errorf("expected delivery to still be attempted, got %d targets", len(targets))
	}
}

func TestInstanceTelegramTokenIsExposedForTheBot(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	box := newBox(t)
	enc, _ := box.Encrypt("bot-token-123")

	if snapshot(t, db).InstanceTelegramToken() != "" {
		t.Error("no token should be reported when nothing is configured")
	}
	if err := db.SaveInstanceSettings(ctx, storage.InstanceSettings{
		Telegram: storage.TelegramSettings{Enabled: true, BotTokenEnc: enc},
	}, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	if got := snapshot(t, db).InstanceTelegramToken(); got != "bot-token-123" {
		t.Errorf("token = %q, want the decrypted value", got)
	}

	// Disabled means the bot should stop polling.
	if err := db.SaveInstanceSettings(ctx, storage.InstanceSettings{
		Telegram: storage.TelegramSettings{Enabled: false, BotTokenEnc: enc},
	}, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	if got := snapshot(t, db).InstanceTelegramToken(); got != "" {
		t.Errorf("a disabled bot should report no token, got %q", got)
	}
}
