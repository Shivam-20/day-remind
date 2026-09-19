# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
docker compose up -d                       # MongoDB on :27017, MailHog on :1025/:8025
cp .env.example .env                       # then set SESSION_KEY (32+ chars) and DATABASE_URL
go run ./cmd/server                        # serves http://localhost:8090
go build -o bin/day-remind ./cmd/server
go test ./...
go test ./internal/storage -run TestUserReminderFlow -v   # single test
```

`go vet ./...` and `gofmt` are the only linting; there is no golangci-lint config or Makefile.

**Every database-backed test creates its own database** (`day_remind_test_<uuid>`, dropped on cleanup). Two reasons: `scheduler.Tick` sweeps every notifiable user, so a shared database would make tests see each other's reminders and whatever is already in yours. The instance settings document is a singleton, so a test that clears it would otherwise wipe the delivery configuration of whatever database `DATABASE_URL` points at, including a running dev instance. **Never write a test that touches `settings` or sweeps globally against a shared database.** Scheduler tests swap in fake senders through `Scheduler.overrideSenders`, which keeps the resolver's real precedence logic in play while nothing touches the network. The SMTP tests run a real in-process SMTP server and the bot tests a stubbed Telegram API, so **no test ever sends anything off the machine**.

Storage tests **skip silently** unless `DATABASE_URL` is set — they hit a real MongoDB, they are not mocked, and they write to the configured database. Run them against the docker-compose instance, not Atlas. CI (`.github/workflows/ci.yml`) spins up `mongo:7` as a service and exports `DATABASE_URL`/`SESSION_KEY` before `go test ./...`.

## Architecture

Single Go binary (module `github.com/shivam-20/day-remind`, Go 1.22) that serves both the JSON API and the embedded SPA. No build step for the frontend — no npm, no bundler, no framework.

**Wiring:** `cmd/server/main.go` loads `.env` via godotenv → `config.Load()` (fails fast if `DATABASE_URL` missing or `SESSION_KEY` < 32 chars) → `storage.Open()` (connects, pings, creates indexes) → `web.New(cfg, db).ListenAndServe()`.

**Layers** (strict one-way dependency: `web` → `storage` → `reminder`):

- `internal/reminder` — domain types and status/priority constants only. No I/O, no Mongo imports. Handlers and storage both depend on it.
- `internal/storage` — the whole data layer. `DB` wraps a `*mongo.Database`; every collection accessor goes through `db.collection(name)`. Each file owns one collection (`users.go`, `sessions.go`, `reminders.go`) and defines a private `xDoc` struct with `bson` tags plus a `xFromDoc` mapper. **The `bson` doc structs are the schema** — there are no migrations, so changing a field name there silently changes the on-disk shape.
- `internal/web` — chi router, handlers, and the embedded UI. Handlers never touch Mongo directly and never return domain structs; they build `map[string]any` payloads (`reminderPayload`, `userPayload`) so the JSON wire format is defined in one place per resource.
- `internal/auth` — bcrypt hashing (cost 12), enforces the 8-char minimum password inside `HashPassword`.
- `internal/config` — env parsing and validation.
- `internal/notify` — channel-agnostic `Message`/template rendering plus the two `Sender` implementations (`SMTPSender`, `TelegramClient`). **Knows nothing about storage or HTTP** — keep it that way; `internal/delivery` is the bridge.
- `internal/secret` — AES-GCM for the credentials stored in the database.
- `internal/delivery` — resolves instance vs per-user senders and merges templates.
- `internal/scheduler` — the sweep loop that turns due reminders into deliveries.
- `internal/bot` — the Telegram command bot (long-polling) and the natural-language time parser used by `/add` and `/snooze`.

**Multi-tenancy is enforced at the query level.** Every reminder query filters on `{_id, user_id}` together; there is no separate ownership check. A missing `user_id` in a new query is a cross-tenant data leak, so keep the pattern when adding storage methods. `userID` reaches handlers only via `userIDFromContext(r.Context())`, populated by the `requireSession` middleware.

**Error translation:** storage returns sentinel errors — `storage.ErrNotFound`, `storage.ErrConflict`, `storage.ErrInvalid` — and validation failures are wrapped with `%w: ...` around `ErrInvalid`. Handlers funnel every storage error through `writeStorageError` (`internal/web/reminders.go`), which maps them to 404/409/400 and everything else to a 500 with a generic message, so driver errors are never echoed to the caller. Use `errors.Is`, not `==`, since these are wrapped. A 0 `MatchedCount`/`DeletedCount` from Mongo is translated to `ErrNotFound` rather than treated as success.

**Indexes** live in `internal/storage/indexes.go` and are created on every startup (`CreateMany` is idempotent). Sessions expire via a TTL index on `expires_at` with `expireAfterSeconds: 0`; `GetSessionUserID` also checks expiry in Go and deletes lazily, because Mongo's TTL reaper only runs about once a minute.

**Admin panel.** There are two roles, `user` and `admin` (`storage.RoleUser`/`RoleAdmin`). A **missing** `role` field on an older document reads as `user` via `normalizeRole`, so the field was introduced with no migration — preserve that when touching `userFromDoc`. Admins are bootstrapped by the `ADMIN_EMAILS` env var (comma-separated, case-insensitive): `syncAdminRole` promotes a listed address on signup and login. It only ever *grants*, so removing an address never silently demotes a sitting admin. `requireAdmin` re-reads the role from Mongo on every request rather than trusting the session, and answers **404, not 403**, so the panel's existence is not disclosed to non-admins. Three guards prevent lockout and are enforced server-side, not just in the UI: a pinned (`ADMIN_EMAILS`) admin cannot be demoted, the last remaining admin cannot be demoted or deleted (`guardLastAdmin`), and an admin cannot delete their own account. **Admin views are metadata-only by design** — `adminUserPayload` deliberately carries reminder counts and dates but never a title or description; keep it that way when adding fields. `DeleteUser` cascades to reminders and sessions explicitly, since Mongo has no foreign keys.

**Delivery pipeline.** Configuration lives in MongoDB, not the environment: a singleton `settings/_id="instance"` document (`internal/storage/settings.go`) holds the instance SMTP server, bot token and templates. `SeedInstanceSettings` writes it **only when absent**, so `SMTP_*` and `TELEGRAM_BOT_TOKEN` seed a fresh install and are ignored afterwards — an admin editing the panel must not be overwritten by a restart. `SCHEDULER_INTERVAL`, `TELEGRAM_API_BASE` and `ALLOW_PRIVATE_SMTP` stay env-only; they are infrastructure, not user config.

`internal/delivery` resolves who sends what. `Resolver.Snapshot(ctx)` reads the instance settings **once per sweep**, and `Snapshot.For(user)` returns the `Target`s for that user: **their own configuration first, then the instance's**, per channel. The scheduler and the web server share one resolver, so the "send test notification" button exercises the real path. A channel the server cannot deliver is refused with 409 rather than being silently accepted.

**Credentials are encrypted at rest** by `internal/secret` (AES-256-GCM, key derived from `SESSION_KEY` via HKDF). These are the only reversible secrets in the database — account passwords remain one-way bcrypt hashes and must never go through this package. Two rules: **a credential is never returned by any API** (report `password_set`/`token_set` and a `secret.Hint`, nothing more), and **a decryption failure is never fatal** — it means `SESSION_KEY` changed, so the affected channel degrades to unconfigured and the UI asks for re-entry.

**A user's own Telegram bot is outbound-only.** Telegram permits one `getUpdates` consumer per token, so a poller per user does not scale; commands stay on the instance bot. A chat id also belongs to one bot, so the instance-linked id is not valid for a user's own bot — `handleDiscoverOwnTelegramChat` makes a *single* `getUpdates` call (not a poller) to find it.

**Templates** (`internal/notify/template.go`) are merged **field by field**: built-in default → instance → user, where a blank field falls through. `EmailHTML` renders with `html/template` so reminder content is auto-escaped; Telegram values are pre-escaped for MarkdownV2 because Telegram rejects the whole message otherwise. Email is `multipart/alternative`. **A template failure must never block a notification** — parse errors are rejected at save time, and any render failure falls back to the built-in default and logs. Keep `sanitizeHeader` on every email header: templates widen that input path.

**Exactly-once is enforced by a claim, not by timing.** `reminders.sent_due_at` and `sent_advance_at` are set by `ClaimReminderNotification`, an atomic `UpdateOne` filtered on the field still being nil. The scheduler **claims before it sends**; if the claim returns false, another tick or process already owns that notification. If every channel then fails, `ReleaseReminderNotification` clears the claim so a later sweep retries. Never add a send path that skips the claim — that is what keeps a second server instance from double-delivering. Changing `due_at` or snoozing clears both claims, because those actions mean "notify me again".

`FireTime(r)` — not `r.DueAt` — is the effective moment a reminder fires; a snooze pushes it out. Use it anywhere you compare a reminder against the clock.

**Timezones in notifications** render via `Message.localDue()` using the *reminder's* stored zone, falling back to UTC when the zone is not installed. The bot parses `/add` input relative to the *user's* zone, so "tomorrow 9am" means their tomorrow (`ParseWhen` takes `now` already expressed in that location).

**Telegram** runs long-polling, so it needs no public URL. Every dynamic string in a reply must go through `escape()` (MarkdownV2) or Telegram rejects the whole message — this is the single most common way to break the bot. `/start <code>` redemption is atomic and single-use: the code is cleared in the same update that sets the chat ID, and expiry is enforced in the query filter rather than by a prior read. One chat maps to at most one account.

**Auth flow:** signup/login create a random 32-byte hex token stored as the session document `_id`, set in the `dr_session` cookie (HttpOnly, SameSite=Lax, 30-day MaxAge). `Secure` is set only when the request arrives over TLS or behind `X-Forwarded-Proto: https`, so cookies work over plain HTTP in local dev. `SESSION_KEY` is validated at startup but is not yet used to sign anything.

**Frontend** (`internal/web/static/`) is embedded with `//go:embed all:static`, so **the binary must be rebuilt after editing any static file** — `go run` picks up changes, a built binary does not. Three plain scripts load in order (`api.js` → `components.js` → `app.js`) with no modules; `API`, `showToast`, `openDrawer` are globals. Routing is hash-based (`#/today`, `#/reminders`, `#/settings`, `#/settings/vacation`, `#/admin`) dispatched through the `routes` map in `app.js`; views render by assigning `innerHTML`. All non-`/api/` paths fall through to `index.html` so deep links work. Colors come exclusively from the CSS custom properties in `theme.css` (`--bg`, `--surface`, `--accent`, …) keyed off `data-bs-theme` on `<html>`.

