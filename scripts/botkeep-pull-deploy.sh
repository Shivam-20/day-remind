#!/usr/bin/env bash
# Pull latest GitHub revision on Botkeep and restart (Developer API).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
API="$ROOT/scripts/botkeep-api.sh"
WL_ID="${BOTKEEP_WORKLOAD_ID:-56ba890b-2b74-4e58-911d-61db5985b287}"
REPO="${BOTKEEP_REPO:-Shivam-20/day-remind}"
BRANCH="${BOTKEEP_BRANCH:-feat/delivery-config-and-templates}"
CONFIRM="${BOTKEEP_SYNC_CONFIRM:-Go server}"

rev="$("$API" github-get "$WL_ID" | python3 -c "import json,sys; print(json.load(sys.stdin)['revision'])")"
echo "Syncing $REPO @ $BRANCH (revision lock ${rev:0:12}…) …"
export BOTKEEP_SYNC_CONFIRM="$CONFIRM"
"$API" github-sync "$WL_ID" "$REPO" "$rev" "$BRANCH"
echo "Queued. Watch Botkeep GitHub / Console for applying → running."
