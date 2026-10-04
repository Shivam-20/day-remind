package webhook

import (
	"context"
	"crypto/hmac"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/reminder"
	"github.com/shivam-20/day-remind/internal/storage"
)

func TestValidateURL(t *testing.T) {
	cases := map[string]bool{
		"https://hooks.example.com/day-remind": true,
		"http://example.org:8443/x?y=1":        true,
		"https://93.184.216.34/hook":           true,
		"ftp://example.com/x":                  false,
		"example.com/x":                        false,
		"https://user:pw@example.com/x":        false,
		"http://localhost:8090/api":            false,
		"http://home:3000/auth":                false, // Docker service name
		"http://mongo:27017":                   false,
		"http://day-remind-app.local/x":        false,
		"http://metadata.google.internal/":     false,
		"http://127.0.0.1/x":                   false,
		"http://10.0.0.5/x":                    false,
		"http://172.18.0.4/x":                  false,
		"http://192.168.1.4/x":                 false,
		"http://169.254.169.254/latest":        false, // cloud metadata
		"http://100.65.65.49/x":                false, // Tailscale (CGNAT)
		"http://[::1]/x":                       false,
		"http://[fd00::1]/x":                   false,
		"http://[::ffff:127.0.0.1]/x":          false,
		"http://0.0.0.0/x":                     false,
	}
	for raw, want := range cases {
		_, err := ValidateURL(raw)
		if (err == nil) != want {
			t.Errorf("ValidateURL(%q) err=%v, want ok=%v", raw, err, want)
		}
	}
}

func TestIsBlockedIP(t *testing.T) {
	for _, s := range []string{"8.8.8.8", "1.1.1.1", "2606:4700::1111"} {
		if isBlockedIP(netip.MustParseAddr(s)) {
			t.Errorf("%s should be allowed", s)
		}
	}
	for _, s := range []string{"127.0.0.1", "10.1.2.3", "100.64.0.1", "198.18.0.1", "224.0.0.1", "fe80::1", "64:ff9b::a00:1"} {
		if !isBlockedIP(netip.MustParseAddr(s)) {
			t.Errorf("%s should be blocked", s)
		}
	}
}

func TestValidateEvents(t *testing.T) {
	got, err := ValidateEvents([]string{" Reminder.Fired ", "reminder.done", "reminder.fired"})
	if err != nil || len(got) != 2 || got[0] != EventReminderFired || got[1] != EventReminderDone {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err := ValidateEvents(nil); err == nil {
		t.Error("empty events accepted")
	}
	if _, err := ValidateEvents([]string{"reminder.deleted"}); err == nil {
		t.Error("unknown event accepted")
	}
}

// The production client must refuse to connect to private IPs even when the
// URL passed validation (e.g. DNS later resolving to 127.0.0.1).
func TestSafeClientBlocksPrivateDial(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("request reached a loopback server")
	}))
	defer srv.Close()
	d := &Dispatcher{client: newSafeClient(2*time.Second, false)}
	_, err := d.post(context.Background(), storage.Webhook{URL: srv.URL, Secret: "s"}, uuid.New(), EventPing, []byte("{}"))
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("err = %v, want ErrBlockedAddress", err)
	}
}

func TestPostSignsAndDoesNotFollowRedirects(t *testing.T) {
	const secret = "whsec_test_secret_value"
	var gotSig, gotEvent, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		if r.URL.Path == "/final" {
			t.Error("redirect was followed")
		}
		b, _ := io.ReadAll(r.Body)
		gotBody, gotSig, gotEvent = string(b), r.Header.Get(SignatureHeader), r.Header.Get(EventHeader)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	d := &Dispatcher{client: newSafeClient(2*time.Second, true)}
	hook := storage.Webhook{URL: srv.URL + "/hook", Secret: secret}

	code, err := d.post(context.Background(), hook, uuid.New(), EventReminderDone, []byte(`{"a":1}`))
	if err != nil || code != http.StatusNoContent {
		t.Fatalf("post: code=%d err=%v", code, err)
	}
	if gotEvent != EventReminderDone || gotBody != `{"a":1}` {
		t.Fatalf("event=%q body=%q", gotEvent, gotBody)
	}
	var ts int64
	var v1 string
	if _, err := fmt.Sscanf(strings.Replace(gotSig, ",v1=", " ", 1), "t=%d %s", &ts, &v1); err != nil {
		t.Fatalf("bad signature header %q: %v", gotSig, err)
	}
	if !hmac.Equal([]byte(v1), []byte(Sign(secret, ts, []byte(gotBody)))) {
		t.Fatal("signature does not verify")
	}

	hook.URL = srv.URL + "/redirect"
	if code, err := d.post(context.Background(), hook, uuid.New(), EventPing, []byte("{}")); err == nil || code != http.StatusFound {
		t.Fatalf("redirect: code=%d err=%v, want 302 error", code, err)
	}
}

func TestNilDispatcherIsNoop(t *testing.T) {
	var d *Dispatcher
	d.EmitDone(context.Background(), reminder.Reminder{})
	d.EmitFired(context.Background(), reminder.Reminder{})
}
