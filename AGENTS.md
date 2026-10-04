# AGENTS.md — day-remind

Guide for AI agents working in this repository. Read this before making changes.

## What this project is

**Day Remind** is a multi-channel reminder web app. Users sign up, create dated reminders, pause/snooze/mark done, and receive notifications via email/Telegram when reminders are due.

- **Language:** Go 1.22 backend + vanilla JS SPA (embedded, no build step)
- **Database:** MongoDB 7 (official Go driver)
- **HTTP:** chi router, JSON API, session cookies
- **Module:** `github.com/shivam-20/day-remind`
- **Entrypoint:** `cmd/server/main.go`
- **Default local URL:** http://localhost:8090 (host-published only if you map the port)
- **Production URL on this machine:** http://161.118.181.5/remind/ (OCI reverse tunnel + home Caddy)

---

## Quick start (local)

```bash
cp .env.example .env          # set SESSION_KEY (32+ chars), optional ADMIN_EMAILS
docker compose up -d          # MongoDB (+ app container when using compose `app` service)
go run ./cmd/server           # or: docker compose up -d --build app
go test ./...
go build -o bin/day-remind ./cmd/server
```

CI (`.github/workflows/ci.yml`) runs tests against a MongoDB service container.

---

## Production deploy (THIS MACHINE — mandatory for public access)

When the user asks to **deploy / publish / expose day-remind**, do it **only** via the home OCI reverse-tunnel + Caddy path proxy. Do **not** open Airtel router ports, Tailscale Funnel, or Oracle `:80`/`:443` for this app.

### Architecture (how traffic reaches the app)

```text
Browser
  → http://161.118.181.5/remind/...          # Oracle Caddy IP vhost :80
      → 127.0.0.1:18080 on Oracle            # SSH -R from home
          → 127.0.0.1:8080 home              # Docker Caddy (~/apps/caddy)
              → strip /remind
                  → day-remind-app:8090      # this compose service on apps-net

  → http://161.118.181.5:18080/remind/...    # same tunnel, skip Oracle Caddy
```

| Piece | Location | Notes |
|-------|----------|-------|
| App + Mongo | this repo `docker compose` | `app` on external Docker network `apps-net`; Mongo on `127.0.0.1:27017` |
| Path proxy | `~/apps/caddy/Caddyfile` | `handle /remind* { uri strip_prefix /remind; reverse_proxy day-remind-app:8090 }` |
| Landing | `~/apps/caddy/www/index.html` | List `/remind` as live |
| Tunnel | `oracle-reverse-ssh.service` | Forwards OCI `:18080` → home Caddy `:8080` (+ SSH `:2222`) |
| Playbook | `~/oracle-home-ssh/docs/DEPLOY-APP.md` | Generic multi-app deploy rules |

### Required env for path deploy

In `.env` (already used on this machine):

```bash
WEB_PORT=8090
APP_BASE_PATH=/remind
# Public URL — REQUIRED on prod (Telegram alert buttons + email links break with localhost):
WEBHOOK_BASE_URL=http://161.118.181.5/remind
# GOOGLE_REDIRECT_URL=http://161.118.181.5/remind/api/integrations/google/callback
```

`APP_BASE_PATH` makes `serveIndex` inject `<base href="/remind/">`. That only works if static tags are **relative** and API calls go through `API.url()` (see below).

### Deploy / update checklist (follow in order)

1. **Preconditions**
   ```bash
   ~/oracle-home-ssh/bin/ohssh app-status
   docker network inspect apps-net >/dev/null   # must exist; caddy + day-remind-app on it
   ```
2. **Build & run this app** (from repo root)
   ```bash
   docker compose up -d --build app
   # confirm: docker ps --filter name=day-remind-app
   # confirm from Caddy network:
   docker exec caddy wget -qO- http://day-remind-app:8090/api/health
   ```
   The image build needs the sibling checkout `../telegram-gateway` (go.mod has
   `replace github.com/shivam-20/telegram-gateway => ../telegram-gateway`;
   compose passes it as the `telegram-gateway` additional build context). Build
   fails with "open /telegram-gateway/go.mod" if the sibling is missing.
   Env-file changes need `docker compose up -d --force-recreate app` (env is
   read at container creation only).
