package notify

import (
	"bytes"
	htmltemplate "html/template"
	"strings"
)

// PriorityAccentColor is the accent used across built-in mail designs.
func PriorityAccentColor(priority string) string {
	switch priority {
	case "urgent":
		return "#dc2626"
	case "high":
		return "#ea580c"
	case "low":
		return "#64748b"
	default:
		return "#2563eb"
	}
}

const defaultEmailLayout = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>
<body style="margin:0;padding:0;background:#f1f3f6;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f1f3f6;padding:24px 12px;">
<tr><td align="center">
<table role="presentation" width="560" cellpadding="0" cellspacing="0" style="max-width:560px;width:100%;background:#ffffff;border-radius:12px;overflow:hidden;">
<tr><td style="height:4px;background:{{.Accent}};font-size:0;line-height:0;">&nbsp;</td></tr>
<tr><td style="padding:28px 32px 8px 32px;">
  <p style="margin:0 0 10px 0;font-size:12px;font-weight:600;letter-spacing:0.08em;text-transform:uppercase;color:{{.Accent}};">{{.Eyebrow}}</p>
  <h1 style="margin:0 0 8px 0;font-size:22px;line-height:1.3;color:#111827;">{{.Title}}</h1>
  {{if .Description}}<p style="margin:0 0 16px 0;font-size:15px;line-height:1.55;color:#4b5563;">{{.Description}}</p>{{end}}
  <table role="presentation" cellpadding="0" cellspacing="0"><tr>
    <td style="background:#f3f4f6;border-radius:8px;padding:8px 14px;font-size:14px;color:#111827;">&#128337; {{.DueAt}}</td>
    {{if .Meta}}<td style="padding-left:8px;"><span style="font-size:13px;color:#6b7280;">{{.Meta}}</span></td>{{end}}
  </tr></table>
</td></tr>
{{if .ReminderURL}}<tr><td style="padding:20px 32px 4px 32px;">
  <a href="{{.ReminderURL}}" style="display:inline-block;background:{{.Accent}};color:#ffffff;text-decoration:none;font-size:14px;font-weight:600;padding:10px 20px;border-radius:8px;">Open in Day Remind</a>
</td></tr>{{end}}
<tr><td style="padding:20px 32px 24px 32px;">
  <p style="margin:0;font-size:12px;color:#9ca3af;border-top:1px solid #f3f4f6;padding-top:14px;">Sent by Day Remind{{if .Today}} &middot; {{.Today}}{{end}}</p>
</td></tr>
</table>
</td></tr></table>
</body></html>`

// DefaultReminderEmail renders the built-in fallback email used when the user
// has no active mail template (or their template has no HTML body).
func DefaultReminderEmail(data WordingData) string {
	accent := PriorityAccentColor(data.Priority)
	eyebrow := "Reminder"
	if data.Advance {
		eyebrow = "Heads up"
	}
	meta := strings.TrimSpace(data.LeadTime)
	params := struct {
		Accent, Eyebrow, Title, Description, DueAt, Meta, ReminderURL, Today string
	}{
		Accent:      accent,
		Eyebrow:     eyebrow,
		Title:       data.Title,
		Description: data.Description,
		DueAt:       data.DueAt,
		Meta:        meta,
		ReminderURL: data.ReminderURL,
		Today:       data.Today,
	}
	t, err := htmltemplate.New("default_email").Parse(defaultEmailLayout)
	if err != nil {
		return "<p><strong>" + data.Title + "</strong></p><p>Due: " + data.DueAt + "</p>"
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, params); err != nil {
		return "<p><strong>" + data.Title + "</strong></p><p>Due: " + data.DueAt + "</p>"
	}
	return buf.String()
}

// WordingPresets are the built-in starter designs offered in the editor.
// They are never active by themselves; the user saves a copy to customize.
func WordingPresets() []Wording {
	return []Wording{
		presetCleanMinimal(),
		presetBillPayment(),
		presetMeetingPrep(),
		presetGentleNudge(),
	}
}

func presetCleanMinimal() Wording {
	return Wording{
		Name:         "Clean minimal",
		EmailSubject: `{{if .Advance}}Heads up: {{.Title}} in {{.LeadTime}}{{else}}{{.Title}}{{end}}`,
		EmailHTML: `<!DOCTYPE html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>
<body style="margin:0;padding:0;background:#fafafa;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#fafafa;padding:28px 12px;"><tr><td align="center">
<table role="presentation" width="520" cellpadding="0" cellspacing="0" style="max-width:520px;width:100%;background:#ffffff;border:1px solid #e5e7eb;border-radius:10px;">
<tr><td style="padding:30px 34px 10px 34px;">
  <p style="margin:0 0 12px 0;font-size:12px;letter-spacing:0.1em;text-transform:uppercase;color:#9ca3af;">{{if .Advance}}In {{.LeadTime}}{{else}}Due now{{end}}</p>
  <h1 style="margin:0 0 10px 0;font-size:21px;color:#111827;">{{.Title}}</h1>
  {{if .Description}}<p style="margin:0 0 14px 0;font-size:15px;line-height:1.6;color:#4b5563;">{{.Description}}</p>{{end}}
  <p style="margin:0 0 14px 0;font-size:15px;color:#111827;">&#128337; <strong>{{.DueAt}}</strong></p>
  {{if .Tags}}<p style="margin:0 0 14px 0;font-size:13px;color:#6b7280;">{{.Tags}}</p>{{end}}
