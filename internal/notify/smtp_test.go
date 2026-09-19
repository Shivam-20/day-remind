package notify

import (
	"bufio"
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSMTP is a minimal SMTP server: enough of the protocol for a client to
// complete one transaction, so the sender is exercised for real.
type fakeSMTP struct {
	addr string
	mu   sync.Mutex
	data []string
	rcpt []string
	from []string
}

func startFakeSMTP(t *testing.T) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeSMTP{addr: ln.Addr().String()}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	return s
}

func (s *fakeSMTP) serve(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	r := bufio.NewReader(conn)
	w := func(line string) { _, _ = conn.Write([]byte(line + "\r\n")) }

	w("220 fake.local ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			w("250-fake.local")
			w("250 SIZE 10240000")
		case strings.HasPrefix(cmd, "MAIL FROM"):
			s.mu.Lock()
			s.from = append(s.from, strings.TrimSpace(line))
			s.mu.Unlock()
			w("250 OK")
		case strings.HasPrefix(cmd, "RCPT TO"):
			s.mu.Lock()
			s.rcpt = append(s.rcpt, strings.TrimSpace(line))
			s.mu.Unlock()
			w("250 OK")
		case cmd == "DATA":
			w("354 End data with <CR><LF>.<CR><LF>")
			var body strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimRight(l, "\r\n") == "." {
					break
				}
				body.WriteString(l)
			}
			s.mu.Lock()
			s.data = append(s.data, body.String())
			s.mu.Unlock()
			w("250 OK")
		case cmd == "QUIT":
			w("221 Bye")
			return
		default:
			w("250 OK")
		}
	}
}

func (s *fakeSMTP) lastMessage(t *testing.T) string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.data) == 0 {
		t.Fatal("no message was delivered")
	}
	return s.data[len(s.data)-1]
}

func newSender(addr string) *SMTPSender {
	host, port, _ := net.SplitHostPort(addr)
	p := 0
	for _, c := range port {
		p = p*10 + int(c-'0')
	}
	return &SMTPSender{Host: host, Port: p, From: "reminders@example.com", Mode: "none", Timeout: 5 * time.Second}
}

func TestSMTPSendDeliversAWellFormedMessage(t *testing.T) {
	srv := startFakeSMTP(t)
	s := newSender(srv.addr)

	due := time.Date(2026, 12, 25, 8, 30, 0, 0, time.UTC)
	m := Message{Title: "Pay the bill", Description: "Electricity", DueAt: due, Timezone: "UTC", Priority: "high"}
	r, _ := Render(TemplateSet{}, m)
	err := s.Send(context.Background(), "user@example.com", m, r)
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	msg := srv.lastMessage(t)
	for _, want := range []string{
		"From: reminders@example.com",
		"To: user@example.com",
		"Subject: Reminder: Pay the bill",
		"Content-Type: text/plain; charset=utf-8",
		"Auto-Submitted: auto-generated",
		"Pay the bill",
		"Electricity",
		"Priority: high",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q\n---\n%s", want, msg)
		}
	}
	// Headers and body must be separated by a blank line.
	if !strings.Contains(msg, "\r\n\r\n") {
		t.Error("no header/body separator")
	}
}

func TestSMTPAdvanceNoticeSubject(t *testing.T) {
	srv := startFakeSMTP(t)
	s := newSender(srv.addr)

	m := Message{Title: "Standup", DueAt: time.Now().UTC().Add(time.Hour),
		Timezone: "UTC", Advance: true, LeadTime: time.Hour}
	r, _ := Render(TemplateSet{}, m)
	err := s.Send(context.Background(), "user@example.com", m, r)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	msg := srv.lastMessage(t)
	if !strings.Contains(msg, "Subject: Reminder in 1 hour: Standup") {
		t.Errorf("unexpected advance subject:\n%s", msg)
	}
	if !strings.Contains(msg, "due in 1 hour") {
		t.Errorf("body should state the lead time:\n%s", msg)
	}
}

// A title carrying CRLF must not be able to inject extra headers.
func TestSMTPHeaderInjectionIsNeutralised(t *testing.T) {
	srv := startFakeSMTP(t)
	s := newSender(srv.addr)

	m := Message{
		Title:    "Hi\r\nBcc: attacker@evil.example\r\nX-Injected: yes",
		DueAt:    time.Now().UTC(),
		Timezone: "UTC",
	}
	r, _ := Render(TemplateSet{}, m)
	err := s.Send(context.Background(), "user@example.com", m, r)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	msg := srv.lastMessage(t)
	headers, _, _ := strings.Cut(msg, "\r\n\r\n")

	// The payload may legitimately survive as literal text inside the Subject
	// value; what must not happen is a new header line being created.
	for _, line := range strings.Split(headers, "\r\n") {
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "bcc:") || strings.HasPrefix(lower, "x-injected:") {
			t.Errorf("injected header line %q in:\n%s", line, headers)
		}
	}
	const wantHeaders = 7 // From, To, Subject, Date, MIME-Version, Auto-Submitted, Content-Type
	if got := len(strings.Split(strings.TrimRight(headers, "\r\n"), "\r\n")); got != wantHeaders {
		t.Errorf("expected exactly %d header lines, got %d:\n%s", wantHeaders, got, headers)
	}
}