3. **Home Caddy route** — ensure `~/apps/caddy/Caddyfile` contains:
   ```caddy
   handle /remind* {
       uri strip_prefix /remind
       reverse_proxy day-remind-app:8090
   }
   ```
   Reload: `cd ~/apps/caddy && docker compose up -d`
4. **Landing page** — `/remind` marked live in `~/apps/caddy/www/index.html`.
5. **Verify**
   ```bash
   curl -fsS http://161.118.181.5/remind/api/health
   curl -fsS -o /dev/null -w '%{http_code} %{content_type}\n' \
     http://161.118.181.5/remind/static/css/app.css
   # expect: 200 text/css
   # browser: http://161.118.181.5/remind/  → full Sign-in UI (not only "Skip to content")
   ```

### Do **not** do these

- Publish host port `8090` publicly (compose may omit host publish on purpose; reach via Caddy/`apps-net`).
- Proxy Oracle `:80` yourself or edit Oracle duckdns Caddy for day-remind (IP vhost already forwards to home `:18080`).
- Open a **new** OCI security-list port for each app (reuse `18080` / `80`).
- Use absolute `/static/...` or raw `fetch('/api/...')` without `API.url()` when `APP_BASE_PATH` is set — browsers ignore `<base>` for absolute paths and you get a blank page showing only **Skip to content**.

### Base-path rules (keep working)

| Layer | Rule |
|-------|------|
| `internal/web/static/index.html` | Asset `href`/`src` must be **relative**: `static/css/...`, not `/static/...` |
| `internal/web/static/js/api.js` | All API traffic via `API.url(path)` / `API.request` (prefixes `APP_BASE_PATH`) |
| Other JS | Any `fetch('/api/...')` or `href="/api/..."` must use `API.url(...)` |
| `server.go` | Injects `<base href="/{APP_BASE_PATH}/">` when `BasePath` non-empty |
| Caddy | `uri strip_prefix /remind` so the Go app still sees `/api` and `/static` at root |
| Service worker | Do not register SW under subpath deploy (scope breaks); `app.js` skips SW when base ≠ empty |

After editing embedded static files: **rebuild the Docker image** (`docker compose up -d --build app`) or rebuild the Go binary — embed is compile-time.

### Symptom → fix

| Symptom | Cause | Fix |
|---------|-------|-----|
| Only “Skip to content” visible | CSS/JS loaded from `/static` (landing Caddy) | Relative assets + rebuild |
| `/remind/` 502 | App container down / not on `apps-net` | `docker compose up -d --build app`; check networks |
| `/api/health` ok on container, fail on OCI | Tunnel or Caddy route | `ohssh app-status`; check Caddyfile `/remind*` |
| Login works locally, cookies fail on OCI path | Wrong public URLs / Secure cookie later | Keep SameSite=Lax; set `WEBHOOK_BASE_URL` to public `/remind` |
| Telegram alert fails: `inline keyboard button URL 'http://localhost:8090/…' is invalid` | `WEBHOOK_BASE_URL` unset/localhost | Set `WEBHOOK_BASE_URL=http://161.118.181.5/remind` in `.env`, `--force-recreate` container |
| Email fails: `550 … domain not allowed` | From-domain not verified in Mailtrap | From must be exactly `hello@mail.dev4help.in` (verified domain is `mail.dev4help.in`; `help4dev.in` is a transposition typo) |
| Email fails: resend `401 API key is invalid` | Mailtrap token pasted into the Resend key field, or provider left on "Resend API" | Provider = **SMTP server**; Mailtrap token only in SMTP password field (server-side guard rejects mismatched provider saves) |
| Alerts not arriving, logs clean | Send errors return to client, not stdout | Check `reminder_deliveries` docs (status/last_error) in Mongo, then Admin → Delivery test send |

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
| Notify | `internal/notify` | Email (Resend API **or** generic SMTP) + Telegram send adapters (gateway-aware when configured) |

Do **not** import `storage` from `reminder`. Web imports both.

### Telegram Gateway (production)

