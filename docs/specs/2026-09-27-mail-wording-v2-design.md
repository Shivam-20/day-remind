# Mail Wording v2 — Design

**Date:** 2026-09-27
**Status:** Approved-by-default (user asked for a richer mail wording; clarifying question unanswered, design follows best judgment on all four improvement axes)

## Problem

The mail wording feature (per-user email/Telegram templates for reminder alerts) is basic:

1. Only 7 template variables; no user name, list, tags, or "open in app" link.
2. The AI authoring flow is copy-paste ("Copy AI prompt" → external LLM → paste JSON back) even though `GEMINI_API_KEY` is configured in env and unused.
3. The built-in fallback email (sent when no template is active) is bare HTML with the due time in UTC.
4. The editor is raw textareas: no live preview, no variable cheat-sheet, no starter designs, no duplicate.

## Goals

- Templates can reference richer, more personal context without changing the data model.
- One-click AI generation of a complete template from a short brief (server-side Gemini call).
- Beautiful out-of-the-box emails: polished fallback design + 4 clonable starter templates.
- A materially better editing experience.

## Non-goals

- No changes to the storage schema (mail wordings stay embedded in the user document, max 30).
- No per-reminder template selection (one active template per user, as today).
- Morning digest emails keep their current design.
- No new email provider work (SMTP/Resend untouched).

## 1. Template variables v2

`notify.WordingData` gains fields; the list API exposes them; the editor shows clickable chips.

| Template action | Meaning |
|---|---|
| `{{.Title}}`, `{{.Description}}` | as before |
| `{{.DueAt}}` | **Now rendered in the reminder's timezone**, human format (e.g. `Fri, 02 Jan 2026 15:04 IST`) — was UTC RFC822 |
| `{{.Timezone}}`, `{{.Priority}}`, `{{.Advance}}` | as before |
| `{{.LeadTime}}` | clean lead, e.g. `1 hour` (was the raw `⏰ In 1 hour:` prefix) |
| `{{.UserName}}` | recipient display name (may be empty) |
| `{{.ListName}}` | list name, empty when the reminder has no list |
| `{{.Tags}}` | comma-separated tag names, empty when untagged |
| `{{.ReminderURL}}` | deep link `WEBHOOK_BASE_URL/#/reminders?open={id}`, empty when base URL unset |
| `{{.Today}}` | current date in the reminder's timezone |

Backward compatible: existing templates keep rendering; unknown fields simply stay unused.

## 2. Send path (scheduler)

`sendDelivery` receives a small `deliveryContext{Advance bool; Lead string}` instead of inferring
`Advance` from `strings.Contains(prefix, "In ")`. The scheduler passes:

- offset/occurrence deliveries → `{true, formatOffsetLead(minutes)}` (e.g. `1 hour`)
- due-now and escalation deliveries → `{false, ""}`

Per delivery the scheduler additionally loads (same per-delivery lookup cost as the existing
`ActiveMailWording` call): user display name, list name (when `ListID` set), tag names (new
`storage.TagNamesByID`), and computes `ReminderURL` from `WEBHOOK_BASE_URL`.

## 3. Built-in designs (`internal/notify/wording_presets.go`)

- **Default fallback email** (`DefaultReminderEmail(data)`): polished HTML — header with priority
  color accent, title, description, due chip in local time, "Open in app" button when `ReminderURL`
  is set, small footer. Inline-critical styles; used by the scheduler when no template is active.
- **4 starter presets**: "Clean minimal", "Bill payment", "Meeting prep", "Gentle nudge" — each a
  full `Wording` (subject/HTML/CSS/text/telegram), validated at startup by a test.
- Exposed to the UI via the list endpoint payload (`presets`), so the frontend stays thin and the
  server is the single source of truth.

## 4. AI generation

`POST /api/me/mail-wordings/generate` — body `{ "brief": "..." }` (max 500 chars), session-authenticated.

- Handler calls `notify.GenerateWordingWithAI(ctx, apiKey, model, brief)`.
- Gemini `generateContent` with `responseMimeType: application/json`; system-style prompt embeds the
  real variable list, sample data, and design quality bar (mobile-friendly, mail-client-safe CSS,
  short subject, plain-text + Telegram variants).
- Parse: strip code fences, extract first `{...}` block, unmarshal, `ValidateWording`. One retry on
  invalid JSON. Model default `gemini-2.5-flash`, overridable via `GEMINI_MODEL` env.
- **Returns the draft; nothing is saved** — the client loads it into the editor for review before
  the user saves (human stays in the loop).
- `400 ai generation is not configured` when no API key.

## 5. Editor UX (`wording-ui.js`)

- **Generate with AI** card: brief input + Generate button → result fills the editor.
- **Variable chips** under the editor: click inserts `{{.Field}}` at the cursor of the focused
  field (focus tracked on the textareas).
- **Live preview**: debounced (~700 ms) preview call while editing; renders the due-time email into
  an iframe below the form; toggleable, default on. The explicit Preview button still shows both
  due and advance renders.
- **Start from a design**: preset chips load a preset into the editor (add mode).
- **Duplicate** button on each card → editor prefilled with `name + " copy"`.
- "Paste AI result" stays as a fallback for deployments without an API key.

## Files

| File | Change |
|---|---|
| `internal/notify/wording.go` | WordingData fields, sample data |
| `internal/notify/wording_presets.go` | NEW: presets + default email |
| `internal/notify/ai_wording.go` | NEW: Gemini call + prompt + JSON parse |
| `internal/scheduler/runner.go` | deliveryContext, rich data lookups, tz-aware due |
| `internal/storage/tags.go` | `TagNamesByID` |
| `internal/web/mail_wording.go` | variables list v2, presets in payload, generate handler |
| `internal/web/server.go` | generate route |
| `internal/config/config.go` | `GeminiModel` (default `gemini-2.5-flash`) |
| `internal/web/static/js/wording-ui.js`, `wording-ai.js`, `css/app.css` | editor UX |
| tests | wording render v2, presets validate, AI parse (httptest), handler 400 |

## Testing

- `go test ./...` unit; notify tests cover rendering with new fields, preset validation, and AI
  response parsing against an `httptest` Gemini stub.
- Scheduler changes covered by an existing-pattern test where cheap; storage tests skip without DB.
- After rebuild: probe `/api/health`, list templates, and generate (requires key) through the
  container network.

## Rollout

Rebuild the Docker image (`docker compose up -d --build app`) — embed is compile-time. No DB
migration; no config change required (AI degrades to a 400 without a key; paste flow remains).
