package notify

import (
	"errors"
	"fmt"
	htmltemplate "html/template"
	"io"
	"regexp"
	"strings"
	texttemplate "text/template"
	"time"
)

// TelegramParseMode values.
const (
	ParseMarkdownV2 = "markdownv2"
	ParsePlain      = "plain"
)

// maxRendered caps a rendered field. A template cannot execute arbitrary code,
// but it can repeat a long value, and an unbounded body would be pushed
// straight into an SMTP conversation or a Telegram request.
const maxRendered = 256 << 10

// TemplateSet is the editable wording for notifications. Every field is
// optional: an empty field falls through to the next source (user → instance →
// built-in default), so a user can override just the subject line.
//
// The bson tags let storage embed this directly.
type TemplateSet struct {
	EmailSubject      string `bson:"email_subject,omitempty"`
	EmailHTML         string `bson:"email_html,omitempty"`
	EmailCSS          string `bson:"email_css,omitempty"`
	EmailText         string `bson:"email_text,omitempty"`
	TelegramText      string `bson:"telegram_text,omitempty"`
	TelegramParseMode string `bson:"telegram_parse_mode,omitempty"`
}

// TemplateData is the variable set available to template authors. Everything is
// pre-formatted so templates stay free of function calls.
type TemplateData struct {
	Title       string
	Description string
	DueAt       string
	Timezone    string
	Priority    string
	DisplayName string
	LeadTime    string
	Kind        string // "due" or "advance"
	Advance     bool
}

// TemplateVariables documents the data set for the editor UI.
var TemplateVariables = []struct{ Name, Description string }{
	{".Title", "The reminder's title"},
	{".Description", "The reminder's description, may be empty"},
	{".DueAt", "Due date and time, in the reminder's own timezone"},
	{".Timezone", "The reminder's timezone, e.g. Asia/Kolkata"},
	{".Priority", "low, normal, high or urgent"},
	{".DisplayName", "The recipient's display name, may be empty"},
	{".LeadTime", "How far ahead an advance notice is, e.g. \"1 hour\""},
	{".Kind", "\"due\" or \"advance\""},
	{".Advance", "true for the advance notice, false for the due one"},
}

// DefaultTemplates is the built-in wording. It is also the final fallback when
// a custom template fails, so it must always parse.
func DefaultTemplates() TemplateSet {
	return TemplateSet{
		EmailSubject: `{{if .Advance}}Reminder in {{.LeadTime}}: {{.Title}}{{else}}Reminder: {{.Title}}{{end}}`,
		EmailHTML: `<!DOCTYPE html>
<html>
  <body style="margin:0;padding:24px;background:#f4f5f7;font-family:-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;">
    <table role="presentation" cellpadding="0" cellspacing="0" style="max-width:520px;margin:0 auto;background:#ffffff;border-radius:12px;border:1px solid #e3e5ea;">
      <tr>
        <td style="padding:20px 24px;border-bottom:1px solid #eceef2;">
          <span style="font-size:13px;font-weight:600;letter-spacing:.06em;text-transform:uppercase;color:#f5a623;">Day Remind</span>
        </td>
      </tr>
      <tr>
        <td style="padding:24px;">
          <p style="margin:0 0 16px;color:#6b758c;font-size:14px;">
            {{if .Advance}}Heads up — this is due in {{.LeadTime}}.{{else}}This is your reminder.{{end}}
          </p>
          <h1 style="margin:0 0 12px;font-size:20px;line-height:1.3;color:#171b26;">{{.Title}}</h1>
          {{if .Description}}<p style="margin:0 0 18px;color:#3c4354;font-size:15px;line-height:1.5;">{{.Description}}</p>{{end}}
          <table role="presentation" cellpadding="0" cellspacing="0" style="width:100%;background:#f7f8fa;border-radius:8px;">
            <tr>
              <td style="padding:12px 14px;font-size:14px;color:#3c4354;">
                <strong style="color:#171b26;">Due</strong><br>{{.DueAt}}
                {{if ne .Priority "normal"}}<br><br><strong style="color:#171b26;">Priority</strong><br>{{.Priority}}{{end}}
              </td>
            </tr>
          </table>
        </td>
      </tr>
      <tr>
        <td style="padding:14px 24px;border-top:1px solid #eceef2;color:#9aa2b5;font-size:12px;">
          You are receiving this because notifications are switched on in Day Remind.
        </td>
      </tr>
    </table>
  </body>
</html>`,
		EmailText: `{{if .Advance}}Heads up — this is due in {{.LeadTime}}.{{else}}This is your reminder.{{end}}

{{.Title}}
{{if .Description}}
{{.Description}}
{{end}}
Due: {{.DueAt}}
{{if ne .Priority "normal"}}Priority: {{.Priority}}
{{end}}`,
		TelegramText: `*{{if .Advance}}Reminder in {{.LeadTime}}: {{end}}{{.Title}}*
{{if .Description}}
{{.Description}}
{{end}}
🕑 {{.DueAt}}{{if ne .Priority "normal"}}
⚑ {{.Priority}}{{end}}`,
		TelegramParseMode: ParseMarkdownV2,
	}
}