Production uses **telegram-gateway** on a1-watcher (`https://dev4help.in/tg`). Setting
`TELEGRAM_GATEWAY_URL` + `TELEGRAM_GATEWAY_API_TOKEN` makes day-remind stop calling
`getUpdates` / `setWebhook` and route all Telegram traffic through the gateway:

- **Outbound:** `callTelegram` via `/v1/telegram/{method}`.
- **Inbound:** pull `/v1/inbound/updates` + ack (`TELEGRAM_GATEWAY_INBOUND_MODE=poll`).
- **Transport:** gateway service id `day-remind` is `update_mode: polling`. Do not start
  a second poller on this bot token.

The gateway host cannot reach `day-remind-app` Docker DNS, so inbound is **poll**, not
a Deliver URL webhook. Rollback: `../telegram-gateway/scripts/safe-rollout-day-remind-rollback.sh`.

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
  webhook/                  # Outbound user webhooks: SSRF-safe client, HMAC signing, outbox dispatcher
  notify/                   # Email (Resend + SMTP in smtp.go) and Telegram send helpers
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
    email_verification.go   # Email verification tokens (24h, single-use)
    admin.go                # Admin ops: all users/reminders, stats, cascade delete
    notifications.go        # Delivery settings (Resend/SMTP/Telegram) in system_settings
    storage_test.go         # Integration test (needs DATABASE_URL)
  web/
    server.go               # Routes, embed static, ListenAndServe
    middleware.go           # requireSession, requireAdmin, context user ID
    auth.go                 # signup/login/logout/me, forgot/reset password
    email_verification.go   # Verify/resend-email handlers + appBaseURL helper (base-path aware links)
    reminders.go            # User reminder handlers + snooze presets
    deliveries.go           # Delivery log + in-app notifications list
    telegram.go             # Telegram dispatcher + commands + polling/webhook + link token API
    admin_telegram.go       # Admin: linked users, unlink/message, webhook status
    admin_settings.go       # Admin delivery settings (Resend/SMTP/Telegram + provider guard)
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
docker-compose.yml          # MongoDB 7 + app service (apps-net for home Caddy)
Dockerfile                  # multi-stage Go build → day-remind binary
.env.example
docs/specs/                 # Product + UI design (some references still say Postgres)
docs/plans/                 # Implementation roadmap
```

---

## Environment variables

| Variable | Required | Description |
|----------|----------|-------------|
| `DATABASE_URL` | yes | MongoDB URI, e.g. `mongodb://dayremind:dayremind@localhost:27017/day_remind?authSource=admin` (compose overrides to `mongo:27017` for the `app` service) |
| `SESSION_KEY` | yes | Min 32 chars (loaded but cookie is stateless token in DB today) |
| `WEB_PORT` | no | Default `8090` |
| `APP_BASE_PATH` | no | Public path prefix, e.g. `/remind` — **required** for OCI/Caddy path deploy |
| `ADMIN_EMAILS` | no | Comma-separated emails auto-promoted to `is_admin: true` on startup + signup |
| `RESEND_API_KEY` | no | Resend API key for email alerts (admin can also set in DB) |
| `EMAIL_FROM` | no | Sender address for email alerts |
| `SMTP_HOST` | no | SMTP email provider host (admin picks Resend **or** SMTP in Admin → Delivery; stored in DB with env fallback). Prod: `live.smtp.mailtrap.io` |
| `SMTP_PORT` | no | Default `587`; `465` implies implicit TLS when `SMTP_TLS` unset |
| `SMTP_USERNAME` | no | SMTP auth username (Mailtrap uses `api`) |
| `SMTP_PASSWORD` | no | SMTP auth secret (write-only via admin API, masked on GET) |
| `SMTP_TLS` | no | `starttls` (default) \| `ssl` \| `none` |
| `TELEGRAM_BOT_TOKEN` | no | Telegram bot token (admin can also set in DB) |
| `TELEGRAM_UPDATE_MODE` | no | `polling` (default, no public URL needed) \| `webhook` \| `off`. Webhook mode requires an `https` `WEBHOOK_BASE_URL`. Ignored for transport when `TELEGRAM_GATEWAY_URL` is set |
| `TELEGRAM_WEBHOOK_SECRET` | webhook mode | `secret_token` for webhook mode (direct or gateway push). **Required** — `/api/webhooks/telegram` rejects every update when unset or mismatched, and returns 404 unless a webhook transport is configured |
| `TELEGRAM_GATEWAY_URL` | no | Telegram-gateway base URL (prod `https://dev4help.in/tg`). When set, day-remind routes all Telegram traffic through the gateway |
| `TELEGRAM_GATEWAY_API_TOKEN` | no | Bearer API token (`tgw_…`) issued by the gateway for the day-remind service |
| `TELEGRAM_GATEWAY_INBOUND_MODE` | no | How day-remind receives updates from the gateway: `poll` (default) \| `webhook` \| `both` |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` / `GOOGLE_REDIRECT_URL` | no | Google OAuth for Calendar/Gmail sync |
| `VAPID_PUBLIC_KEY` / `VAPID_PRIVATE_KEY` | no | Web push notifications |
| `TWILIO_ACCOUNT_SID` / `TWILIO_AUTH_TOKEN` / `TWILIO_FROM` | no | SMS channel |
| `AI_PROVIDER` | no | `openrouter` \| `gemini` — default: OpenRouter when `OPENROUTER_API_KEY` is set, else Gemini |
| `OPENROUTER_API_KEY` | no | OpenRouter API key (mail wording via [OpenRouter Go SDK](https://github.com/OpenRouterTeam/go-sdk)) |
| `OPENROUTER_MODEL` | no | OpenRouter model slug (default `google/gemini-2.5-flash`) |
| `GEMINI_API_KEY` | no | Direct Google Gemini API key when `AI_PROVIDER=gemini` |
| `GEMINI_MODEL` | no | Gemini model for wording generation (default `gemini-2.5-flash`) |
| `WEBHOOK_BASE_URL` | no | Public base URL — drives Telegram inline-button deep links, password-reset/verification email links. **Prod must set `http://161.118.181.5/remind`** or buttons use localhost and Telegram rejects them |
| `MAX_FREE_REMINDERS` | no | Plan limit (default 50) |

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
  "email_verified_at": null,
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

