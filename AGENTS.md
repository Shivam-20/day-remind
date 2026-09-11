# AGENTS.md — day-remind

Guide for AI agents working in this repository. Read this before making changes.

## What this project is

**Day Remind** is a multi-channel reminder web app. Users sign up, create dated reminders, pause/snooze/mark done, and receive notifications via email/Telegram when reminders are due.

- **Language:** Go 1.22 backend + vanilla JS SPA (embedded, no build step)
- **Database:** MongoDB 7 (official Go driver)
- **HTTP:** chi router, JSON API, session cookies
- **Module:** `github.com/shivam-20/day-remind`
- **Entrypoint:** `cmd/server/main.go`
- **Default URL:** http://localhost:8090

---

## Quick start

```bash
cp .env.example .env          # set SESSION_KEY (32+ chars), optional ADMIN_EMAILS
docker compose up -d          # MongoDB on port 27017
go run ./cmd/server
go test ./...
go build -o bin/day-remind ./cmd/server
```

CI (`.github/workflows/ci.yml`) runs tests against a MongoDB service container.

---

## Architecture

```
Browser (hash SPA)  →  internal/web  →  internal/storage  →  MongoDB
                           ↓                    ↑
                    internal/scheduler ─────────┘
                           ↓
                    internal/notify (email/Telegram send)
                    internal/reminder (domain types)
                    internal/auth (bcrypt passwords)
                    internal/config (env)
```

**Request flow**

1. Static UI loads from `//go:embed` in `internal/web/server.go`.
2. UI calls `/api/*` with `credentials: 'include'` (session cookie).
3. Protected routes use `requireSession` middleware → loads `userID` into context.
4. Admin routes add `requireAdmin` → checks `user.IsAdmin` in MongoDB.
5. Handlers call methods on `*storage.DB`; no repository interface layer.

**Layer rules**

| Layer | Package | Responsibility |
|-------|---------|----------------|
| Domain | `internal/reminder` | Types, status/priority constants — no I/O |
| Auth | `internal/auth` | Password hash/check only |
| Storage | `internal/storage` | All MongoDB access |
| Web | `internal/web` | HTTP handlers, routing, embedded static |
| Config | `internal/config` | Env parsing |
| Scheduler | `internal/scheduler` | Polls due reminders every 30s, dispatches alerts |
| Notify | `internal/notify` | Resend email + Telegram HTTP send adapters |

Do **not** import `storage` from `reminder`. Web imports both.

---

## Directory map

```
cmd/server/main.go          # Load config, open DB, start HTTP server
internal/
  auth/password.go          # bcrypt HashPassword / CheckPassword
  config/config.go          # DATABASE_URL, WEB_PORT, SESSION_KEY, ADMIN_EMAILS, RESEND, TELEGRAM
  reminder/reminder.go      # Reminder struct, CreateInput, UpdateInput, ListFilter
  reminder/snooze.go        # Snooze presets (+15m, +1h, +3h, tomorrow_9am)
  scheduler/runner.go       # Due-reminder poller, delivery dispatch + retries
  notify/                   # Email (Resend) and Telegram send helpers
  storage/
    db.go                   # Open(), Close(), connection + admin email set
    indexes.go              # ensureIndexes() on startup
    users.go                # User CRUD, notifications pause, quiet hours
    sessions.go             # Session tokens, TTL
    reminders.go            # User-scoped reminder CRUD
    deliveries.go           # reminder_deliveries outbox (pending/sent/failed/skipped)
    scheduler_queries.go    # ListDueReminders, GetUserNotifyState
    telegram.go             # Link tokens, chat ID lookup
    password_reset.go       # Password reset tokens
    admin.go                # Admin ops: all users/reminders, stats, cascade delete
    notifications.go        # Delivery settings (Resend/Telegram) in system_settings
    storage_test.go         # Integration test (needs DATABASE_URL)
  web/
    server.go               # Routes, embed static, ListenAndServe
    middleware.go           # requireSession, requireAdmin, context user ID
    auth.go                 # signup/login/logout/me, forgot/reset password
    reminders.go            # User reminder handlers + snooze presets
    deliveries.go           # Delivery log + in-app notifications list
    telegram.go             # Telegram webhook + link token API
    password_reset.go       # Forgot/reset password handlers
    notifications.go        # User alert channel settings
    admin.go                # Admin API handlers
    json.go                 # writeJSON, writeError, decodeJSON
    static/                 # Embedded frontend (edit → rebuild Go binary)
      index.html
      js/api.js             # fetch wrapper
      js/components.js      # toasts, drawers, cards, theme
      js/app.js             # Main SPA routes (#/today, #/reminders, #/settings, drawers)
      js/admin.js           # Admin panel (#/admin, #/admin/users, #/admin/reminders)
      sw.js                 # PWA service worker (static shell cache)
      css/theme.css         # CSS variables (dark/light)
      css/app.css           # Layout and components
docker-compose.yml          # MongoDB 7
.env.example
docs/specs/                 # Product + UI design (some references still say Postgres)
docs/plans/                 # Implementation roadmap
```

