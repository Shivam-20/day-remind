#!/usr/bin/env bash
# Pterodactyl startup: run pre-built binary only (no go build on the node).
set -euo pipefail
cd /app 2>/dev/null || cd "$(dirname "$0")/.."
exec ./day-remind