## Conventions

- **All timestamps are stored and serialized as UTC**; `Timezone` is carried per reminder and per user for display only. API request/response times are RFC3339 strings; conversion happens at the handler boundary, never in storage.
- IDs are `uuid.UUID` (v4) generated in Go, used directly as Mongo `_id` — not ObjectIDs.
- Reminder status is `scheduled` | `paused` | `done` | `cancelled`; priority is `low` | `normal` | `high` | `urgent`. Use the constants in `internal/reminder`, never string literals.
- `decodeJSON` sets `DisallowUnknownFields()`, caps the body at 1 MiB, and rejects trailing JSON after the first object, so adding a field to the UI payload without adding it to the corresponding request struct returns 400.
- **`PATCH` request structs use pointer fields** (`updateReminderRequest`) so an omitted field stays `nil` and is left untouched; a non-pointer struct here silently blanks every field the client did not send. `POST` bodies use value fields because create supplies everything.
- `status` and `priority` are validated by `reminder.ValidStatus` / `reminder.ValidPriority` at the storage boundary, so an unknown enum never reaches the database.
- Free-text search runs through `regexp.QuoteMeta` before it becomes a Mongo `$regex` — user input must never be compiled as a pattern.
- A credential field in a request body is a **pointer**: omitted keeps the stored value, `""` clears it. Since the stored value is never sent to the browser, a form that round-trips cannot silently erase it.
- User-supplied SMTP is restricted to ports 25/465/587/2525/1025 and, unless `ALLOW_PRIVATE_SMTP=true`, may not resolve to a private address — otherwise any user can make this process probe the internal network.
- Pause/resume are guarded state transitions in Mongo (`PauseReminder` only matches `scheduled`/`paused`, `ResumeReminder` only matches `paused`), so an invalid transition surfaces as 404, not a silent no-op.
- Default timezone fallback is `Asia/Kolkata` (the `defaultTimezone` constant in `internal/storage/db.go`).
- The admin UI holds its own `adminState` and re-renders rows from cached data while searching or sorting; only an action that mutates data refetches.

## Not built yet

Templates are instance-wide and per-user but **not per event**: one wording covers both the due and the advance notice, differentiated inside the template with `{{if .Advance}}`. Delivery has **no retry backoff and no per-channel retry limit**: a failed send releases the claim and is retried on every subsequent sweep, forever, until it succeeds. A permanently bad address will retry indefinitely and fill the delivery log. There is **no admin audit log** — role changes and user deletions are not recorded anywhere, and deletion is unrecoverable with no backups configured. `resume_at` is stored but unused (pause is manual-resume only). Lists (`list_id`) are accepted and persisted but have no collection, no CRUD, and no UI. See `docs/plans/2026-09-11-day-remind.md` and `docs/specs/2026-09-11-day-remind-design.md` for the intended roadmap; treat the specs as aspirational, not as a description of current behavior.

## Note

`.env.example` previously held a live MongoDB Atlas connection string with real credentials and is tracked in git. The file now carries placeholders, but **the old credential is still in the git history and must be rotated** in Atlas before this repo is published.
