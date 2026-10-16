package notify

import (
	"strings"
	"testing"
	"time"
)

func dueMessage() Message {
	return Message{
		Title:       "Pay the bill",
		Description: "Account 12345",
		DueAt:       time.Date(2026, 12, 25, 3, 30, 0, 0, time.UTC),
		Timezone:    "Asia/Kolkata",
		Priority:    "high",
		DisplayName: "Sam",
	}
}

func TestDefaultTemplatesAlwaysParse(t *testing.T) {
	// The defaults are the last-resort fallback, so a mistake here would take
	// down every notification.
	if errs := Validate(DefaultTemplates()); len(errs) != 0 {
		t.Fatalf("built-in defaults do not validate: %v", errs)
	}
}

func TestRenderDefaults(t *testing.T) {
	out, problems := Render(TemplateSet{}, dueMessage())
	if len(problems) != 0 {
		t.Fatalf("unexpected problems: %v", problems)
	}
	if out.Subject != "Reminder: Pay the bill" {
		t.Errorf("subject = %q", out.Subject)
	}
	for _, want := range []string{"Pay the bill", "Account 12345", "09:00"} {
		if !strings.Contains(out.HTML, want) {
			t.Errorf("HTML missing %q", want)
		}
		if !strings.Contains(out.Text, want) {
			t.Errorf("text missing %q", want)
		}
	}
	if !strings.Contains(out.Text, "Priority: high") {
		t.Errorf("non-normal priority should appear in the text part:\n%s", out.Text)
	}
	if out.TelegramPlain {
		t.Error("default parse mode should be MarkdownV2")
	}
}

func TestAdvanceRendersLeadTime(t *testing.T) {
	m := dueMessage()
	m.Advance = true
	m.LeadTime = time.Hour
	out, problems := Render(TemplateSet{}, m)
	if len(problems) != 0 {
		t.Fatalf("problems: %v", problems)
	}
	if out.Subject != "Reminder in 1 hour: Pay the bill" {
		t.Errorf("subject = %q", out.Subject)
	}
	if !strings.Contains(out.Text, "due in 1 hour") {
		t.Errorf("text should state the lead time:\n%s", out.Text)
	}
}

func TestMergeIsFieldByField(t *testing.T) {
	instance := TemplateSet{EmailSubject: "instance subject", EmailText: "instance text"}
	user := TemplateSet{EmailSubject: "user subject"}

	got := Merge(DefaultTemplates(), instance, user)
	if got.EmailSubject != "user subject" {
		t.Errorf("user should win: %q", got.EmailSubject)
	}
	if got.EmailText != "instance text" {
		t.Errorf("instance should show through where the user set nothing: %q", got.EmailText)
	}
	if !strings.Contains(got.EmailHTML, "Day Remind") {
		t.Error("default should show through where neither set anything")
	}
	// Whitespace is not an override.
	if Merge(DefaultTemplates(), TemplateSet{EmailSubject: "   "}).EmailSubject != DefaultTemplates().EmailSubject {
		t.Error("a blank field must not override")
	}
}

func TestCustomTemplateIsUsed(t *testing.T) {
	out, problems := Render(TemplateSet{
		EmailSubject: "[{{.Priority}}] {{.Title}}",
		TelegramText: "{{.Title}} at {{.DueAt}}",
	}, dueMessage())
	if len(problems) != 0 {
		t.Fatalf("problems: %v", problems)
	}
	if out.Subject != "[high] Pay the bill" {
		t.Errorf("subject = %q", out.Subject)
	}
	if !strings.HasPrefix(out.Telegram, "Pay the bill at") {
		t.Errorf("telegram = %q", out.Telegram)
	}
}

// A cosmetic mistake must never stop a reminder being delivered.
func TestBrokenTemplateFallsBackToDefault(t *testing.T) {
	cases := map[string]TemplateSet{
		"unclosed action": {EmailSubject: "{{.Title"},
		"unknown field":   {EmailSubject: "{{.NoSuchField}}"},
		"bad function":    {EmailSubject: `{{nope .Title}}`},
	}
	for name, ts := range cases {
		out, problems := Render(ts, dueMessage())
		if len(problems) == 0 {
			t.Errorf("%s: expected the problem to be reported", name)
		}
		if out.Subject != "Reminder: Pay the bill" {
			t.Errorf("%s: expected the default subject, got %q", name, out.Subject)
		}
		if out.HTML == "" {
			t.Errorf("%s: the other fields should still render", name)
		}
	}
}

func TestValidateReportsTheFailingField(t *testing.T) {
	errs := Validate(TemplateSet{
		EmailSubject: "{{.Title}}",
		EmailHTML:    "{{.Title",
		TelegramText: "{{bogus}}",
	})
	if len(errs) != 2 {
		t.Fatalf("expected 2 field errors, got %d: %v", len(errs), errs)
	}
	fields := map[string]bool{}
	for _, e := range errs {
		fields[e.Field] = true
	}
	if !fields["email_html"] || !fields["telegram_text"] {
		t.Errorf("wrong fields reported: %v", fields)
	}
	if Validate(TemplateSet{}) != nil {
		t.Error("an empty set is valid: every field falls back")
	}
}

func TestValidateRejectsUnknownParseMode(t *testing.T) {
	if errs := Validate(TemplateSet{TelegramParseMode: "html"}); len(errs) != 1 {
		t.Errorf("expected parse mode to be rejected, got %v", errs)
	}
	for _, mode := range []string{"", ParsePlain, ParseMarkdownV2} {
		if errs := Validate(TemplateSet{TelegramParseMode: mode}); len(errs) != 0 {
			t.Errorf("mode %q should be valid: %v", mode, errs)
		}
	}
}

