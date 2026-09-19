package config

import "testing"

func TestParseAdminEmails(t *testing.T) {
	cfg := Config{AdminEmails: parseAdminEmails(" Boss@Example.COM , second@example.com ,, ")}

	for _, email := range []string{"boss@example.com", "BOSS@EXAMPLE.COM", " boss@example.com ", "second@example.com"} {
		if !cfg.IsAdminEmail(email) {
			t.Errorf("IsAdminEmail(%q) = false, want true", email)
		}
	}
	for _, email := range []string{"", "other@example.com", "boss@example.org"} {
		if cfg.IsAdminEmail(email) {
			t.Errorf("IsAdminEmail(%q) = true, want false", email)
		}
	}
	if len(cfg.AdminEmails) != 2 {
		t.Errorf("expected 2 entries after dropping blanks, got %d", len(cfg.AdminEmails))
	}
}

func TestIsAdminEmailWithNoneConfigured(t *testing.T) {
	cfg := Config{AdminEmails: parseAdminEmails("")}
	if len(cfg.AdminEmails) != 0 {
		t.Errorf("expected no admins, got %d", len(cfg.AdminEmails))
	}
	if cfg.IsAdminEmail("anyone@example.com") {
		t.Error("nobody should be admin when ADMIN_EMAILS is unset")
	}
}

func TestLoadSMTP(t *testing.T) {
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_FROM", "reminders@example.com")
	t.Setenv("SMTP_PORT", "")
	t.Setenv("SMTP_TLS", "")

	c, err := loadSMTP()
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != 587 {
		t.Errorf("default port = %d, want 587", c.Port)
	}
	if c.TLS != "starttls" {
		t.Errorf("default TLS = %q, want starttls", c.TLS)
	}
	if !c.Enabled() {
		t.Error("host and from are set, so email should be enabled")
	}
}

func TestSMTPDisabledWhenUnset(t *testing.T) {
	t.Setenv("SMTP_HOST", "")
	t.Setenv("SMTP_FROM", "")
	c, err := loadSMTP()
	if err != nil {
		t.Fatal(err)
	}
	if c.Enabled() {
		t.Error("email should be disabled with no host")
	}
}

// A half-configured mailer would fail at send time; fail at startup instead.
func TestSMTPRejectsIncompleteConfig(t *testing.T) {
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_FROM", "")
	if _, err := loadSMTP(); err == nil {
		t.Error("SMTP_HOST without SMTP_FROM should be rejected")
	}
}

func TestSMTPRejectsBadPortAndMode(t *testing.T) {
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_FROM", "a@example.com")
	t.Setenv("SMTP_TLS", "")

	for _, port := range []string{"0", "70000", "-1", "abc"} {
		t.Setenv("SMTP_PORT", port)
		if _, err := loadSMTP(); err == nil {
			t.Errorf("SMTP_PORT=%q should be rejected", port)
		}
	}
	t.Setenv("SMTP_PORT", "587")
	for _, mode := range []string{"ssl", "tls", "yes"} {
		t.Setenv("SMTP_TLS", mode)
		if _, err := loadSMTP(); err == nil {
			t.Errorf("SMTP_TLS=%q should be rejected", mode)
		}
	}
	for _, mode := range []string{"starttls", "implicit", "none", "STARTTLS", " none "} {
		t.Setenv("SMTP_TLS", mode)
		if _, err := loadSMTP(); err != nil {
			t.Errorf("SMTP_TLS=%q should be accepted: %v", mode, err)
		}
	}
}
