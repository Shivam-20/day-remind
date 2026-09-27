package notify

import (
	"fmt"
	htmltemplate "html/template"
	"io"
	"regexp"
	"strings"
	texttemplate "text/template"
	"time"
)

// Wording is one saved email and message design.
type Wording struct {
	Name         string
	EmailSubject string
	EmailHTML    string
	EmailCSS     string
	EmailText    string
	TelegramText string
}

// WordingData is what a mail template can reference.
type WordingData struct {
	Title       string
	Description string
	DueAt       string
	Timezone    string
	Priority    string
	Advance     bool
	LeadTime    string

	// Context added in mail wording v2. Empty values simply render as nothing.
	UserName    string
	ListName    string
	Tags        string
	ReminderURL string
	Today       string
}

// RenderedWording is the result of applying a design. Empty fields mean the
// caller should keep its built-in wording for that part.
type RenderedWording struct {
	Subject  string
	HTML     string
	CSS      string
	Text     string
	Telegram string
	Problems []string
}

func ValidateWording(w Wording) error {
	check := func(field, body string, html bool) error {
		if strings.TrimSpace(body) == "" {
			return nil
		}
		var err error
		if html {
			var t *htmltemplate.Template
			t, err = htmltemplate.New(field).Parse(body)
			if err == nil {
				// Execute against empty data so a misspelled variable (e.g.
				// {{.Ttile}}) is rejected on save instead of failing at send time.
				err = t.Execute(io.Discard, WordingData{})
			}
		} else {
			var t *texttemplate.Template
			t, err = texttemplate.New(field).Parse(body)
			if err == nil {
				err = t.Execute(io.Discard, WordingData{})
			}
		}
		if err != nil {
			return fmt.Errorf("%s: %w", field, err)
		}
		return nil
	}
	for _, c := range []struct {
		field, body string
		html        bool
	}{
		{"email_subject", w.EmailSubject, false},
		{"email_html", w.EmailHTML, true},
		{"email_css", w.EmailCSS, false},
		{"email_text", w.EmailText, false},
		{"telegram_text", w.TelegramText, false},
	} {
		if err := check(c.field, c.body, c.html); err != nil {
			return err
		}
	}
	return nil
}

func RenderWording(w Wording, data WordingData) RenderedWording {
	var out RenderedWording
	render := func(field, body string, html bool) string {
		body = strings.TrimSpace(body)
		if body == "" {
			return ""
		}
		var sb strings.Builder
		var err error
		if html {
			t, perr := htmltemplate.New(field).Parse(body)
			if perr != nil {
				out.Problems = append(out.Problems, field+": "+perr.Error())
				return ""
			}
			err = t.Execute(&sb, data)
		} else {
			t, perr := texttemplate.New(field).Parse(body)
			if perr != nil {
				out.Problems = append(out.Problems, field+": "+perr.Error())
				return ""
			}
			err = t.Execute(&sb, data)
		}
		if err != nil {
			out.Problems = append(out.Problems, field+": "+err.Error())
			return ""
		}
		return sb.String()
	}
	out.Subject = strings.TrimSpace(render("email_subject", w.EmailSubject, false))
	html := render("email_html", w.EmailHTML, true)
	out.CSS = render("email_css", w.EmailCSS, false)
	if html != "" {
		out.HTML = injectWordingCSS(html, out.CSS)
	}
	out.Text = strings.TrimSpace(render("email_text", w.EmailText, false))
	out.Telegram = strings.TrimSpace(render("telegram_text", w.TelegramText, false))
	return out
}

func SampleWordingData(advance bool) WordingData {
	lead := ""
	if advance {
		lead = "1 hour"
	}
	loc := time.FixedZone("IST", 5*3600+1800)
	return WordingData{
		Title:       "Pay the electricity bill",
		Description: "Account 12345",
		DueAt:       time.Date(2026, 12, 25, 9, 0, 0, 0, loc).Format("Mon, 02 Jan 2006 15:04 MST"),
		Timezone:    "Asia/Kolkata",
		Priority:    "high",
		Advance:     advance,
		LeadTime:    lead,
		UserName:    "Alex",
		ListName:    "Bills",
		Tags:        "utilities, monthly",
		ReminderURL: "https://day-remind.example/#/reminders?open=demo",
		Today:       time.Date(2026, 12, 25, 9, 0, 0, 0, loc).Format("Mon, 02 Jan 2006"),
	}
}

// FormatDueAt renders a due time for templates and the default email, in the
// reminder's timezone. Falls back to UTC when the timezone is unknown.
func FormatDueAt(dueAt time.Time, timezone string) string {
	loc, err := time.LoadLocation(strings.TrimSpace(timezone))
	if err != nil || loc == nil {
		loc = time.UTC
	}
	return dueAt.In(loc).Format("Mon, 02 Jan 2006 15:04 MST")
}

var (
	reWordingScript = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</\s*script\s*>`)
	reWordingOpen   = regexp.MustCompile(`(?i)<\s*/?\s*script\b[^>]*>`)
	reWordingClose  = regexp.MustCompile(`(?i)</\s*style\s*>`)
)

// ApplyCSS inserts a stylesheet into an HTML document.
func ApplyCSS(html, css string) string {
	return injectWordingCSS(html, css)
}

func injectWordingCSS(html, css string) string {
	css = reWordingScript.ReplaceAllString(css, "")
	css = reWordingOpen.ReplaceAllString(css, "")
	css = reWordingClose.ReplaceAllString(css, "")
	if strings.TrimSpace(css) == "" {
		return html
	}
	block := "<style>" + css + "</style>"
	if i := strings.Index(strings.ToLower(html), "</head>"); i >= 0 {
		return html[:i] + block + html[i:]
	}
	return block + html
}
