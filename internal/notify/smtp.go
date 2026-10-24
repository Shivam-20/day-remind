package notify

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// SMTPSender delivers notifications over SMTP.
type SMTPSender struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	// Mode is "starttls", "implicit" (SMTPS) or "none" for a local catcher.
	Mode string
	// Timeout bounds the whole conversation with the server.
	Timeout time.Duration
}

func (s *SMTPSender) Name() string { return ChannelEmail }

func (s *SMTPSender) timeout() time.Duration {
	if s.Timeout <= 0 {
		return 20 * time.Second
	}
	return s.Timeout
}

func (s *SMTPSender) Send(ctx context.Context, dest string, m Message, r Rendered) error {
	if dest == "" {
		return fmt.Errorf("no email address")
	}
	// A header injected through a display name or subject could forge extra
	// headers, so refuse anything carrying a line break.
	if strings.ContainsAny(dest, "\r\n") {
		return fmt.Errorf("invalid email address")
	}

	addr := net.JoinHostPort(s.Host, fmt.Sprint(s.Port))
	dialer := &net.Dialer{Timeout: s.timeout()}

	var conn net.Conn
	var err error
	if s.Mode == "implicit" {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: s.Host})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(s.timeout()))

	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return fmt.Errorf("smtp handshake: %w", err)
	}
	defer c.Close()

	if s.Mode == "starttls" {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return fmt.Errorf("server does not advertise STARTTLS (set SMTP_TLS=none for a local catcher)")
		}
		if err := c.StartTLS(&tls.Config{ServerName: s.Host}); err != nil {
			return fmt.Errorf("starttls: %w", err)
		}
	}

	if s.Username != "" {
		// PlainAuth refuses to run unencrypted, which is the behaviour we want
		// everywhere except an explicitly plaintext local catcher.
		auth := smtp.PlainAuth("", s.Username, s.Password, s.Host)
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	if err := c.Mail(s.From); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	if err := c.Rcpt(dest); err != nil {
		return fmt.Errorf("rcpt to: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	if _, err := w.Write(s.buildMessage(dest, r)); err != nil {
		return fmt.Errorf("write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close body: %w", err)
	}
	return c.Quit()
}

// sanitizeHeader removes anything that could terminate a header line. This is
// not optional: mime.QEncoding.Encode passes pure-ASCII text through unchanged,
// so a title containing CRLF would otherwise inject arbitrary headers.
func sanitizeHeader(v string) string {
	return strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' {
			return ' '
		}
		return r
	}, v)
}

// buildMessage assembles a multipart/alternative message: a plain-text part for
// clients that will not render HTML, and the styled HTML part. The text part
// comes first, as RFC 2046 asks for least-rich-first ordering.
//
// Note there is no dot-stuffing here: the writer returned by smtp.Client.Data
// is a textproto.DotWriter, which already escapes leading dots and normalises
// line endings. Doing it here as well would double-escape.
func (s *SMTPSender) buildMessage(dest string, r Rendered) []byte {
	boundary := newBoundary()

	var b strings.Builder
	b.WriteString("From: " + sanitizeHeader(s.From) + "\r\n")
	b.WriteString("To: " + sanitizeHeader(dest) + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", sanitizeHeader(r.Subject)) + "\r\n")
	b.WriteString("Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Auto-Submitted: auto-generated\r\n")

	html := strings.TrimSpace(r.HTML)
	if html == "" {
		// No HTML part to offer, so send a simple text message.
		b.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
		writeBody(&b, r.Text)
		return []byte(b.String())
	}

	b.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n\r\n")
	b.WriteString("This is a message in MIME format.\r\n")

	b.WriteString("\r\n--" + boundary + "\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
	writeBody(&b, r.Text)

	b.WriteString("\r\n--" + boundary + "\r\n")
	b.WriteString("Content-Type: text/html; charset=utf-8\r\n\r\n")
	writeBody(&b, html)

	b.WriteString("\r\n--" + boundary + "--\r\n")
	return []byte(b.String())
}

func writeBody(b *strings.Builder, body string) {
	for _, line := range strings.Split(body, "\n") {
		b.WriteString(strings.TrimRight(line, "\r") + "\r\n")
	}
}

// newBoundary returns a delimiter that cannot occur in the parts it separates.
func newBoundary() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		// Only reached if the system entropy source fails; a fixed value still
		// produces a well-formed message.
		return "dayremind-boundary-fallback"
	}
	return "dayremind-" + hex.EncodeToString(buf)
}
