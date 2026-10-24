#!/usr/bin/env bash
# Upload only files that differ from origin/HEAD (or listed paths) via Botkeep files API.
# Avoids a 150+ file GitHub tree apply. Requires files:write on the API key.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
API="$ROOT/scripts/botkeep-api.sh"
WL_ID="${BOTKEEP_WORKLOAD_ID:-e7527a22-2987-4019-bbcb-cdaafc88d2c1}"
cd "$ROOT"

if [[ $# -gt 0 ]]; then
  files=("$@")
else
  mapfile -t files < <(git diff --name-only origin/feat/delivery-config-and-templates...HEAD -- \
    ':!.github' ':!docs' ':!scripts' ':!AGENTS.md' ':!CLAUDE.md' ':!.gitignore' \
    ':!.botkeep-sftp.env.example' ':!docs/**' 2>/dev/null)
  if [[ ${#files[@]} -eq 0 ]]; then
    files=(
      internal/web/static/index.html
      internal/web/static/js/api.js
      internal/web/server.go
      internal/config/config.go
    )
  fi
fi

echo "Uploading ${#files[@]} file(s) to workload $WL_ID (not a full GitHub sync)"
for f in "${files[@]}"; do
  [[ -f "$f" ]] || continue
  echo "  $f"
  "$API" file-put "$WL_ID" "/$f" "$ROOT/$f" >/dev/null
done

echo "Removing stale binary so next start rebuilds once …"
"$API" files-delete "$WL_ID" "/day-remind" >/dev/null || true
echo "Restarting …"
"$API" action "$WL_ID" restart
echo "Done. Site should come up on the allocated WEB_PORT without copying the whole repo."
