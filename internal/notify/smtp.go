package notify

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// SMTPTLSMode values: "starttls" (default), "ssl" (implicit TLS, usually port 465), "none".
const (
	SMTPTLSStartTLS = "starttls"
	SMTPTLSSSL      = "ssl"
	SMTPTLSNone     = "none"
)

// NormalizeSMTPTLSMode validates an admin-supplied TLS mode, defaulting to starttls.
func NormalizeSMTPTLSMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case SMTPTLSSSL, "tls", "implicit":
		return SMTPTLSSSL
	case SMTPTLSNone, "off", "plain":
		return SMTPTLSNone
	default:
		return SMTPTLSStartTLS
	}
}

// deliverSMTP sends one HTML email over SMTP. Credentials arrive from the
// delivery settings store or env — never from literals in source.
func deliverSMTP(ctx context.Context, host string, port int, username, password, tlsMode, from, to, subject, htmlBody string) error {
	addr := fmt.Sprintf("%s:%d", host, port)
	d := net.Dialer{Timeout: 15 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("smtp: dial %s: %w", addr, err)
	}
	mode := NormalizeSMTPTLSMode(tlsMode)
	// Port 465 without an explicit mode is traditionally implicit TLS.
	if strings.TrimSpace(tlsMode) == "" && port == 465 {
		mode = SMTPTLSSSL
	}
	if mode == SMTPTLSSSL {
		conn = tls.Client(conn, &tls.Config{ServerName: host})
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("smtp: %w", err)
	}
	defer client.Close()

	if mode == SMTPTLSStartTLS {
		if ok, _ := client.Extension("StartTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: host}); err != nil {
				return fmt.Errorf("smtp: starttls: %w", err)
			}
		}
	}
	if username != "" {
		if ok, _ := client.Extension("AUTH"); ok {
			auth := smtp.PlainAuth("", username, password, host)
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("smtp: auth: %w", err)
			}
		}
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp: mail from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp: rcpt: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp: data: %w", err)
	}
	if _, err := w.Write([]byte(buildSMTPMessage(from, to, subject, htmlBody))); err != nil {
		_ = w.Close()
		return fmt.Errorf("smtp: write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp: close body: %w", err)
	}
	return client.Quit()
}

func buildSMTPMessage(from, to, subject, htmlBody string) string {
	var b strings.Builder
	b.WriteString("From: " + sanitizeHeaderLine(from) + "\r\n")
	b.WriteString("To: " + sanitizeHeaderLine(to) + "\r\n")
	b.WriteString("Subject: " + sanitizeHeaderLine(subject) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(htmlBody)
	b.WriteString("\r\n")
	return b.String()
}

// sanitizeHeaderLine keeps a header value to a single CR/LF-free line so
// untrusted input can never inject additional headers.
func sanitizeHeaderLine(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
