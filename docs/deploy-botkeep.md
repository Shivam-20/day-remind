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
   - Optional: `WEB_PORT`, `ADMIN_EMAILS`, `DAY_REMIND_SKIP_DOTENV=1`

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