// Reminder content is user input and ends up inside an HTML document.
func TestReminderContentIsEscapedInHTML(t *testing.T) {
	m := dueMessage()
	m.Title = `<script>alert('xss')</script>`
	m.Description = `<img src=x onerror=alert(1)>`

	out, problems := Render(TemplateSet{}, m)
	if len(problems) != 0 {
		t.Fatalf("problems: %v", problems)
	}
	// The payload may survive as inert escaped text; what must not happen is a
	// real tag being introduced into the document.
	if strings.Contains(out.HTML, "<script") || strings.Contains(out.HTML, "<img") {
		t.Errorf("user content introduced a live tag:\n%s", out.HTML)
	}
	if !strings.Contains(out.HTML, "&lt;script&gt;") {
		t.Errorf("expected the title to appear escaped:\n%s", out.HTML)
	}
}

// Telegram rejects the whole message if a special character is unescaped, and
// the values come from user input, so the engine must escape them.
func TestTelegramValuesAreEscapedForMarkdown(t *testing.T) {
	m := dueMessage()
	m.Title = "Pay (urgent!) 50% now"

	out, _ := Render(TemplateSet{}, m)
	if strings.Contains(out.Telegram, "(urgent!)") {
		t.Errorf("special characters were not escaped:\n%s", out.Telegram)
	}
	if !strings.Contains(out.Telegram, `\(urgent\!\)`) {
		t.Errorf("expected escaped punctuation:\n%s", out.Telegram)
	}

	// Plain mode is for authors who would rather not think about escaping.
	plain, _ := Render(TemplateSet{TelegramParseMode: ParsePlain}, m)
	if !plain.TelegramPlain {
		t.Error("plain mode should be reported to the sender")
	}
	if !strings.Contains(plain.Telegram, "(urgent!)") {
		t.Errorf("plain mode should not escape:\n%s", plain.Telegram)
	}
}

func TestTextIsDerivedWhenTemplateIsBlank(t *testing.T) {
	// Only the HTML is customised; the text part must still be usable.
	out, problems := Render(TemplateSet{
		EmailHTML: `<html><body><style>p{color:red}</style><h1>{{.Title}}</h1><p>{{.Description}}</p></body></html>`,
		EmailText: "",
	}, dueMessage())
	if len(problems) != 0 {
		t.Fatalf("problems: %v", problems)
	}
	if out.Text == "" {
		t.Fatal("a plain-text alternative is required")
	}
	if strings.Contains(out.Text, "<h1>") || strings.Contains(out.Text, "color:red") {
		t.Errorf("derived text still contains markup:\n%s", out.Text)
	}
	if !strings.Contains(out.Text, "Pay the bill") {
		t.Errorf("derived text lost the content:\n%s", out.Text)
	}
}

// A template that repeats a value must not produce an unbounded body.
func TestOversizedOutputFallsBack(t *testing.T) {
	m := dueMessage()
	m.Description = strings.Repeat("x", 1024)
	huge := strings.Repeat("{{.Description}}", 512) // ~512KB, over the cap

	out, problems := Render(TemplateSet{EmailSubject: huge}, m)
	if len(problems) == 0 {
		t.Error("expected the size limit to be reported")
	}
	if out.Subject != "Reminder: Pay the bill" {
		t.Errorf("expected the default subject after the cap, got %d chars", len(out.Subject))
	}
}

func TestEmailCSSIsInjectedAndCannotBreakOut(t *testing.T) {
	html := `<html><head><title>n</title></head><body><h1>{{.Title}}</h1></body></html>`
	css := `h1{color:{{if .Advance}}red{{else}}blue{{end}}}</style><script>alert(1)</script>p{margin:0}`
	out, problems := Render(TemplateSet{EmailHTML: html, EmailCSS: css}, dueMessage())
	if len(problems) != 0 {
		t.Fatalf("problems: %v", problems)
	}
	if !strings.Contains(out.HTML, "<style>h1{color:blue}p{margin:0}</style></head>") {
		t.Errorf("CSS was not injected before </head>:\n%s", out.HTML)
	}
	if strings.Contains(strings.ToLower(out.HTML), "<script") || strings.Count(strings.ToLower(out.HTML), "</style>") != 1 {
		t.Errorf("stylesheet broke out of the style block:\n%s", out.HTML)
	}
	if strings.Contains(out.Text, "color:blue") {
		t.Errorf("plain text should not include the stylesheet:\n%s", out.Text)
	}

	bare, problems := Render(TemplateSet{
		EmailHTML: `<p>{{.Title}}</p>`,
		EmailCSS:  `p{color:green}`,
	}, dueMessage())
	if len(problems) != 0 {
		t.Fatalf("problems: %v", problems)
	}
	if !strings.HasPrefix(bare.HTML, "<style>p{color:green}</style><p>") {
		t.Errorf("CSS should be prepended when there is no head:\n%s", bare.HTML)
	}
}

func TestSampleMessageRendersBothKinds(t *testing.T) {
	for _, advance := range []bool{false, true} {
		out, problems := Render(TemplateSet{}, SampleMessage(advance))
		if len(problems) != 0 {
			t.Errorf("advance=%v: %v", advance, problems)
		}
		if out.Subject == "" || out.HTML == "" || out.Text == "" || out.Telegram == "" {
			t.Errorf("advance=%v: preview produced an empty field", advance)
		}
	}
}