### `webhooks` / `webhook_deliveries` (outbound user webhooks)

`webhooks`: user endpoint (`url`, `secret`, `events`, `enabled`, failure streak,
`last_*`). `webhook_deliveries`: outbox row per event × endpoint, unique
`idempotency_key` = `webhook_id:event:reminder_id:due_at`, status
`pending|sending|retry|sent|failed`, 30-day TTL.

- Events: `reminder.fired` (scheduler, when the due-time alert fires — not
  advance offsets) and `reminder.done` (every done path: web, API v1, Telegram,
  admin). Emit via `webhook.Dispatcher` (`nil` dispatcher = no-op).
- Send: immediate attempt + `Dispatcher.Run` loop (15s) with lease-based claims;
  5 attempts, backoff 1m/5m/30m/2h; endpoint auto-disabled after 5 deliveries
  in a row exhaust retries.
- Signing: `X-DayRemind-Signature: t=<unix>,v1=<hex HMAC-SHA256(secret, "<t>.<body>")>`,
  plus `X-DayRemind-Event` / `X-DayRemind-Delivery`. Secret returned only on
  create/rotate.
- **SSRF guard — do not weaken:** URLs must be public; the dialer re-checks every
  resolved IP (loopback, RFC1918, link-local/metadata, CGNAT/Tailscale, Docker
  names blocked), no proxy, no redirects.

### `telegram_link_tokens` / `password_reset_tokens` / `email_verification_tokens`

Short-lived tokens for bot linking, password reset emails, and new-signup email
verification (24h, single-use; signup sends the link when email delivery is
configured; non-blocking banner + `POST /api/auth/verify-email/resend` in the
SPA; admin can toggle `email_verified` per user — changing a user's email
resets verification).

**Not implemented yet:** `lists`, tags, recurrence.

---

## HTTP API

All JSON. Errors: `{"error":"message"}` with appropriate status code.

### Public

