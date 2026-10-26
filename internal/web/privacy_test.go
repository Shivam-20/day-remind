package web

import (
	"net/http"
	"strings"
	"testing"
)

// privacyHarness signs a user up and configures the privacy password, ready to
// create hidden reminders.
func privacyHarness(t *testing.T) (*harness, *http.Client) {
	t.Helper()
	h := newHarness(t, nil)
	c := h.user(t)
	status, body := h.do(t, c, http.MethodPost, "/api/me/privacy", map[string]any{
		"new_password": "privacypass1",
	})
	if status != http.StatusOK {
		t.Fatalf("set privacy password: %d %s", status, body)
	}
	return h, c
}

// createHiddenReminder creates a hidden reminder and returns the response body.
// The create response is masked like every other read of a hidden reminder.
func createHiddenReminder(t *testing.T, h *harness, c *http.Client, title string) map[string]any {
	t.Helper()
	status, body := h.do(t, c, http.MethodPost, "/api/reminders", map[string]any{
		"title":       title,
		"description": "hush " + title,
		"due_at":      "2030-01-01T09:00:00Z",
		"hidden":      true,
	})
	if status != http.StatusCreated {
		t.Fatalf("create hidden reminder: %d %s", status, body)
	}
	return decode(t, body)
}

func reminderID(t *testing.T, payload map[string]any) string {
	t.Helper()
	id, ok := payload["id"].(string)
	if !ok || id == "" {
		t.Fatalf("no reminder id in response: %v", payload)
	}
	return id
}

func TestHideWithoutPrivacyPasswordIsRefused(t *testing.T) {
	h := newHarness(t, nil)
	c := h.user(t)
	status, body := h.do(t, c, http.MethodPost, "/api/reminders", map[string]any{
		"title": "no password yet", "due_at": "2030-01-01T09:00:00Z", "hidden": true,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("hidden without privacy password: %d %s", status, body)
	}
	// The inline bootstrap path must work: password offered, reminder hidden.
	status, body = h.do(t, c, http.MethodPost, "/api/reminders", map[string]any{
		"title": "with password", "due_at": "2030-01-01T09:00:00Z",
		"hidden": true, "privacy_password": "privacypass1",
	})
	if status != http.StatusCreated {
		t.Fatalf("hidden with inline privacy password: %d %s", status, body)
	}
	me := decode(t, mustGet(t, h, c, "/api/me"))
	if me["privacy_enabled"] != true {
		t.Fatalf("privacy_enabled = %v, want true", me["privacy_enabled"])
	}
}

func TestHiddenReminderMasksEveryRead(t *testing.T) {
	h, c := privacyHarness(t)
	created := createHiddenReminder(t, h, c, "Sekrit project")
	if created["title"] != hiddenTitlePlaceholder {
		t.Fatalf("create response title = %q, want masked", created["title"])
	}
	if created["description"] != "" {
		t.Fatalf("create response description = %q, want empty", created["description"])
	}
	if created["hidden"] != true {
		t.Fatalf("create response hidden = %v, want true", created["hidden"])
	}
	id := reminderID(t, created)

	list := decode(t, mustGet(t, h, c, "/api/reminders"))
	items := list["reminders"].([]any)
	if len(items) != 1 {
		t.Fatalf("list has %d reminders, want 1", len(items))
	}
	first := items[0].(map[string]any)
	if first["title"] != hiddenTitlePlaceholder || first["description"] != "" {
		t.Fatalf("list leak: %v / %q", first["title"], first["description"])
	}

	detail := decode(t, mustGet(t, h, c, "/api/reminders/"+id))
	if detail["title"] != hiddenTitlePlaceholder {
		t.Fatalf("detail leak: %q", detail["title"])
	}
	// Metadata stays readable so the locked card can show when it fires.
	if detail["due_at"] == "" || detail["status"] != "scheduled" {
		t.Fatalf("detail lost metadata: %v", detail)
	}
}

func TestUnlockUnhideFlow(t *testing.T) {
	h, c := privacyHarness(t)
	created := createHiddenReminder(t, h, c, "Sekrit project")
	id := reminderID(t, created)
	unlockPath := "/api/reminders/" + id + "/unlock"
	unhidePath := "/api/reminders/" + id + "/unhide"

	status, body := h.do(t, c, http.MethodPost, unlockPath, map[string]any{"password": "wrong-pass"})
	if status != http.StatusUnauthorized {
		t.Fatalf("unlock with wrong password: %d %s", status, body)
	}
	full := decode(t, mustGetBody(t, h, c, http.MethodPost, unlockPath, map[string]any{"password": "privacypass1"}))
	if full["title"] != "Sekrit project" || full["description"] != "hush Sekrit project" {
		t.Fatalf("unlock did not return real content: %v", full)
	}
	// Unlock must not change the stored state.
	if full["hidden"] != true {
		t.Fatalf("unlock changed hidden: %v", full["hidden"])
	}

	status, body = h.do(t, c, http.MethodPost, unhidePath, map[string]any{"password": "wrong-pass"})
	if status != http.StatusUnauthorized {
		t.Fatalf("unhide with wrong password: %d %s", status, body)
	}
	unhidden := decode(t, mustGetBody(t, h, c, http.MethodPost, unhidePath, map[string]any{"password": "privacypass1"}))
	if unhidden["hidden"] != false || unhidden["title"] != "Sekrit project" {
		t.Fatalf("unhide: %v", unhidden)
	}
	detail := decode(t, mustGet(t, h, c, "/api/reminders/"+id))
	if detail["title"] != "Sekrit project" {
		t.Fatalf("detail after unhide still masked: %q", detail["title"])
	}
}

func mustGetBody(t *testing.T, h *harness, c *http.Client, method, path string, body any) string {
	t.Helper()
	status, out := h.do(t, c, method, path, body)
	if status != http.StatusOK {
		t.Fatalf("%s %s: %d %s", method, path, status, out)
	}
	return out
}

func TestSearchSkipsHiddenReminders(t *testing.T) {
	h, c := privacyHarness(t)
	createHiddenReminder(t, h, c, "Sekrit project")
	status, body := h.do(t, c, http.MethodPost, "/api/reminders", map[string]any{
		"title": "Groceries", "due_at": "2030-01-02T09:00:00Z",
	})
	if status != http.StatusCreated {
		t.Fatalf("create visible reminder: %d %s", status, body)
	}

	// A hit would confirm the hidden content matches the query.
	found := decode(t, mustGet(t, h, c, "/api/reminders?q=Sekrit"))
	if items := found["reminders"].([]any); len(items) != 0 {
		t.Fatalf("search surfaced hidden reminder: %d hits", len(items))
	}
	visible := decode(t, mustGet(t, h, c, "/api/reminders?q=Groceries"))
	if items := visible["reminders"].([]any); len(items) != 1 {
		t.Fatalf("visible search: %d hits, want 1", len(items))
	}
	all := decode(t, mustGet(t, h, c, "/api/reminders"))
	if items := all["reminders"].([]any); len(items) != 2 {
		t.Fatalf("unfiltered list: %d reminders, want 2", len(items))
	}
}

func TestSubtasksLockedWhileHidden(t *testing.T) {
	h, c := privacyHarness(t)
	created := createHiddenReminder(t, h, c, "Sekrit project")
	id := reminderID(t, created)

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/reminders/" + id + "/subtasks"},
		{http.MethodPost, "/api/reminders/" + id + "/subtasks"},
		{http.MethodPost, "/api/reminders/" + id + "/subtasks/reorder"},
		{http.MethodGet, "/api/reminders/" + id + "/comments"},
		{http.MethodPost, "/api/reminders/" + id + "/comments"},
	} {
		status, body := h.do(t, c, route.method, route.path, map[string]any{"title": "x", "body": "x"})
		if status != http.StatusLocked {
			t.Fatalf("%s %s: %d %s, want %d", route.method, route.path, status, body, http.StatusLocked)
		}
	}

	// After unhiding, the attachments are reachable again.
	_, _ = h.do(t, c, http.MethodPost, "/api/reminders/"+id+"/unhide", map[string]any{"password": "privacypass1"})
	status, body := h.do(t, c, http.MethodGet, "/api/reminders/"+id+"/subtasks", nil)
	if status != http.StatusOK {
		t.Fatalf("subtasks after unhide: %d %s", status, body)
	}
}

