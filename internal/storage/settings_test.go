package storage

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/notify"
	"github.com/shivam-20/day-remind/internal/secret"
	"go.mongodb.org/mongo-driver/bson"
)

func testBox(t *testing.T) *secret.Box {
	t.Helper()
	b, err := secret.NewBox("storage-settings-test-session-key-32ch")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestInstanceSettingsRoundTrip(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// A fresh install has nothing configured, and that is not an error.
	got, err := db.GetInstanceSettings(ctx)
	if err != nil {
		t.Fatalf("a missing document should not be an error: %v", err)
	}
	if got.SMTP.Complete() || got.Telegram.Complete() {
		t.Error("nothing should be configured yet")
	}

	enc, err := testBox(t).Encrypt("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	actor := uuid.New()
	want := InstanceSettings{
		SMTP: SMTPSettings{
			Enabled: true, Host: "smtp.example.com", Port: 465, Username: "postmaster",
			PasswordEnc: enc, From: "reminders@example.com", TLS: TLSImplicit,
		},
		Telegram:  TelegramSettings{Enabled: true, BotTokenEnc: enc},
		Templates: notify.TemplateSet{EmailSubject: "hi {{.Title}}"},
	}
	if err := db.SaveInstanceSettings(ctx, want, actor); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err = db.GetInstanceSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.SMTP.Host != want.SMTP.Host || got.SMTP.Port != 465 || got.SMTP.TLS != TLSImplicit {
		t.Errorf("smtp did not round trip: %+v", got.SMTP)
	}
	if got.Templates.EmailSubject != "hi {{.Title}}" {
		t.Errorf("templates did not round trip: %+v", got.Templates)
	}
	if got.UpdatedBy != actor {
		t.Errorf("updated_by = %v, want %v", got.UpdatedBy, actor)
	}
	if got.UpdatedAt.IsZero() {
		t.Error("updated_at was not stamped")
	}
}

// Environment variables bootstrap a fresh install and are ignored thereafter.
func TestSeedOnlyWritesOnce(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	first := InstanceSettings{SMTP: SMTPSettings{
		Enabled: true, Host: "from.env", Port: 587, From: "env@example.com", TLS: TLSStartTLS,
	}}
	seeded, err := db.SeedInstanceSettings(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	if !seeded {
		t.Fatal("the first seed should write")
	}

	// An admin then changes it in the panel.
	edited := first
	edited.SMTP.Host = "edited.in.panel"
	if err := db.SaveInstanceSettings(ctx, edited, uuid.New()); err != nil {
		t.Fatal(err)
	}

	// A restart must not undo that.
	seeded, err = db.SeedInstanceSettings(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	if seeded {
		t.Error("a second seed should not write")
	}
	got, err := db.GetInstanceSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.SMTP.Host != "edited.in.panel" {
		t.Errorf("host = %q, the admin's edit should survive a restart", got.SMTP.Host)
	}
}

// The central promise of this feature: credentials are not readable in the
// database. This inspects the raw document rather than the typed API.
func TestCredentialsAreNotStoredInPlaintext(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	box := testBox(t)

	const password = "sup3r-s3cret-smtp-pw"
	const token = "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11"
	pwEnc, err := box.Encrypt(password)
	if err != nil {
		t.Fatal(err)
	}
	tokEnc, err := box.Encrypt(token)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveInstanceSettings(ctx, InstanceSettings{
		SMTP: SMTPSettings{
			Enabled: true, Host: "smtp.example.com", Port: 587,
			Username: "postmaster", PasswordEnc: pwEnc,
			From: "reminders@example.com", TLS: TLSStartTLS,
		},
		Telegram: TelegramSettings{Enabled: true, BotTokenEnc: tokEnc},
	}, uuid.New()); err != nil {
		t.Fatal(err)
	}

	var raw bson.M
	if err := db.collection("settings").FindOne(ctx, bson.M{"_id": instanceSettingsID}).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	dump, err := bson.MarshalExtJSON(raw, false, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{password, token} {
		if strings.Contains(string(dump), leak) {
			t.Errorf("a credential is stored in plaintext: %s", dump)
		}
	}
	// And it must still be usable.
	got, err := db.GetInstanceSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if back, err := box.Decrypt(got.SMTP.PasswordEnc); err != nil || back != password {
		t.Errorf("stored password did not decrypt back: %q, %v", back, err)
	}
}

func TestUserDeliveryCredentialsAreEncryptedAtRest(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	box := testBox(t)

	user := newTestUser(t, db, ctx)
	defer func() { _ = db.DeleteUser(ctx, user.ID) }()

	const password = "my-own-mail-password"
	enc, err := box.Encrypt(password)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SetUserDelivery(ctx, user.ID, UserDelivery{
		SMTP: SMTPSettings{
			Enabled: true, Host: "mail.mine", Port: 587,
			PasswordEnc: enc, From: "me@example.com", TLS: TLSStartTLS,
		},
	}); err != nil {
		t.Fatal(err)
	}

	var raw bson.M
	if err := db.collection("users").FindOne(ctx, bson.M{"_id": user.ID}).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	dump, _ := bson.MarshalExtJSON(raw, false, false)
	if strings.Contains(string(dump), password) {
		t.Errorf("the user's mail password is stored in plaintext: %s", dump)
	}
}

func TestSMTPSettingsValidation(t *testing.T) {
	// A disabled server is never validated: half-filled is fine while off.
	if err := (SMTPSettings{Enabled: false, Host: ""}).Validate(); err != nil {
		t.Errorf("a disabled server should not be validated: %v", err)
	}

	bad := []SMTPSettings{
		{Enabled: true, From: "a@example.com"},                                     // no host
		{Enabled: true, Host: "smtp.example.com"},                                  // no from
		{Enabled: true, Host: "smtp.example.com", From: "not-an-email"},            // bad from
		{Enabled: true, Host: "smtp.example.com", From: "a@example.com", Port: -1}, // bad port
		{Enabled: true, Host: "smtp.example.com", From: "a@example.com", TLS: "ssl"},
	}
	for i, c := range bad {
		if err := c.Validate(); !errors.Is(err, ErrInvalid) {
			t.Errorf("case %d should be rejected with ErrInvalid, got %v", i, err)
		}
	}
	good := SMTPSettings{Enabled: true, Host: "smtp.example.com", From: "a@example.com"}
	if err := good.Validate(); err != nil {
		t.Errorf("a valid server was rejected: %v", err)
	}
}

func TestSMTPNormalizeAppliesDefaults(t *testing.T) {
	got := SMTPSettings{Host: "  smtp.example.com  ", From: " a@example.com "}.Normalize()
	if got.Port != 587 {
		t.Errorf("port = %d, want the 587 default", got.Port)
	}
	if got.TLS != TLSStartTLS {
		t.Errorf("tls = %q, want the starttls default", got.TLS)
	}
	if got.Host != "smtp.example.com" || got.From != "a@example.com" {
		t.Errorf("whitespace was not trimmed: %+v", got)
	}
}

// A template that cannot parse must be refused at save time, not discovered
// when a reminder fails to render.
func TestSaveRejectsABrokenTemplate(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	err := db.SaveInstanceSettings(ctx, InstanceSettings{
		Templates: notify.TemplateSet{EmailSubject: "{{.Title"},
	}, uuid.New())
	if !errors.Is(err, ErrInvalid) {
		t.Errorf("expected ErrInvalid for a broken template, got %v", err)
	}
}

func TestSetUserTemplatesRejectsBrokenInput(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	user := newTestUser(t, db, ctx)
	defer func() { _ = db.DeleteUser(ctx, user.ID) }()

	if _, err := db.SetUserTemplates(ctx, user.ID, notify.TemplateSet{TelegramText: "{{nope}}"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("expected ErrInvalid, got %v", err)
	}
	if _, err := db.SetUserTemplates(ctx, user.ID, notify.TemplateSet{EmailSubject: "ok {{.Title}}"}); err != nil {
		t.Errorf("a valid template was rejected: %v", err)
	}
}
