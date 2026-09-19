# Day Remind

**Standalone repository** — not part of [expenseTrackerLLM](https://github.com/Shivam-20/expenseTrackerLLM).

Multi-channel reminder app — create dated reminders, pause when needed, and get notified your way.

## Publish to GitHub (one-time)

Create an empty repo on GitHub named `day-remind`, then:

```bash
git remote add origin https://github.com/Shivam-20/day-remind.git
git push -u origin main
```

## Quick start

```bash
cd day-remind
cp .env.example .env   # edit SESSION_KEY (32+ chars)
docker compose up -d
go run ./cmd/server
```

Open http://localhost:8090

### Admin access

Set `ADMIN_EMAILS` in `.env` to a comma-separated list of addresses, then sign up
or log in with one of them — the role is applied on the way in and an **Admin**
link appears in the sidebar.

```bash
ADMIN_EMAILS=you@example.com
```

Admins see instance-wide stats and can manage users (promote/demote, pause
notifications, delete). They see reminder **counts only** — titles and
descriptions are never exposed. The last remaining admin cannot be demoted or
deleted, and an address listed in `ADMIN_EMAILS` cannot be demoted from the UI.

### Notifications

Reminders are delivered by an in-process scheduler that sweeps every
`SCHEDULER_INTERVAL` (default 30s). Two channels are available; each user turns
them on per account under **Settings → Notifications**, and can also pick an
advance warning (e.g. "1 hour before") on top of the due-time notification.

**Delivery is configured in the database, not the environment.** An admin sets
the instance mail server and bot under **Admin → Delivery**; changes take effect
within seconds, with no restart. The `SMTP_*` and `TELEGRAM_BOT_TOKEN` variables
only *seed* a fresh install — once the settings document exists they are
ignored. Credentials are encrypted at rest with a key derived from
`SESSION_KEY`, and are never returned by any API.

> Changing `SESSION_KEY` makes every stored credential unreadable. The admin
> panel flags them so they can be re-entered, but there is no way to recover
> them.

**Email (SMTP).** For local development, `docker compose up -d mailhog` gives
you a catcher that keeps all mail on your machine — read it at
<http://localhost:8025> (host `localhost`, port `1025`, encryption `none`, and
set `ALLOW_PRIVATE_SMTP=true` so a private address is permitted).

**Telegram.** Create a bot with [@BotFather](https://t.me/botfather) and paste
the token into the admin panel. The bot long-polls, so no public URL or webhook
is needed. To connect an account: Settings → Notifications → *Get link code*,
then send `/start <code>` to the bot.

Bot commands:

| Command | Does |
|---|---|
| `/start <code>` | Link this chat to your account |
| `/list` | Your next 10 reminders, numbered |
| `/add <when> \| <title>` | Create one — `tomorrow 9am`, `in 3h`, `2026-12-25 08:00`, `25 dec` |
| `/done <n>` | Complete the reminder at that number |
| `/snooze <n> <duration>` | Push it back — `30m`, `2h`, `1d` |
| `/unlink` | Disconnect this chat |
| `/help` | Command list |

Notifications also carry inline **Done / +1h / Tomorrow 9am** buttons.

### Bring your own server

Any user may route their own reminders through their own mail server or
Telegram bot, under **Settings → Notifications**. Their configuration takes
precedence over the instance's; anything they leave unset falls back to it.

A user's own bot is **outbound only** — commands still go to the shared bot,
because Telegram allows one `getUpdates` consumer per token. Since a chat id
belongs to a single bot, they send `/start` to their own bot and press *Find my
chat* to connect it.

### Message templates

**Admin → Templates** edits the email subject, HTML body, plain-text body and
Telegram message for the whole instance, with a live preview and a reset. Each
user can override any of those fields for themselves under **Settings → Message
wording**; a field left blank falls through to the instance, then to the
built-in default.

Email is sent as `multipart/alternative`, so clients that refuse HTML still get
a readable message. Reminder content is escaped automatically — HTML for email,
MarkdownV2 for Telegram. A template that fails to render never blocks a
notification: the built-in default is used and the problem is logged.

Use **Send test notification** to verify a channel before relying on it.
Delivery attempts are recorded for 30 days and shown under Admin → Delivery.

## Development

```bash
go test ./...
go build -o bin/day-remind ./cmd/server
```

MongoDB runs on port **27017** (see `docker-compose.yml`).

## Docs

- Product design: [`docs/specs/2026-09-11-day-remind-design.md`](docs/specs/2026-09-11-day-remind-design.md)
- UI design: [`docs/specs/2026-09-11-day-remind-ui-design.md`](docs/specs/2026-09-11-day-remind-ui-design.md)
- Implementation plan: [`docs/plans/2026-09-11-day-remind.md`](docs/plans/2026-09-11-day-remind.md)

## MVP status (this branch)

- [x] MongoDB collections + indexes
- [x] Auth (signup, login, session cookie)
- [x] Reminder CRUD + pause/resume/snooze/done
- [x] Vacation mode (pause all notifications)
- [x] Embedded web UI (Today, All, Settings, create drawer)
- [x] Admin panel (instance stats, user management, roles)
- [x] DB-backed delivery config, per-user SMTP/bot, editable templates
- [x] Email (SMTP) + Telegram delivery, scheduler, advance warnings
- [x] Telegram bot commands (/add, /list, /done, /snooze)
- [ ] JSON import/export UI (Phase 8)
- [ ] Google/Gmail sync (Phase 11)