---

## Environment variables

| Variable | Required | Description |
|----------|----------|-------------|
| `DATABASE_URL` | yes | MongoDB URI, e.g. `mongodb://dayremind:dayremind@localhost:27017/day_remind?authSource=admin` |
| `SESSION_KEY` | yes | Min 32 chars (loaded but cookie is stateless token in DB today) |
| `WEB_PORT` | no | Default `8090` |
| `ADMIN_EMAILS` | no | Comma-separated emails auto-promoted to `is_admin: true` on startup + signup |
| `RESEND_API_KEY` | no | Resend API key for email alerts (admin can also set in DB) |
| `EMAIL_FROM` | no | Sender address for email alerts |
| `TELEGRAM_BOT_TOKEN` | no | Telegram bot token (admin can also set in DB) |

---

## MongoDB collections

Indexes created in `storage/indexes.go` on every `Open()`.

### `users`

```json
{
  "_id": "uuid",
  "email": "unique lowercase",
  "password_hash": "bcrypt",
  "display_name": "",
  "timezone": "Asia/Kolkata",
  "locale": "en",
  "is_admin": false,
  "notifications_paused": false,
  "notifications_paused_until": null,
  "notification_prefs": [{ "channel": "email", "enabled": true, "address": "..." }],
  "quiet_start": "22:00",
  "quiet_end": "08:00",
  "created_at": "UTC",
  "updated_at": "UTC"
}
```

### `sessions`

```json
{
  "_id": "hex token (64 chars)",
  "user_id": "uuid",
  "expires_at": "UTC (+30 days)",
  "created_at": "UTC"
}
```

TTL index on `expires_at` auto-deletes expired sessions.

### `reminders`

```json
{
  "_id": "uuid",
  "user_id": "uuid",
  "list_id": null,
  "title": "",
  "description": "",
  "due_at": "UTC",
  "timezone": "Asia/Kolkata",
  "status": "scheduled|paused|done|cancelled",
  "priority": "low|normal|high|urgent",
  "pinned": false,
  "snoozed_until": null,
  "paused_at": null,
  "resume_at": null,
  "completed_at": null,
  "created_at": "UTC",
  "updated_at": "UTC"
}
```

**Scheduler:** `internal/scheduler` polls every 30s, creates `reminder_deliveries` rows, sends via enabled channels. Respects vacation mode, quiet hours (urgent bypasses), paused/snoozed reminders. Retries failed sends up to 3 times.

### `reminder_deliveries`

```json
{
  "_id": "uuid",
  "reminder_id": "uuid",
  "user_id": "uuid",
  "channel": "email|telegram",
  "status": "pending|sent|failed|skipped",
  "attempt_count": 0,
  "last_error": "",
  "sent_at": null,
  "idempotency_key": "reminder_id:channel:due_at",
  "created_at": "UTC",
  "updated_at": "UTC"
}
```

### `telegram_link_tokens` / `password_reset_tokens`

Short-lived tokens for bot linking and password reset emails.

**Not implemented yet:** `lists`, tags, recurrence.

---

## HTTP API