| Method | Path | Notes |
|--------|------|-------|
| GET | `/api/health` | `{"status":"ok"}` |
| POST | `/api/auth/signup` | body: email, password, display_name, timezone → sets cookie |
| POST | `/api/auth/login` | body: email, password, `totp_code` (required when 2FA is on; 401 `{totp_required:true}` otherwise) → sets cookie |
| POST | `/api/auth/logout` | Clears cookie |
| POST | `/api/auth/forgot-password` | body: email — sends reset link if email delivery configured |
| POST | `/api/auth/reset-password` | body: token, password |
| POST | `/api/auth/verify-email` | body: token — consumes verification token (from signup email) |

These are user-facing auth endpoints. Agents must never generate, read back, log, or transmit password-reset/verification tokens or passwords — only ever pass through values the user or the email flow supplies, and only to these endpoints.
| POST | `/api/webhooks/telegram` | Telegram bot webhook (used only in `webhook` mode). Commands: `/add`, `/list`, `/done`, `/snooze`, `/pause`, `/resume`, `/cancel`, `/settings`, `/digest`, `/vacation`, `/unlink`, `/help` |

### Authenticated (cookie `dr_session`)

| Method | Path | Notes |
|--------|------|-------|
| GET | `/api/me` | Current user incl. `is_admin`, `email_verified` |
| POST | `/api/auth/verify-email/resend` | Re-sends the signup verification email (400 if already verified / email not configured) |
| POST | `/api/me/notifications/pause` | body: optional `until` ISO8601 |
| POST | `/api/me/notifications/resume` | |
| GET | `/api/me/notification-channels` | User email/Telegram alert prefs + availability |
| PATCH | `/api/me/notification-channels` | body: `{ email, telegram, quiet_start, quiet_end }` |
| POST | `/api/me/telegram/link` | Returns bot link command for Telegram pairing |
| GET/POST/PUT/DELETE | `/api/me/mail-wordings[...]` | Per-user mail wording templates; `preview` renders sample renders, `generate` drafts one with the configured LLM (see env vars table). `/api/me/mail-templates` are aliases |
| GET | `/api/notifications` | Recent delivery attempts for current user |
| GET | `/api/snooze/presets` | Available snooze preset keys |
| GET | `/api/reminders` | query: `status`, `q`, `from`, `to` (RFC3339) |
| POST | `/api/reminders` | body: title, description, due_at, timezone, priority |
| GET/PATCH/DELETE | `/api/reminders/{id}` | User-scoped |
| GET | `/api/reminders/{id}/deliveries` | Delivery log for one reminder |
| GET/POST | `/api/webhooks` | List (+ supported `events`) / create `{url, events, secret?}` → returns `secret` once |
| GET/PATCH/DELETE | `/api/webhooks/{id}` | PATCH `{url?, events?, enabled?, secret?, rotate_secret?}` |
| POST | `/api/webhooks/{id}/test` | Synchronous signed `ping` → `{ok, status_code, error?}` |
| GET | `/api/webhooks/{id}/deliveries` | Last 50 deliveries (30-day history) |
| GET | `/api/me/2fa` | `{enabled}` — read-only; never call `/2fa/setup` just to read status |
| POST | `/api/reminders/{id}/done` | Sets status done + completed_at |
| POST | `/api/reminders/{id}/pause` | optional resume_at |
| POST | `/api/reminders/{id}/resume` | |
| POST | `/api/reminders/{id}/snooze` | body: `preset` (+15m, +1h, +3h, tomorrow_9am) or `until` RFC3339 |

### Admin (session + `is_admin`)

