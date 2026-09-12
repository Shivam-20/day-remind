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

**Public deploy on this home PC** (OCI reverse tunnel + Caddy `/remind`): see **`AGENTS.md` → Production deploy**. Generic playbook: `~/oracle-home-ssh/docs/DEPLOY-APP.md`.

## Development

```bash
go test ./...
go build -o bin/day-remind ./cmd/server
```

MongoDB runs on port **27017** (see `docker-compose.yml`).

Set `ADMIN_EMAILS` in `.env` (comma-separated) to grant admin access. Admins see an **Admin** link in the sidebar and can open `#/admin` for the management panel.

## Admin panel UI

| Route | Features |
|-------|----------|
| `#/admin` | Dashboard stats |
| `#/admin/users` | Search, create, edit, delete users; revoke sessions |
| `#/admin/reminders` | Filter, create, done/pause/resume/delete across all users |

## Admin API (session + admin role)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/admin/stats` | User/reminder/session counts |
| GET/POST | `/api/admin/users` | List / create users |
| GET/PATCH/DELETE | `/api/admin/users/{id}` | Get / update / delete user |
| DELETE | `/api/admin/users/{id}/sessions` | Revoke all sessions |
| GET/POST | `/api/admin/reminders` | List / create reminders (any user) |
| GET/PATCH/DELETE | `/api/admin/reminders/{id}` | Get / update / delete reminder |
| POST | `/api/admin/reminders/{id}/done` | Mark done |
| POST | `/api/admin/reminders/{id}/pause` | Pause |
| POST | `/api/admin/reminders/{id}/resume` | Resume |
| POST | `/api/admin/reminders/{id}/snooze` | Snooze |

## Docs

- Product design: [`docs/specs/2026-09-11-day-remind-design.md`](docs/specs/2026-09-11-day-remind-design.md)
- UI design: [`docs/specs/2026-09-11-day-remind-ui-design.md`](docs/specs/2026-09-11-day-remind-ui-design.md)
- Implementation plan: [`docs/plans/2026-09-11-day-remind.md`](docs/plans/2026-09-11-day-remind.md)

## MVP status

- [x] MongoDB + auth + reminders + scheduler + delivery
- [x] Lists, tags, search, dashboard stats
- [x] Calendar view + subtasks
- [x] Recurrence (RRULE) + advance alert offsets
- [x] Extended channels (web push, SMS, Slack)
- [x] JSON/CSV/iCal import & export + templates + API keys
- [x] NL parse, email ingest, morning digest, escalation
- [x] Sharing, groups, comments
- [x] Google OAuth + calendar/Gmail sync (requires credentials)
- [x] Webhooks, 2FA, plan limits, admin panel