All JSON. Errors: `{"error":"message"}` with appropriate status code.

### Public

| Method | Path | Notes |
|--------|------|-------|
| GET | `/api/health` | `{"status":"ok"}` |
| POST | `/api/auth/signup` | body: email, password, display_name, timezone → sets cookie |
| POST | `/api/auth/login` | body: email, password → sets cookie |
| POST | `/api/auth/logout` | Clears cookie |
| POST | `/api/auth/forgot-password` | body: email — sends reset link if Resend configured |
| POST | `/api/auth/reset-password` | body: token, password |
| POST | `/api/webhooks/telegram` | Telegram bot webhook (`/start`, `/list`, `/done`, `/snooze`, `/help`) |

### Authenticated (cookie `dr_session`)

| Method | Path | Notes |
|--------|------|-------|
| GET | `/api/me` | Current user incl. `is_admin` |
| POST | `/api/me/notifications/pause` | body: optional `until` ISO8601 |
| POST | `/api/me/notifications/resume` | |
| GET | `/api/me/notification-channels` | User email/Telegram alert prefs + availability |
| PATCH | `/api/me/notification-channels` | body: `{ email, telegram, quiet_start, quiet_end }` |
| POST | `/api/me/telegram/link` | Returns bot link command for Telegram pairing |
| GET | `/api/notifications` | Recent delivery attempts for current user |
| GET | `/api/snooze/presets` | Available snooze preset keys |
| GET | `/api/reminders` | query: `status`, `q`, `from`, `to` (RFC3339) |
| POST | `/api/reminders` | body: title, description, due_at, timezone, priority |
| GET/PATCH/DELETE | `/api/reminders/{id}` | User-scoped |
| GET | `/api/reminders/{id}/deliveries` | Delivery log for one reminder |
| POST | `/api/reminders/{id}/done` | Sets status done + completed_at |
| POST | `/api/reminders/{id}/pause` | optional resume_at |
| POST | `/api/reminders/{id}/resume` | |
| POST | `/api/reminders/{id}/snooze` | body: `preset` (+15m, +1h, +3h, tomorrow_9am) or `until` RFC3339 |

### Admin (session + `is_admin`)

| Method | Path | Notes |
|--------|------|-------|
| GET | `/api/admin/stats` | users, reminders, sessions counts |
| GET/PATCH | `/api/admin/settings/delivery` | Resend + Telegram bot config (secrets masked on GET) |
| POST | `/api/admin/settings/delivery/test` | body: `{ channel, to }` — send test alert |
| GET/POST | `/api/admin/users` | list (q, limit, offset) / create |
| GET/PATCH/DELETE | `/api/admin/users/{id}` | Cannot delete self |
| DELETE | `/api/admin/users/{id}/sessions` | Revoke all sessions |
| GET/POST | `/api/admin/reminders` | list (user_id, status, q, from, to) / create (needs user_id) |
| GET/PATCH/DELETE | `/api/admin/reminders/{id}` | Cross-user |
| POST | `/api/admin/reminders/{id}/done\|pause\|resume\|snooze` | Same as user routes (snooze accepts preset or until) |

Storage errors: `ErrNotFound` → 404, `ErrConflict` → 409 (duplicate email).

---

## Frontend (embedded SPA)

**No bundler.** Edit files in `internal/web/static/`, then rebuild/restart Go server.

**Routing:** hash-based (`#/today`, `#/reminders`, `#/settings`, `#/reset-password`, `#/admin/...`).

| File | Role |
|------|------|
| `app.js` | Auth gate, main shell, today/all/settings views, create/detail/notification drawers, password reset, SW registration |
| `admin.js` | Admin shell, dashboard, user/reminder tables, admin drawers |
| `components.js` | Shared UI: `showToast`, `openDrawer`, `reminderCardHTML`, theme toggle |
| `api.js` | `API.get/post/patch/delete` with credentials |

**Global state:** `state.user`, `state.reminders` in `app.js`; `adminState` in `admin.js`.

Admin routes registered via `window.DR_ADMIN_ROUTES` merged into `routes` in `app.js`.