func TestSMTPRejectsAddressWithNewline(t *testing.T) {
	srv := startFakeSMTP(t)
	s := newSender(srv.addr)
	m := Message{Title: "x", DueAt: time.Now().UTC(), Timezone: "UTC"}
	r, _ := Render(TemplateSet{}, m)
	err := s.Send(context.Background(), "user@example.com\r\nRCPT TO:<attacker@evil.example>", m, r)
	if err == nil {
		t.Fatal("an address containing CRLF must be rejected")
	}
}

// A body line of "." must not terminate the DATA block early.
func TestSMTPDotStuffing(t *testing.T) {
	srv := startFakeSMTP(t)
	s := newSender(srv.addr)

	m := Message{Title: "Dots", Description: "line one\n.\nline two", DueAt: time.Now().UTC(), Timezone: "UTC"}
	r, _ := Render(TemplateSet{}, m)
	err := s.Send(context.Background(), "user@example.com", m, r)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	msg := srv.lastMessage(t)
	if !strings.Contains(msg, "line two") {
		t.Errorf("body was truncated at the lone dot:\n%s", msg)
	}
	// smtp.Client.Data uses a textproto.DotWriter, which escapes the leading
	// dot exactly once. Anything more would arrive as ".." after the receiving
	// server unstuffs it.
	if !strings.Contains(msg, "\r\n..\r\n") {
		t.Errorf("lone dot was not stuffed exactly once:\n%q", msg)
	}
	if strings.Contains(msg, "\r\n...\r\n") {
		t.Errorf("lone dot was double-stuffed:\n%q", msg)
	}
}

func TestSMTPReportsDialFailure(t *testing.T) {
	s := &SMTPSender{Host: "127.0.0.1", Port: 1, From: "a@example.com", Mode: "none", Timeout: time.Second}
	m := Message{Title: "x", DueAt: time.Now(), Timezone: "UTC"}
	r, _ := Render(TemplateSet{}, m)
	err := s.Send(context.Background(), "user@example.com", m, r)
	if err == nil {
		t.Fatal("expected a dial error against a closed port")
	}
	if !strings.Contains(err.Error(), "dial") {
		t.Errorf("error should name the failing step, got %v", err)
	}
}

// A multipart message must offer both alternatives, in least-rich-first order.
func TestSMTPSendsMultipartAlternative(t *testing.T) {
	srv := startFakeSMTP(t)
	s := newSender(srv.addr)

	m := Message{Title: "Pay the bill", Description: "Electricity",
		DueAt: time.Date(2026, 12, 25, 3, 30, 0, 0, time.UTC), Timezone: "Asia/Kolkata"}
	r, _ := Render(TemplateSet{}, m)
	if err := s.Send(context.Background(), "user@example.com", m, r); err != nil {
		t.Fatalf("send: %v", err)
	}

	msg := srv.lastMessage(t)
	if !strings.Contains(msg, "Content-Type: multipart/alternative; boundary=") {
		t.Fatalf("not multipart:\n%s", msg)
	}
	textAt := strings.Index(msg, "Content-Type: text/plain")
	htmlAt := strings.Index(msg, "Content-Type: text/html")
	if textAt < 0 || htmlAt < 0 {
		t.Fatalf("expected both parts:\n%s", msg)
	}
	if textAt > htmlAt {
		t.Error("the plain-text part must come before the HTML part")
	}
	if !strings.Contains(msg, "<h1") {
		t.Error("HTML part is missing its markup")
	}
	// The boundary must be closed, or clients will treat the mail as truncated.
	head, _, _ := strings.Cut(msg, "\r\n")
	_ = head
	bIdx := strings.Index(msg, "boundary=\"") + len("boundary=\"")
	boundary := msg[bIdx : bIdx+strings.Index(msg[bIdx:], "\"")]
	if !strings.Contains(msg, "--"+boundary+"--") {
		t.Error("closing boundary delimiter is missing")
	}
	if strings.Count(msg, "--"+boundary) < 3 {
		t.Error("expected two part delimiters and one closing delimiter")
	}
}

// 03:30 UTC is 09:00 in Kolkata: the reminder's own zone, not the server's.
func TestMessageRendersDueTimeInItsOwnZone(t *testing.T) {
	due := time.Date(2026, 12, 25, 3, 30, 0, 0, time.UTC)
	r, _ := Render(TemplateSet{}, Message{Title: "x", DueAt: due, Timezone: "Asia/Kolkata"})
	if !strings.Contains(r.Text, "09:00") {
		t.Errorf("due time should render in the reminder's zone:\n%s", r.Text)
	}
	bad, _ := Render(TemplateSet{}, Message{Title: "x", DueAt: due, Timezone: "Not/AZone"})
	if !strings.Contains(bad.Text, "UTC") {
		t.Errorf("unknown zone should fall back to UTC:\n%s", bad.Text)
	}
}
