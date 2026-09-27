package notify

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/shivam-20/day-remind/internal/config"
	"github.com/shivam-20/day-remind/internal/storage"
)

func TestBuildSMTPMessage(t *testing.T) {
	msg := buildSMTPMessage("reminders@example.test", "user@example.test", "Day Remind test", "<p>hi</p>")
	if !strings.Contains(msg, "From: reminders@example.test\r\n") {
		t.Errorf("missing From header: %q", msg)
	}
	if !strings.Contains(msg, "To: user@example.test\r\n") {
		t.Errorf("missing To header: %q", msg)
	}
	if !strings.Contains(msg, "Subject: Day Remind test\r\n") {
		t.Errorf("missing Subject header: %q", msg)
	}
	if !strings.Contains(msg, "Content-Type: text/html; charset=UTF-8\r\n") {
		t.Errorf("missing Content-Type header: %q", msg)
	}
	if !strings.HasSuffix(msg, "<p>hi</p>\r\n") {
		t.Errorf("body not last: %q", msg)
	}
}

func TestBuildSMTPMessageHeaderInjection(t *testing.T) {
	msg := buildSMTPMessage(
		"a@example.test",
		"b@example.test",
		"line1\r\nBcc: victim@example.test",
		"<p>ok</p>",
	)
	// The CRLF must be stripped so "Bcc:" never starts its own header line.
	if strings.Contains(msg, "\r\nBcc:") {
		t.Errorf("header injection not sanitized: %q", msg)
	}
	if !strings.Contains(msg, "Subject: line1 Bcc: victim@example.test\r\n") {
		t.Errorf("subject should be flattened to one line: %q", msg)
	}
}

func TestNormalizeSMTPTLSMode(t *testing.T) {
	cases := map[string]string{
		"":         SMTPTLSStartTLS,
		"starttls": SMTPTLSStartTLS,
		"SSL":      SMTPTLSSSL,
		"tls":      SMTPTLSSSL,
		"none":     SMTPTLSNone,
		"weird":    SMTPTLSStartTLS,
	}
	for in, want := range cases {
		if got := NormalizeSMTPTLSMode(in); got != want {
			t.Errorf("NormalizeSMTPTLSMode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEffectiveDeliverySMTPFromEnv(t *testing.T) {
	cfg := config.Config{
		SMTPHost:     "smtp.example.test",
		SMTPPort:     465,
		SMTPUsername: "mailer@example.test",
		EmailFrom:    "reminders@example.test",
	}
	d := EffectiveDelivery(cfg, storage.DeliverySettings{})

	if d.EmailProvider != EmailProviderSMTP {
		t.Errorf("provider = %q, want smtp when only SMTP env is set", d.EmailProvider)
	}
	if d.SMTPHost != "smtp.example.test" || d.SMTPPort != 465 {
		t.Errorf("smtp host/port = %q/%d", d.SMTPHost, d.SMTPPort)
	}
	if !EmailProviderReady(d) {
		t.Error("smtp provider should be ready with host + from")
	}
	if !ChannelAvailability(d)[storage.ChannelEmail] {
		t.Error("email should be available (auto-enabled from env)")
	}
}

func TestEffectiveDeliveryStoredSMTPAndProviderChoice(t *testing.T) {
	stored := storage.DeliverySettings{
		EmailEnabled:  true,
		EmailFrom:     "reminders@example.test",
		EmailProvider: EmailProviderSMTP,
		SMTPHost:      "smtp.stored.test",
		SMTPPort:      2525,
		UpdatedAt:     time.Now(),
	}
	// Non-secret fixture: provider selection only needs a non-empty value.
	// Real Resend keys are never embedded in source (see AGENTS.md / env config).
	envResendValue := "env-fixture-value"
	cfg := config.Config{ResendAPIKey: envResendValue, SMTPHost: "smtp.env.test"}
	d := EffectiveDelivery(cfg, stored)

	if d.EmailProvider != EmailProviderSMTP {
		t.Errorf("provider = %q, want stored smtp", d.EmailProvider)
	}
	if d.SMTPHost != "smtp.stored.test" {
		t.Errorf("smtp host = %q, want stored", d.SMTPHost)
	}
	if d.SMTPPort != 2525 {
		t.Errorf("smtp port = %d, want 2525", d.SMTPPort)
	}
}

func TestEffectiveDeliveryDefaultPort(t *testing.T) {
	d := EffectiveDelivery(config.Config{}, storage.DeliverySettings{SMTPHost: "smtp.example.test"})
	if d.SMTPPort != 587 {
		t.Errorf("default smtp port = %d, want 587", d.SMTPPort)
	}
}

func TestSendEmailRequiresProviderConfig(t *testing.T) {
	err := SendEmail(context.Background(), DeliveryConfig{EmailEnabled: true}, "to@example.test", "s", "<p>b</p>")
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("expected not-configured error, got %v", err)
	}
}