Non-admins navigating to `#/admin` are redirected to `#/today`.

---

## Conventions (follow these)

1. **Timestamps:** Store UTC in MongoDB; UI displays in user/browser timezone.
2. **Emails:** Lowercase + trim on write; unique index on `users.email`.
3. **Session cookie:** Name `dr_session`, HttpOnly, SameSite=Lax, 30-day TTL.
4. **Reminder status:** Only `scheduled`, `paused`, `done`, `cancelled`.
5. **Reminder priority:** `low`, `normal`, `high`, `urgent` (default `normal`).
6. **Static embed:** After editing `internal/web/static/*`, run `go build` or `go run` again.
7. **Storage API:** Add new DB methods on `*storage.DB`; keep handlers thin.
8. **JSON decode:** `decodeJSON` uses `DisallowUnknownFields` — match request struct tags.
9. **Imports:** Top of file only (no inline imports).
10. **Scope:** Minimal diffs; match existing naming and patterns; avoid over-abstraction.

---

## Testing

```bash
go test ./...                              # unit + storage (storage skips without DB)
DATABASE_URL=mongodb://... go test ./internal/storage/...
```

`storage_test.go` covers signup → create reminder → pause → list flow.

---

## Common tasks for agents

### Add a new API endpoint

1. Add storage method in `internal/storage/` if DB access needed.
2. Add handler in `internal/web/` (or extend existing file by domain).
3. Register route in `internal/web/server.go`.
4. Use `requireSession` and/or `requireAdmin` via chi route groups.
5. Add frontend call in `api.js` + relevant JS if user-facing.

### Add a new UI page

1. Add render function and route in `app.js` (or `admin.js`).
2. Add nav link in `shellHTML` / `adminShellHTML`.
3. Add styles in `app.css` if needed.
4. Rebuild server.

### Add a MongoDB collection

1. Define BSON document struct in storage package.
2. Add indexes in `indexes.go`.
3. Add CRUD methods; wire handlers if exposed via API.

### Promote admin user

Set `ADMIN_EMAILS=their@email.com` in `.env`, restart server. Existing user gets `is_admin: true` on startup; new signups with that email are admin immediately.

---

## MVP status

**Done**

- MongoDB storage + indexes
- Auth (signup, login, session cookie, logout, forgot/reset password)
- Reminder CRUD + pause/resume/snooze/done (presets: +15m, +1h, +3h, tomorrow 9am)
- Vacation mode (pause all notifications) + quiet hours
- User alert channels (email address + Telegram chat ID / bot link)
- Admin delivery settings (Resend API + Telegram bot) with test send
- Scheduler: due-reminder dispatch with outbox, retries, vacation/quiet-hour gates
- Telegram bot webhook (`/start`, `/list`, `/done`, `/snooze`)
- Delivery log API + in-app notification bell
- Embedded web UI (Today, All, Settings, alert channels, detail drawer)
- Admin API + admin panel UI
- PWA service worker (static shell cache)

**Not done** (see `docs/plans/2026-09-11-day-remind.md`)

- Lists, tags, recurrence
- JSON import/export UI
- Google/Gmail sync

---

## Docs reference

| File | Contents |
|------|----------|
| `README.md` | User-facing quick start + admin API table |
| `docs/specs/2026-09-11-day-remind-design.md` | Full product vision (DB section outdated — now MongoDB) |
| `docs/specs/2026-09-11-day-remind-ui-design.md` | UI/UX spec |
| `docs/plans/2026-09-11-day-remind.md` | Phased implementation checklist |

When specs conflict with code, **trust the code** and update docs if asked.

---

## Pitfalls

- **Docker path:** Repo root is the module root (not a `day-remind/` subfolder despite older README examples).
- **Postgres references:** Removed; do not reintroduce pgx/goose without explicit request.
- **Admin self-delete:** Blocked in `handleAdminDeleteUser`.
- **Delete user:** Cascades reminders + sessions in `storage.DeleteUser`.
- **Embedded static caching:** Browser may cache `/static/*`; HTML sets `no-cache` on `/`.