</td></tr>
{{if .ReminderURL}}<tr><td style="padding:0 34px 24px 34px;"><a href="{{.ReminderURL}}" style="font-size:14px;color:#2563eb;text-decoration:underline;">Open in Day Remind &rarr;</a></td></tr>{{end}}
</table></td></tr></table></body></html>`,
		EmailText: `{{if .Advance}}Heads up — {{.Title}} is due in {{.LeadTime}}.{{else}}{{.Title}} is due now.{{end}}
{{if .Description}}{{.Description}}
{{end}}Due: {{.DueAt}}
{{if .ReminderURL}}{{.ReminderURL}}{{end}}`,
		TelegramText: `{{if .Advance}}⏰ In {{.LeadTime}}:{{else}}🔔 Now:{{end}} <b>{{.Title}}</b>
{{if .Description}}{{.Description}}
{{end}}🕐 {{.DueAt}}{{if .ReminderURL}}
{{.ReminderURL}}{{end}}`,
	}
}

func presetBillPayment() Wording {
	return Wording{
		Name:         "Bill payment",
		EmailSubject: `{{if .Advance}}Payment due in {{.LeadTime}}: {{.Title}}{{else}}Pay now: {{.Title}}{{end}}`,
		EmailHTML: `<!DOCTYPE html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>
<body style="margin:0;padding:0;background:#fefce8;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#fefce8;padding:24px 12px;"><tr><td align="center">
<table role="presentation" width="560" cellpadding="0" cellspacing="0" style="max-width:560px;width:100%;background:#ffffff;border-radius:14px;overflow:hidden;border:1px solid #fde68a;">
<tr><td style="height:5px;background:#d97706;font-size:0;line-height:0;">&nbsp;</td></tr>
<tr><td style="padding:30px 34px 6px 34px;">
  <p style="margin:0 0 10px 0;font-size:12px;font-weight:700;letter-spacing:0.08em;text-transform:uppercase;color:#b45309;">{{if .Advance}}Payment reminder &middot; due in {{.LeadTime}}{{else}}Payment due{{end}}</p>
  <h1 style="margin:0 0 8px 0;font-size:24px;color:#1c1917;">{{.Title}}</h1>
  {{if .Description}}<p style="margin:0 0 16px 0;font-size:15px;color:#57534e;">Reference: {{.Description}}</p>{{end}}
  <table role="presentation" cellpadding="0" cellspacing="0"><tr><td style="background:#fffbeb;border:1px solid #fde68a;border-radius:10px;padding:12px 18px;font-size:16px;color:#1c1917;">Pay by <strong>{{.DueAt}}</strong></td></tr></table>
  {{if .Tags}}<p style="margin:14px 0 0 0;font-size:13px;color:#a8a29e;">Filed under: {{.Tags}}</p>{{end}}
</td></tr>
{{if .ReminderURL}}<tr><td style="padding:22px 34px 4px 34px;">
  <a href="{{.ReminderURL}}" style="display:inline-block;background:#d97706;color:#ffffff;text-decoration:none;font-size:15px;font-weight:600;padding:12px 24px;border-radius:10px;">Mark as paid</a>
</td></tr>{{end}}
<tr><td style="padding:18px 34px 26px 34px;"><p style="margin:0;font-size:12px;color:#d6d3d1;">Avoid late fees — settle before {{.DueAt}}.</p></td></tr>
</table></td></tr></table></body></html>`,
		EmailText: `{{if .Advance}}Payment reminder: {{.Title}} is due in {{.LeadTime}}.{{else}}Payment due: {{.Title}}.{{end}}
{{if .Description}}Reference: {{.Description}}
{{end}}Pay by: {{.DueAt}}
{{if .ReminderURL}}Open to mark as paid: {{.ReminderURL}}{{end}}`,
		TelegramText: `{{if .Advance}}💸 Due in {{.LeadTime}}:{{else}}💸 Pay now:{{end}} <b>{{.Title}}</b>
{{if .Description}}Ref: {{.Description}}
{{end}}🕐 Pay by {{.DueAt}}{{if .ReminderURL}}
{{.ReminderURL}}{{end}}`,
	}
}

func presetMeetingPrep() Wording {
	return Wording{
		Name:         "Meeting prep",
		EmailSubject: `{{if .Advance}}Coming up in {{.LeadTime}}: {{.Title}}{{else}}Starting now: {{.Title}}{{end}}`,
		EmailHTML: `<!DOCTYPE html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>
