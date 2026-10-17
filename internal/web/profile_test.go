package web

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestUpdateMe(t *testing.T) {
	h := newHarness(t, nil)
	c := h.user(t)

	status, body := h.do(t, c, http.MethodPatch, "/api/me", map[string]any{
		"display_name": "Renamed",
		"timezone":     "Asia/Kolkata",
	})
	if status != http.StatusOK {
		t.Fatalf("PATCH /api/me = %d %s", status, body)
	}
	got := decode(t, body)
	if got["display_name"] != "Renamed" || got["timezone"] != "Asia/Kolkata" {
		t.Fatalf("profile not applied: %v", body)
	}

	// An unknown timezone must be rejected, not silently stored.
	status, body = h.do(t, c, http.MethodPatch, "/api/me", map[string]any{"timezone": "Mars/Olympus"})
	if status != http.StatusBadRequest {
		t.Fatalf("bad timezone = %d %s, want 400", status, body)
	}
}

func TestChangePassword(t *testing.T) {
	h := newHarness(t, nil)
	email := "pw-" + strings.Split(time.Now().Format("150405.000000000"), ".")[0] + "@example.com"
	c := h.client(t)
	h.signup(t, c, email)

	// A second session that must die when the password changes.
	other := h.client(t)
	status, body := h.do(t, other, http.MethodPost, "/api/auth/login", map[string]any{
		"email": email, "password": "password123",
	})
	if status != http.StatusOK {
		t.Fatalf("second login: %d %s", status, body)
	}

	status, body = h.do(t, c, http.MethodPost, "/api/me/password", map[string]any{
		"current_password": "wrong", "new_password": "newpassword456",
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("wrong current = %d %s, want 401", status, body)
	}

	status, body = h.do(t, c, http.MethodPost, "/api/me/password", map[string]any{
		"current_password": "password123", "new_password": "short",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("short new = %d %s, want 400", status, body)
	}

	status, body = h.do(t, c, http.MethodPost, "/api/me/password", map[string]any{
		"current_password": "password123", "new_password": "newpassword456",
	})
	if status != http.StatusOK {
		t.Fatalf("change = %d %s", status, body)
	}

	// Old password no longer signs in; the new one does.
	status, _ = h.do(t, h.client(t), http.MethodPost, "/api/auth/login", map[string]any{
		"email": email, "password": "password123",
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("old password accepted: %d", status)
	}
	status, _ = h.do(t, h.client(t), http.MethodPost, "/api/auth/login", map[string]any{
		"email": email, "password": "newpassword456",
	})
	if status != http.StatusOK {
		t.Fatalf("new password rejected: %d", status)
	}

	// The other session was revoked: /api/me must now refuse it.
	req, _ := http.NewRequest(http.MethodGet, h.srv.URL+"/api/me", nil)
	res, err := other.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	io.Copy(io.Discard, res.Body)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("other session survived password change: %d", res.StatusCode)
	}
}

func TestExport(t *testing.T) {
	h := newHarness(t, nil)
	c := h.user(t)

	status, body := h.do(t, c, http.MethodPost, "/api/reminders", map[string]any{
		"title": "Exported yearly", "due_at": "2027-09-03T15:30:00Z",
		"timezone": "Asia/Kolkata", "priority": "normal", "recurrence": "yearly",
	})
	if status != http.StatusCreated {
		t.Fatalf("create = %d %s", status, body)
	}

	// JSON export
	res := h.get(t, c, "/api/export/json")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("export/json = %d", res.StatusCode)
	}
	if cd := res.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Fatalf("json export not a download: %q", cd)
	}
	raw, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(raw), "Exported yearly") || !strings.Contains(string(raw), `"yearly"`) {
		t.Fatalf("json export missing reminder: %s", raw)
	}

	// iCal export with an RRULE for the yearly recurrence
	res = h.get(t, c, "/api/export/ical")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("export/ical = %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "text/calendar") {
		t.Fatalf("ical content type: %q", ct)
	}
	raw, _ = io.ReadAll(res.Body)
	text := string(raw)
	for _, want := range []string{"BEGIN:VCALENDAR", "DTSTART:20270903T153000Z", "RRULE:FREQ=YEARLY", "SUMMARY:Exported yearly", "END:VCALENDAR"} {
		if !strings.Contains(text, want) {
			t.Fatalf("ical export missing %q in:\n%s", want, text)
		}
	}
}

// get performs a GET with query-free path and returns the raw response; the
// caller must close the body.
func (h *harness) get(t *testing.T, c *http.Client, path string) *http.Response {
	t.Helper()
	u, err := url.Parse(h.srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Do(&http.Request{Method: http.MethodGet, URL: u, Header: http.Header{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}