func TestPatchCannotUnhideWithoutPassword(t *testing.T) {
	h, c := privacyHarness(t)
	created := createHiddenReminder(t, h, c, "Sekrit project")
	id := reminderID(t, created)

	status, body := h.do(t, c, http.MethodPatch, "/api/reminders/"+id, map[string]any{"hidden": false})
	if status != http.StatusForbidden {
		t.Fatalf("PATCH hidden=false: %d %s, want %d", status, body, http.StatusForbidden)
	}
	detail := decode(t, mustGet(t, h, c, "/api/reminders/"+id))
	if detail["hidden"] != true {
		t.Fatalf("reminder was unhidden via PATCH: %v", detail)
	}
}

func TestPrivacyPasswordLifecycle(t *testing.T) {
	h, c := privacyHarness(t)

	status, body := h.do(t, c, http.MethodPost, "/api/me/privacy", map[string]any{
		"current_password": "wrong-pass", "new_password": "privacypass2",
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("rotate with wrong current: %d %s", status, body)
	}
	status, body = h.do(t, c, http.MethodPost, "/api/me/privacy", map[string]any{
		"current_password": "privacypass1", "new_password": "privacypass2",
	})
	if status != http.StatusOK {
		t.Fatalf("rotate: %d %s", status, body)
	}
	// The old password must no longer unlock.
	created := createHiddenReminder(t, h, c, "Sekrit project")
	id := reminderID(t, created)
	status, _ = h.do(t, c, http.MethodPost, "/api/reminders/"+id+"/unlock", map[string]any{"password": "privacypass1"})
	if status != http.StatusUnauthorized {
		t.Fatalf("old password still unlocks: %d", status)
	}

	// Removing the password is refused while reminders are hidden.
	status, body = h.do(t, c, http.MethodDelete, "/api/me/privacy", map[string]any{"password": "privacypass2"})
	if status != http.StatusConflict {
		t.Fatalf("remove with hidden reminders: %d %s", status, body)
	}
	if !strings.Contains(body, "1 hidden reminder") {
		t.Fatalf("removal message should name the count: %s", body)
	}

	_, _ = h.do(t, c, http.MethodPost, "/api/reminders/"+id+"/unhide", map[string]any{"password": "privacypass2"})
	status, body = h.do(t, c, http.MethodDelete, "/api/me/privacy", map[string]any{"password": "privacypass2"})
	if status != http.StatusOK {
		t.Fatalf("remove: %d %s", status, body)
	}
	me := decode(t, mustGet(t, h, c, "/api/me"))
	if me["privacy_enabled"] != false {
		t.Fatalf("privacy_enabled = %v, want false", me["privacy_enabled"])
	}
}
