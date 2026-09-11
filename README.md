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

## Development

```bash
go test ./...
go build -o bin/day-remind ./cmd/server
```

MongoDB runs on port **27017** (see `docker-compose.yml`).

Set `ADMIN_EMAILS` in `.env` (comma-separated) to grant admin access. Admins see an **Admin** link in the sidebar and can open `#/admin` for the management panel.

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

## MVP status (this branch)

- [x] MongoDB collections + indexes
- [x] Auth (signup, login, session cookie)
- [x] Reminder CRUD + pause/resume/snooze/done
- [x] Vacation mode (pause all notifications)
- [x] Embedded web UI (Today, All, Settings, create drawer)
- [ ] Email/Telegram delivery (Phase 2)
- [ ] JSON import/export UI (Phase 8)
- [ ] Google/Gmail sync (Phase 11)
