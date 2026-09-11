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
│   ├── config/
│   ├── reminder/      # domain types
│   ├── storage/       # postgres + migrations
│   └── web/           # HTTP API + embedded static/
└── docker-compose.yml
```

## Rules

- Timestamps: UTC in DB; display in user timezone in UI.
- Static UI is `//go:embed` — rebuild after `internal/web/static/` edits.
- Session cookie: `dr_session`, HttpOnly.
- Reminder status: `scheduled` | `paused` | `done` | `cancelled`.
