package notify

import "testing"

func TestTelegramCallbackDataRoundTrip(t *testing.T) {
	cases := []struct {
		action, id, arg string
	}{
		{TelegramCallbackDone, "9c1f-abc", ""},
		{TelegramCallbackSnooze, "9c1f-abc", "1h"},
		{TelegramCallbackPause, "9c1f-abc", ""},
	}
	for _, c := range cases {
		action, id, arg := ParseTelegramCallbackData(TelegramCallbackData(c.action, c.id, c.arg))
		if action != c.action || id != c.id || arg != c.arg {
			t.Fatalf("round trip = (%q,%q,%q), want (%q,%q,%q)", action, id, arg, c.action, c.id, c.arg)
		}
	}
}

func TestParseTelegramCallbackDataEmpty(t *testing.T) {
	action, id, arg := ParseTelegramCallbackData("")
	if action != "" || id != "" || arg != "" {
		t.Fatalf("expected empty parts, got (%q,%q,%q)", action, id, arg)
	}
}

func TestEscapeTelegramHTML(t *testing.T) {
	got := EscapeTelegramHTML(`<b>a & b</b>`)
	want := `&lt;b&gt;a &amp; b&lt;/b&gt;`
	if got != want {
		t.Fatalf("EscapeTelegramHTML = %q, want %q", got, want)
	}
}

func TestTelegramReminderKeyboard(t *testing.T) {
	withLink := TelegramReminderKeyboard("abc", "https://x/#/reminders?open=abc")
	if len(withLink.InlineKeyboard) != 2 {
		t.Fatalf("expected 2 rows with deep link, got %d", len(withLink.InlineKeyboard))
	}
	first := withLink.InlineKeyboard[0]
	if len(first) != 3 || first[0].CallbackData != "done:abc" || first[1].CallbackData != "snooze:abc:1h" {
		t.Fatalf("unexpected first row: %+v", first)
	}
	withoutLink := TelegramReminderKeyboard("abc", "")
	if len(withoutLink.InlineKeyboard) != 1 {
		t.Fatalf("expected 1 row without deep link, got %d", len(withoutLink.InlineKeyboard))
	}
}
