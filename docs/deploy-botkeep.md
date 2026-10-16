# Deploy on Botkeep / Pterodactyl

## Why `no space left on device` during `go build`

Many eggs use a **Go SDK image** and run `go build` on every start. That writes gigabytes under `/tmp/go-build*` and the server disk quota fills up. The daemon reports **Out of memory: false** because this is **disk**, not RAM.

**Fix:** run a **pre-built binary** (slim Docker image). Do not compile on the node.

## Recommended: GHCR image (after CI publish)

1. Push to `main` or `feat/delivery-config-and-templates` — workflow **Publish container image** pushes:
   - `ghcr.io/shivam-20/day-remind:latest`
   - `ghcr.io/shivam-20/day-remind:<git-sha>`

2. In the Pterodactyl egg / Botkeep server settings:
   - **Docker image:** `ghcr.io/shivam-20/day-remind:latest`
   - **Startup command:** `/app/day-remind`
   - **Install script:** leave empty or only `chmod +x` — **no `go build`**

3. **Environment variables** (panel → Variables), not a baked `.env`:
   - `DATABASE_URL` — include `/day_remind` in the path
   - `SESSION_KEY` — 32+ characters
   - `WEB_PORT` — **must match** the panel allocation (e.g. `49118`), not `8090`
   - `APP_BASE_PATH` — `/remind` when served at `https://dev4help.in/remind/`
   - Optional: `ADMIN_EMAILS`, `DAY_REMIND_SKIP_DOTENV=1`

4. If the package is private, add a GHCR pull credential on the node.

## Build the image yourself (one machine with disk)

```bash
docker build -t day-remind:latest .
docker save day-remind:latest | ssh your-node docker load
```

Point the egg at your registry tag instead of GHCR.

## If you must compile on the server (not recommended)

- Free disk: remove old Docker layers, `go clean -cache`, clear `/tmp`
- Point caches at the server volume:
  ```bash
  export GOCACHE=/home/container/.cache/go-build
  export GOTMPDIR=/home/container/tmp
  mkdir -p "$GOCACHE" "$GOTMPDIR"
  ```
- Still prefer cross-compiling locally:
  ```bash
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o day-remind ./cmd/server
  ```
  Upload `day-remind` and run it from an **Alpine** image with `ca-certificates`.

## MongoDB from Botkeep

Atlas **Network Access** must allow the node’s **outbound IP**. TLS errors often mean allowlist or blocked port `27017`, not slow queries.

## Botkeep Developer API (`botkeep.cloud`, not `botkeeper.cloud`)

- **Base URL:** `https://api.botkeep.cloud`
- **Auth:** `Authorization: Bearer bk_live_…` (create keys in the panel under **Developer → API keys**)
- **OpenAPI:** `GET /api/v1/developer/openapi.json` (same bearer token)

Global Cursor skill: `~/.cursor/skills/botkeep-api/` (agent auto-use when you mention Botkeep / `bk_live`).

Local helpers (key in `~/.botkeep-api-key`, gitignored `.botkeep-api-key`, or `BOTKEEP_API_KEY`):

```bash
chmod +x scripts/botkeep-api.sh scripts/botkeep-tune-day-remind.sh
./scripts/botkeep-api.sh workloads
./scripts/botkeep-api.sh workload 56ba890b-2b74-4e58-911d-61db5985b287
./scripts/botkeep-tune-day-remind.sh   # cached go build, WEB_PORT, DATABASE_URL/db name, restart
./scripts/botkeep-pull-deploy.sh       # pull latest GitHub commit (needs BOTKEEP_SYNC_CONFIRM=server name)
```

## Deploy via SFTP (skip GitHub sync)

1. Botkeep panel → **Go server** → **SFTP** → **Create SFTP credentials** (password shown once; ~1 hour).
2. `cp .botkeep-sftp.env.example .botkeep-sftp.env` and fill host, port, user, password.
3. `brew install lftp` if needed, then:

```bash
chmod +x scripts/botkeep-sftp-deploy.sh
./scripts/botkeep-sftp-deploy.sh
```

Uploads the repo root (excludes `.git`, secrets, `bin/`) and **restarts** the workload. Use when GitHub sync is slow or stuck.

Useful endpoints for Day Remind:

| Action | Method | Path |
|--------|--------|------|
| List servers | GET | `/api/v1/developer/workloads` |
| Startup / branch | PUT | `/api/v1/developer/workloads/{id}/settings` |
| Env var | PUT | `/api/v1/developer/workloads/{id}/environment` |
| GitHub pull | POST | `/api/v1/developer/workloads/{id}/github/sync` |
| Restart | POST | `/api/v1/developer/workloads/{id}/actions` body `{"action":"restart"}` |

Mutating calls need header `Idempotency-Key` (16–64 alphanumeric/`_-`). The helper script generates one.

**Security:** Rotate any `bk_live_` key pasted into chat. Never commit API keys; `GET …/environment` can return variable values—treat the key like a password.
