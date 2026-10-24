package bot

import (
	"strings"
	"testing"
	"time"
)

// Fixed reference point: Wednesday 17 June 2026, 14:30 in Kolkata.
func ref(t *testing.T) (time.Time, *time.Location) {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	return time.Date(2026, 6, 17, 14, 30, 0, 0, loc), loc
}

func TestParseWhen(t *testing.T) {
	now, loc := ref(t)
	if now.Weekday() != time.Wednesday {
		t.Fatalf("reference day should be Wednesday, got %s", now.Weekday())
	}

	cases := []struct {
		in   string
		want string // local time in Kolkata
	}{
		{"in 2h", "2026-06-17 16:30"},
		{"in 30m", "2026-06-17 15:00"},
		{"in 3 days", "2026-06-20 14:30"},
		{"in 1w", "2026-06-24 14:30"},

		{"today 18:00", "2026-06-17 18:00"},
		{"tomorrow", "2026-06-18 09:00"},
		{"tomorrow 9am", "2026-06-18 09:00"},
		{"tomorrow 9 am", "2026-06-18 09:00"},
		{"tomorrow 7pm", "2026-06-18 19:00"},
		{"tonight", "2026-06-17 20:00"},
		{"day after tomorrow 6am", "2026-06-19 06:00"},

		// Bare clock: ahead today stays today, behind rolls to tomorrow.
		{"18:30", "2026-06-17 18:30"},
		{"9am", "2026-06-18 09:00"},
		{"12am", "2026-06-18 00:00"},
		{"12pm", "2026-06-18 12:00"},

		// Weekdays. Wednesday is the reference day.
		{"friday", "2026-06-19 09:00"},
		{"fri 14:30", "2026-06-19 14:30"},
		{"monday", "2026-06-22 09:00"},
		{"wednesday", "2026-06-24 09:00"},   // today at 9am has passed
		{"next friday", "2026-06-26 09:00"}, // skips the coming Friday

		{"2026-12-25", "2026-12-25 09:00"},
		{"2026-12-25 08:00", "2026-12-25 08:00"},
		{"25 dec", "2026-12-25 09:00"},
		{"25 december 9am", "2026-12-25 09:00"},
		{"1 jan", "2027-01-01 09:00"}, // already past this year
	}

	for _, c := range cases {
		got, err := ParseWhen(c.in, now, loc)
		if err != nil {
			t.Errorf("ParseWhen(%q) error: %v", c.in, err)
			continue
		}
		if have := got.In(loc).Format("2006-01-02 15:04"); have != c.want {
			t.Errorf("ParseWhen(%q) = %s, want %s", c.in, have, c.want)
		}
	}
}

func TestParseWhenRejectsGarbage(t *testing.T) {
	now, loc := ref(t)
	for _, in := range []string{"", "   ", "sometime", "banana", "25:00", "2026-13-45", "99 smarch", "in 0h", "in -2h"} {
		if got, err := ParseWhen(in, now, loc); err == nil {
			t.Errorf("ParseWhen(%q) should fail, got %v", in, got)
		}
	}
}

func TestParseWhenIsTimezoneRelative(t *testing.T) {
	kolkata, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	la, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	// The same instant, expressed in two zones, is a different calendar day.
	instant := time.Date(2026, 6, 17, 2, 0, 0, 0, kolkata)

	a, err := ParseWhen("tomorrow 9am", instant.In(kolkata), kolkata)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ParseWhen("tomorrow 9am", instant.In(la), la)
	if err != nil {
		t.Fatal(err)
	}
	if a.Equal(b) {
		t.Error(`"tomorrow 9am" should resolve differently in Kolkata and Los Angeles`)
	}
	if got := a.In(kolkata).Format("2006-01-02 15:04"); got != "2026-06-18 09:00" {
		t.Errorf("Kolkata tomorrow 9am = %s", got)
	}
	// 02:00 in Kolkata is still the previous afternoon in LA.
	if got := b.In(la).Format("2006-01-02 15:04"); got != "2026-06-17 09:00" {
		t.Errorf("LA tomorrow 9am = %s", got)
	}
}

func TestParseDuration(t *testing.T) {
	cases := map[string]time.Duration{
		"30m": 30 * time.Minute, "45min": 45 * time.Minute, "10 minutes": 10 * time.Minute,
		"2h": 2 * time.Hour, "3hrs": 3 * time.Hour,
		"1d": 24 * time.Hour, "2days": 48 * time.Hour,
		"1w": 7 * 24 * time.Hour,
	}
	for in, want := range cases {
		got, err := ParseDuration(in)
		if err != nil {
			t.Errorf("ParseDuration(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseDuration(%q) = %v, want %v", in, got, want)
		}
	}
	for _, in := range []string{"", "0h", "-1h", "abc", "5", "h", "99y", "500d"} {
		if got, err := ParseDuration(in); err == nil {
			t.Errorf("ParseDuration(%q) should fail, got %v", in, got)
		}
	}
}

func TestEscapeMarkdownV2(t *testing.T) {
	// Telegram rejects the whole message if any special character is left
	// unescaped, so assert the real invariant: every special rune is preceded
	// by a backslash.
	const special = "_*[]()~`>#+-=|{}.!\\"
	got := escape("Pay bill (urgent!) [see notes] 50% *now* a-b.c")
	runes := []rune(got)
	for i, r := range runes {
		if !strings.ContainsRune(special, r) {
			continue
		}
		if r == '\\' && i+1 < len(runes) && strings.ContainsRune(special, runes[i+1]) {
			continue // this backslash is the escape for the next rune
		}
		if i == 0 || runes[i-1] != '\\' {
			t.Errorf("unescaped %q at %d in %q", r, i, got)
		}
	}
	if escape("plain text 50%") != "plain text 50%" {
		t.Errorf("non-special text should pass through unchanged, got %q", escape("plain text 50%"))
	}
}