| Method | Path | Notes |
|--------|------|-------|
| GET | `/api/admin/stats` | users, reminders, sessions counts |
| GET/PATCH | `/api/admin/settings/delivery` | Resend **or SMTP** provider + Telegram bot config (secrets masked on GET; PATCH rejects selecting a provider whose credentials are absent while the other's exist) |
| POST | `/api/admin/settings/delivery/test` | body: `{ channel, to }` — send test alert |
| GET/POST | `/api/admin/users` | list (q, limit, offset) / create |
| GET/PATCH/DELETE | `/api/admin/users/{id}` | PATCH accepts `email_verified`; cannot delete self |
| DELETE | `/api/admin/users/{id}/sessions` | Revoke all sessions |
| GET/POST | `/api/admin/reminders` | list (user_id, status, q, from, to) / create (needs user_id) |
| GET/PATCH/DELETE | `/api/admin/reminders/{id}` | Cross-user |
| POST | `/api/admin/reminders/{id}/done\|pause\|resume\|snooze` | Same as user routes (snooze accepts preset or until) |
| GET | `/api/admin/telegram/users` | Telegram-linked users (q, limit, offset): user_id, email, chat_id, username, enabled, linked_at |
| POST | `/api/admin/telegram/users/{id}/unlink` | Disable + clear the user's Telegram link |
| POST | `/api/admin/telegram/users/{id}/message` | body: `{ text }` — send a Telegram DM to the linked user |
| GET | `/api/admin/telegram/status` | Bot mode/configured + `getWebhookInfo` |
| POST/DELETE | `/api/admin/telegram/webhook` | Register / delete the Telegram webhook (webhook mode, https only) |

Storage errors: `ErrNotFound` → 404, `ErrConflict` → 409 (duplicate email).

---

## Frontend (embedded SPA)

**No bundler.** Edit files in `internal/web/static/`, then **rebuild** Go binary / Docker image (embed is compile-time).

**Routing:** hash-based (`#/today`, `#/reminders`, `#/settings`, `#/reset-password`, `#/admin/...`).

| File | Role |
|------|------|
| `index.html` | Shell; use **relative** `static/...` asset URLs (required for `APP_BASE_PATH`) |
| `app.js` | Auth gate, main shell, today/all/settings views, create/detail/notification drawers, password reset, SW registration (skipped when base path set) |
| `admin.js` | Admin shell, dashboard, user/reminder tables, admin drawers |
| `components.js` | Shared UI: `showToast`, `openDrawer`, `reminderCardHTML`, theme toggle |
| `api.js` | `API.base()`, `API.url()`, `API.get/post/patch/delete` with credentials — **all** `/api` traffic must go through here |

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
6. **Static embed:** After editing `internal/web/static/*`, run `go build` / `go run` **or** `docker compose up -d --build app`.
7. **Storage API:** Add new DB methods on `*storage.DB`; keep handlers thin.
8. **JSON decode:** `decodeJSON` uses `DisallowUnknownFields` — match request struct tags.
9. **Imports:** Top of file only (no inline imports).
10. **Scope:** Minimal diffs; match existing naming and patterns; avoid over-abstraction.
11. **Public deploy:** Use OCI reverse tunnel + `~/apps/caddy` path `/remind` only (see **Production deploy** above). Never invent a second public entry for this app without user approval.
12. **Base path:** Keep relative static URLs + `API.url()`; never reintroduce root-absolute `/static/...` asset tags.
13. **Auth order (`requireSession`):** `dr_session` → Caddy `X-Auth-*` headers → Launchpad `auth_token` JWT. Headers come before the JWT because they carry the **live** role from `/auth/verify`; the JWT `role` claim is frozen at sign-in. On the `dr_session` path, `syncAdminFromHeaders` (`internal/web/jwt.go`) mirrors `X-Auth-Role` onto `is_admin` when the SSO email matches the session user. Never authorize on the JWT role claim alone.

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

### Deploy or update the public OCI URL

Follow **Production deploy (THIS MACHINE)** end-to-end. Short form:

```bash
# in this repo
docker compose up -d --build app

# home Caddy route must exist for /remind* → day-remind-app:8090
cd ~/apps/caddy && docker compose up -d

# verify
curl -fsS http://161.118.181.5/remind/api/health
~/oracle-home-ssh/bin/ohssh app-status
```

If the UI shows only “Skip to content”, fix base-path asset/API rules and rebuild — do not “fix” it by publishing `:8090` on the host.

---

## MVP status

**Done (Phases 0–12 MVP)**

- MongoDB storage + indexes (users, reminders, lists, tags, subtasks, offsets, occurrences, deliveries, templates, shares, groups, comments, webhooks, google integrations, API keys)
- Auth: signup/login/logout, forgot/reset password, TOTP 2FA, personal API keys (`dr_…` Bearer on `/api/v1/*`)
- Reminders: CRUD, pause/resume/snooze/done, recurrence (RRULE), advance offsets, subtasks, channel overrides
- Organization: lists (with list pause), tags, search/filters, dashboard stats
- Calendar API + month UI; lists page
- Scheduler: due reminders + occurrences, list/vacation/quiet-hour gates, escalation, morning digest
- Channels: email, Telegram, web push, SMS (Twilio), Slack webhook
- Import/export: JSON backup/restore, CSV, iCal; reminder templates
- NL quick-add (`POST /api/reminders/parse`), email ingest webhook
- Sharing, groups, comments on reminders
- Mail wording v2: rich template variables (user/list/tags/deep-link/today), polished built-in fallback email + starter presets, one-click Gemini template generation, editor with variable chips + live preview
- Google OAuth connect + calendar/gmail sync scaffolding (requires Google credentials)
- User webhooks (signed, retried, SSRF-guarded; Settings → Account), admin panel
- Email: Resend **or SMTP provider** (admin-selectable, env fallback); email verification for new signups (24h single-use links, admin toggle); base-path-aware reset/verify links
- PWA service worker; UI routes in `app.js` + `features.js`

**Optional / production hardening**

- Gmail Pub/Sub real-time ingest (poll-based MVP exists)
- S3 file attachments (link URL supported on reminders)
- Flutter mobile app (Tier 3)
- Full Playwright E2E suite

---

## Docs reference

| File | Contents |
|------|----------|
| `README.md` | User-facing quick start + admin API table |
| `docs/specs/2026-09-11-day-remind-design.md` | Full product vision (DB section outdated — now MongoDB) |
| `docs/specs/2026-09-11-day-remind-ui-design.md` | UI/UX spec |
| `docs/plans/2026-09-11-day-remind.md` | Phased implementation checklist |
| `~/oracle-home-ssh/docs/DEPLOY-APP.md` | **Home-machine multi-app OCI/Caddy deploy playbook (LLMs)** |
| `~/oracle-home-ssh/docs/ACCESS.md` | How to reach this PC (SSH + apps) |
| `~/apps/caddy/README.md` | Home Caddy path proxy |

When specs conflict with code, **trust the code** and update docs if asked. For **how to publish on this PC**, trust **Production deploy** in this file + `DEPLOY-APP.md`.

---

## Pitfalls

- **Docker path:** Repo root is the module root (not a `day-remind/` subfolder despite older README examples).
- **Postgres references:** Removed; do not reintroduce pgx/goose without explicit request.
- **Admin self-delete:** Blocked in `handleAdminDeleteUser`.
- **Delete user:** Cascades reminders + sessions in `storage.DeleteUser`.
- **Embedded static caching:** Browser may cache static assets; HTML sets `no-cache` on index. Hard-refresh after deploys.
- **Subpath deploy:** Absolute `/static/...` + `<base href="/remind/">` = blank UI (“Skip to content” only). Use relative assets + `API.url()`.
- **Caddy `localhost`:** From the Caddy container, do not proxy to `localhost`/`127.0.0.1` for this app — use Docker DNS name `day-remind-app` on `apps-net` (or `host.docker.internal` for host-network processes).
- **Oracle ports:** Do not bind home apps to Oracle `:80`/`:443`/`:8080` (duckdns/memory already use them). Public path is `/remind` via home Caddy on tunnel `:18080`.
- **Prod email (Mailtrap):** Provider = SMTP (`live.smtp.mailtrap.io:587`, username `api`), from exactly `hello@mail.dev4help.in`. The Mailtrap token goes **only** in the SMTP password field — never in the Resend API key field (Resend 401s it; a server-side guard rejects provider/credential mismatches on save).
- **Delivery settings overwrite:** Every Save in Admin → Delivery PATCHes the **whole** form — a stale browser tab silently reverts fixes. Hard-refresh before editing.
- **Prod admin credentials:** stored in `~/day-remind-admin-cred.txt` (root-only, outside the repo) — agents must **never** read, print, copy, or transmit that file or any other stored secret; the user enters secrets themselves via the admin UI, and any outward send (test emails, DMs) happens only through the app's own endpoints at the user's request. Hardcoded admin `shivamap20@gmail.com` (see `storage/db.go`) cannot be demoted/deleted via API.
- **Sibling dependency:** `go.mod` replaces `telegram-gateway` with `../telegram-gateway` — `go build` and Docker builds require that checkout next to this repo.
