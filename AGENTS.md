# AGENTS.md — day-remind

Go module: `github.com/shivam-20/day-remind` · Entrypoint: `cmd/server/main.go` · Default port: **8090**

## Commands

```bash
cd day-remind
docker compose up -d
cp .env.example .env
go run ./cmd/server
go test ./...
go build -o bin/day-remind ./cmd/server
```

## Layout

```
day-remind/
├── cmd/server/main.go
├── internal/
│   ├── auth/          # password hashing
│   ├── bot/           # telegram command bot (long-poll) + time parsing
│   ├── config/
│   ├── delivery/      # resolves instance vs per-user senders + templates
│   ├── notify/        # message rendering, SMTP + telegram senders
│   ├── reminder/      # domain types
│   ├── scheduler/     # due-reminder sweep and dispatch
│   ├── secret/        # AES-GCM for credentials stored in the DB
│   ├── storage/       # mongodb + indexes
│   └── web/           # HTTP API + embedded static/
└── docker-compose.yml
```

## Rules

- Timestamps: UTC in DB; display in user timezone in UI.
- Static UI is `//go:embed` — rebuild after `internal/web/static/` edits.
- Session cookie: `dr_session`, HttpOnly.
- Reminder status: `scheduled` | `paused` | `done` | `cancelled`.
- Roles: `user` | `admin`; a missing `role` field reads as `user`. Bootstrap admins with `ADMIN_EMAILS`.
- Admin views are metadata-only — never expose reminder titles or descriptions to another user.
- Delivery is claim-based: `sent_due_at` / `sent_advance_at` are set atomically before sending. Never send without claiming first.
- Changing `due_at` or snoozing clears those claims so the reminder re-arms.
- Telegram replies are MarkdownV2 — run every dynamic string through `escape()` or the API rejects the message.
- Delivery config lives in the DB (`settings/_id=instance`); env only seeds a fresh install.
- Credentials are encrypted via `internal/secret` and must never appear in an API response.
- A broken template falls back to the built-in default — never drop a notification over wording.
