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

## Learned User Preferences

- Exercise the live app through an SSH tunnel to the home-server install when local MongoDB or Atlas is unavailable.
- For UI and Message wording changes validated via the tunnel, mirror them in this repo and rebuild the home-server Docker app when the user is testing production.
- Mail wording: named template library with HTML/CSS, Copy AI prompt, Paste AI result (JSON into the form), and View as mail for due and advance notices.

## Learned Workspace Facts

- Production Day Remind runs in Docker on home-server `system04`; public `/remind` is fronted by Caddy on Oracle **`a1-watcher`** (`161.118.181.5`), which strips `/remind` and proxies to **`node9.botkeep.cloud:49118`** — same path for **`https://dev4help.in/remind/`** and **`http://161.118.181.5/remind/`** (other `dev4help.in` paths still use the home reverse tunnel on `127.0.0.1:18080`).
- **`https://dev4help.in/remind/`** needs an explicit **`http://dev4help.in`** block in `/etc/caddy/Caddyfile` (Cloudflare **Flexible SSL** sends HTTP to the origin); HTTPS-only `dev4help.in` caused a **308 redirect loop**. The `dev4help-remind-proxy` Worker was removed — fix Caddy, do not proxy `/remind` with a Worker.
- Oracle/Caddy work: SSH **`ubuntu@161.118.181.5`** port **22**; **`system04` on port 2222** may be refused when reverse-SSH is down — use Oracle for Caddy or restore the tunnel for home-server deploys (keep mail forwards in `oracle-reverse-ssh-mail`, not the core tunnel unit).
- Local access to the home install: `ssh -L 127.0.0.1:18080:127.0.0.1:8080 system04`, then `http://127.0.0.1:18080/remind/` (e.g. `#/admin`, `#/settings/wording`).
- Server checkout path: `/media/system04/4E36DB0524ADCE65/code/day-remind`; after embedded static changes, `docker compose up -d --build app`.
- The tunneled production tree can diverge from this workspace (e.g. `features.js`, `wording-ai.js` / `wording-ui.js`, `mail-wordings` routes); confirm which codebase you are editing before changing wording UI.