// Merge layers template sets. Later arguments win, field by field, so a user
// who overrides only the subject keeps the instance body.
func Merge(sets ...TemplateSet) TemplateSet {
	out := TemplateSet{}
	for _, s := range sets {
		if v := strings.TrimSpace(s.EmailSubject); v != "" {
			out.EmailSubject = s.EmailSubject
		}
		if v := strings.TrimSpace(s.EmailHTML); v != "" {
			out.EmailHTML = s.EmailHTML
		}
		if v := strings.TrimSpace(s.EmailCSS); v != "" {
			out.EmailCSS = s.EmailCSS
		}
		if v := strings.TrimSpace(s.EmailText); v != "" {
			out.EmailText = s.EmailText
		}
		if v := strings.TrimSpace(s.TelegramText); v != "" {
			out.TelegramText = s.TelegramText
		}
		if v := strings.TrimSpace(s.TelegramParseMode); v != "" {
			out.TelegramParseMode = s.TelegramParseMode
		}
	}
	return out
}

// FieldError names the template field that failed, so the editor can point at
// the right box instead of showing one opaque message.
type FieldError struct {
	Field string
	Err   error
}

func (e FieldError) Error() string { return e.Field + ": " + e.Err.Error() }
func (e FieldError) Unwrap() error { return e.Err }

// Validate parses every non-empty field and checks the parse mode. It is called
// before saving so a broken template is rejected at the point someone can fix
// it, rather than discovered at 3am when a reminder fails to render.
func Validate(ts TemplateSet) []FieldError {
	var errs []FieldError
	check := func(field, body string, html bool) {
		if strings.TrimSpace(body) == "" {
			return
		}
		var err error
		if html {
			_, err = htmltemplate.New(field).Parse(body)
		} else {
			_, err = texttemplate.New(field).Parse(body)
		}
		if err != nil {
			errs = append(errs, FieldError{Field: field, Err: err})
		}
	}
	check("email_subject", ts.EmailSubject, false)
	check("email_html", ts.EmailHTML, true)
	check("email_css", ts.EmailCSS, false)
	check("email_text", ts.EmailText, false)
	check("telegram_text", ts.TelegramText, false)

	switch strings.TrimSpace(ts.TelegramParseMode) {
	case "", ParseMarkdownV2, ParsePlain:
	default:
		errs = append(errs, FieldError{
			Field: "telegram_parse_mode",
			Err:   fmt.Errorf("must be %q or %q", ParseMarkdownV2, ParsePlain),
		})
	}
	return errs
}

// Rendered is the output of a template set for one message.
type Rendered struct {
	Subject       string
	HTML          string
	Text          string
	Telegram      string
	TelegramPlain bool // true when Telegram should be sent with no parse_mode
}

// Data converts a Message into the template variable set.
func (m Message) Data() TemplateData {
	kind := "due"
	if m.Advance {
		kind = "advance"
	}
	lead := ""
	if m.LeadTime > 0 {
		lead = humanDuration(m.LeadTime)
	}
	priority := m.Priority
	if priority == "" {
		priority = "normal"
	}
	return TemplateData{
		Title:       m.Title,
		Description: m.Description,
		DueAt:       m.localDue(),
		Timezone:    m.Timezone,
		Priority:    priority,
		DisplayName: m.DisplayName,
		LeadTime:    lead,
		Kind:        kind,
		Advance:     m.Advance,
	}
}

// escapeData returns the data with every value escaped for MarkdownV2. Telegram
// rejects a message with an unescaped special character, and the values come
// from user-entered reminder text, so escaping cannot be left to the author.
func escapeData(d TemplateData) TemplateData {
	d.Title = escapeMarkdown(d.Title)
	d.Description = escapeMarkdown(d.Description)
	d.DueAt = escapeMarkdown(d.DueAt)
	d.Timezone = escapeMarkdown(d.Timezone)
	d.Priority = escapeMarkdown(d.Priority)
	d.DisplayName = escapeMarkdown(d.DisplayName)
	d.LeadTime = escapeMarkdown(d.LeadTime)
	return d
}