<body style="margin:0;padding:0;background:#eff6ff;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#eff6ff;padding:24px 12px;"><tr><td align="center">
<table role="presentation" width="560" cellpadding="0" cellspacing="0" style="max-width:560px;width:100%;background:#ffffff;border-radius:14px;overflow:hidden;">
<tr><td style="height:5px;background:#1d4ed8;font-size:0;line-height:0;">&nbsp;</td></tr>
<tr><td style="padding:30px 34px 6px 34px;">
  <p style="margin:0 0 10px 0;font-size:13px;font-weight:600;color:#1d4ed8;">{{if .Advance}}Starts in {{.LeadTime}}{{else}}Starting now{{end}}</p>
  <h1 style="margin:0 0 10px 0;font-size:22px;color:#0f172a;">{{.Title}}</h1>
  <p style="margin:0 0 14px 0;font-size:15px;color:#334155;">&#128197; <strong>{{.DueAt}}</strong></p>
  {{if .Description}}<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f8fafc;border-left:3px solid #1d4ed8;border-radius:0 8px 8px 0;margin:0 0 16px 0;"><tr><td style="padding:12px 16px;font-size:14px;line-height:1.6;color:#475569;">{{.Description}}</td></tr></table>{{end}}
  {{if .UserName}}<p style="margin:0 0 12px 0;font-size:14px;color:#64748b;">See you there, {{.UserName}}.</p>{{end}}
</td></tr>
{{if .ReminderURL}}<tr><td style="padding:0 34px 26px 34px;">
  <a href="{{.ReminderURL}}" style="display:inline-block;background:#1d4ed8;color:#ffffff;text-decoration:none;font-size:14px;font-weight:600;padding:11px 22px;border-radius:8px;">Open details</a>
</td></tr>{{end}}
</table></td></tr></table></body></html>`,
		EmailText: `{{if .Advance}}{{.Title}} starts in {{.LeadTime}}.{{else}}{{.Title}} is starting now.{{end}}
When: {{.DueAt}}
{{if .Description}}Agenda: {{.Description}}
{{end}}{{if .ReminderURL}}Details: {{.ReminderURL}}{{end}}`,
		TelegramText: `{{if .Advance}}📅 Starts in {{.LeadTime}}:{{else}}📅 Starting now:{{end}} <b>{{.Title}}</b>
🕐 {{.DueAt}}{{if .Description}}
{{.Description}}{{end}}{{if .ReminderURL}}
{{.ReminderURL}}{{end}}`,
	}
}

func presetGentleNudge() Wording {
	return Wording{
		Name:         "Gentle nudge",
		EmailSubject: `{{if .Advance}}A little heads-up about {{.Title}}{{else}}Time for {{.Title}}{{end}}`,
		EmailHTML: `<!DOCTYPE html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>
<body style="margin:0;padding:0;background:#fdf4ff;font-family:Georgia,'Times New Roman',serif;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#fdf4ff;padding:28px 12px;"><tr><td align="center">
<table role="presentation" width="520" cellpadding="0" cellspacing="0" style="max-width:520px;width:100%;background:#ffffff;border-radius:20px;padding:8px;">
<tr><td style="background:#fdf2f8;border-radius:14px;padding:32px 36px 26px 36px;">
  <p style="margin:0 0 14px 0;font-size:26px;">&#127800;</p>
  <h1 style="margin:0 0 10px 0;font-size:21px;font-weight:normal;color:#4a044e;">{{if .Advance}}{{.Title}} — coming up in {{.LeadTime}}{{else}}When you have a moment: {{.Title}}{{end}}</h1>
  {{if .Description}}<p style="margin:0 0 14px 0;font-size:15px;line-height:1.7;color:#701a75;">{{.Description}}</p>{{end}}
  <p style="margin:0 0 16px 0;font-size:15px;color:#86198f;">Planned for <strong>{{.DueAt}}</strong></p>
  {{if .UserName}}<p style="margin:0 0 14px 0;font-size:14px;color:#9d174d;">— your friendly reminder, {{.UserName}}</p>{{end}}
  {{if .ReminderURL}}<p style="margin:0;"><a href="{{.ReminderURL}}" style="color:#a21caf;text-decoration:underline;font-size:14px;">View it in Day Remind</a></p>{{end}}
</td></tr></table></td></tr></table></body></html>`,
		EmailText: `{{if .Advance}}A gentle heads-up: {{.Title}} is coming up in {{.LeadTime}}.{{else}}A gentle nudge: {{.Title}}.{{end}}
{{if .Description}}{{.Description}}
{{end}}Planned for: {{.DueAt}}
{{if .ReminderURL}}{{.ReminderURL}}{{end}}`,
		TelegramText: `{{if .Advance}}🌸 Coming up in {{.LeadTime}}:{{else}}🌸 When you have a moment:{{end}} <b>{{.Title}}</b>
🕐 {{.DueAt}}{{if .Description}}
{{.Description}}{{end}}`,
	}
}