// Render produces every output for one message. A failure in a custom template
// is never fatal: the built-in default is used for that field and the problem
// is returned for logging, because a cosmetic mistake must not stop a reminder
// from being delivered.
func Render(ts TemplateSet, m Message) (Rendered, []error) {
	def := DefaultTemplates()
	effective := Merge(def, ts)
	data := m.Data()

	var problems []error
	render := func(field, body, fallback string, html bool, d TemplateData) string {
		out, err := execute(field, body, html, d)
		if err == nil {
			return out
		}
		problems = append(problems, FieldError{Field: field, Err: err})
		// The default is known-good; if even that fails, something is very
		// wrong and an empty string is better than a panic.
		out, err = execute(field+"(default)", fallback, html, d)
		if err != nil {
			problems = append(problems, FieldError{Field: field + " fallback", Err: err})
			return ""
		}
		return out
	}

	plain := strings.TrimSpace(effective.TelegramParseMode) == ParsePlain
	tgData := data
	if !plain {
		tgData = escapeData(data)
	}

	html := render("email_html", effective.EmailHTML, def.EmailHTML, true, data)
	css := ""
	if strings.TrimSpace(effective.EmailCSS) != "" {
		css = render("email_css", effective.EmailCSS, "", false, data)
	}
	out := Rendered{
		Subject:       strings.TrimSpace(render("email_subject", effective.EmailSubject, def.EmailSubject, false, data)),
		HTML:          injectCSS(html, css),
		Telegram:      strings.TrimSpace(render("telegram_text", effective.TelegramText, def.TelegramText, false, tgData)),
		TelegramPlain: plain,
	}
	out.Text = strings.TrimSpace(render("email_text", effective.EmailText, def.EmailText, false, data))
	if out.Text == "" {
		// An author who clears the text part still needs a plain alternative,
		// so derive one rather than sending an HTML-only message.
		out.Text = htmlToText(out.HTML)
	}
	return out, problems
}

var errTooLarge = errors.New("rendered output exceeded the size limit")

func execute(name, body string, html bool, data TemplateData) (string, error) {
	var sb strings.Builder
	w := &cappedWriter{w: &sb, remaining: maxRendered}
	if html {
		t, err := htmltemplate.New(name).Parse(body)
		if err != nil {
			return "", err
		}
		if err := t.Execute(w, data); err != nil {
			return "", err
		}
	} else {
		t, err := texttemplate.New(name).Parse(body)
		if err != nil {
			return "", err
		}
		if err := t.Execute(w, data); err != nil {
			return "", err
		}
	}
	return sb.String(), nil
}

// cappedWriter stops a template from producing an unbounded body.
type cappedWriter struct {
	w         io.Writer
	remaining int
}

func (c *cappedWriter) Write(p []byte) (int, error) {
	if len(p) > c.remaining {
		return 0, errTooLarge
	}
	c.remaining -= len(p)
	return c.w.Write(p)
}

// Go's regexp has no backreferences, so script and style need one pattern each.
var (
	reScript     = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</\s*script\s*>`)
	reScriptOpen = regexp.MustCompile(`(?i)<\s*/?\s*script\b[^>]*>`)
	reStyleClose = regexp.MustCompile(`(?i)</\s*style\s*>`)
	reStyle      = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</\s*style\s*>`)
	reBreak      = regexp.MustCompile(`(?i)<(br\s*/?|/p|/div|/tr|/h[1-6])\s*>`)
	reTag        = regexp.MustCompile(`(?s)<[^>]*>`)
	reBlank      = regexp.MustCompile(`\n{3,}`)
)

// injectCSS puts a rendered stylesheet into the HTML document. Email clients
// that honor a head style block pick it up; authors who prefer inline styles
// can still put them in the HTML itself.
func injectCSS(html, css string) string {
	css = sanitizeCSS(css)
	if strings.TrimSpace(css) == "" {
		return html
	}
	block := "<style>" + css + "</style>"
	if i := strings.Index(strings.ToLower(html), "</head>"); i >= 0 {
		return html[:i] + block + html[i:]
	}
	return block + html
}

// sanitizeCSS keeps a stylesheet from closing its own style element or
// smuggling a script tag into the message.
func sanitizeCSS(css string) string {
	css = reScript.ReplaceAllString(css, "")
	css = reScriptOpen.ReplaceAllString(css, "")
	css = reStyleClose.ReplaceAllString(css, "")
	return css
}

// htmlToText derives a readable plain-text alternative from a rendered HTML
// body. It is a fallback for authors who clear the text template, not a general
// purpose converter.
func htmlToText(h string) string {
	s := reScript.ReplaceAllString(h, "")
	s = reStyle.ReplaceAllString(s, "")
	s = reBreak.ReplaceAllString(s, "\n")
	s = reTag.ReplaceAllString(s, "")
	s = htmltemplate.HTMLEscapeString(s)
	s = strings.NewReplacer(
		"&amp;", "&", "&lt;", "<", "&gt;", ">", "&#34;", `"`, "&#39;", "'", "&nbsp;", " ",
	).Replace(s)

	var lines []string
	for _, line := range strings.Split(s, "\n") {
		lines = append(lines, strings.TrimSpace(line))
	}
	return strings.TrimSpace(reBlank.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}

// SampleMessage is representative content for the template preview, so an
// author can see their changes without waiting for a real reminder.
func SampleMessage(advance bool) Message {
	m := Message{
		ReminderID:  "00000000-0000-0000-0000-000000000000",
		Title:       "Pay the electricity bill",
		Description: "Account 12345 — due before the end of the month.",
		DueAt:       time.Now().UTC().Add(2 * time.Hour),
		Timezone:    "Asia/Kolkata",
		Priority:    "high",
		DisplayName: "Sam",
	}
	if advance {
		m.Advance = true
		m.LeadTime = time.Hour
	}
	return m
}
