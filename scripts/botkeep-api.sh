#!/usr/bin/env bash
# Botkeep Developer API helper (https://api.botkeep.cloud).
# Auth: Bearer bk_live_* from panel → Developer → API keys.
# Set BOTKEEP_API_KEY or put the key in .botkeep-api-key (gitignored).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASE="${BOTKEEP_API_BASE:-https://api.botkeep.cloud}"

if [[ -z "${BOTKEEP_API_KEY:-}" ]]; then
  if [[ -f "$ROOT/.botkeep-api-key" ]]; then
    BOTKEEP_API_KEY="$(tr -d '[:space:]' <"$ROOT/.botkeep-api-key")"
  elif [[ -f "${HOME}/.botkeep-api-key" ]]; then
    BOTKEEP_API_KEY="$(tr -d '[:space:]' <"${HOME}/.botkeep-api-key")"
  fi
fi
if [[ -z "${BOTKEEP_API_KEY:-}" ]]; then
  echo "Set BOTKEEP_API_KEY, $ROOT/.botkeep-api-key, or ~/.botkeep-api-key" >&2
  exit 1
fi

auth=( -H "Authorization: Bearer ${BOTKEEP_API_KEY}" -H "Accept: application/json" )

idem() {
  # 16–64 chars for Idempotency-Key
  if [[ -n "${BOTKEEP_IDEMPOTENCY_KEY:-}" ]]; then
    echo "$BOTKEEP_IDEMPOTENCY_KEY"
  else
    uuidgen 2>/dev/null | tr -d '-' || date +%s%N
  fi
}

cmd="${1:-}"
shift || true

case "$cmd" in
  openapi)
    curl -sS "${auth[@]}" "$BASE/api/v1/developer/openapi.json"
    ;;
  workloads)
    curl -sS "${auth[@]}" "$BASE/api/v1/developer/workloads?limit=${1:-50}"
    ;;
  workload)
    id="${1:?workload id}"
    curl -sS "${auth[@]}" "$BASE/api/v1/developer/workloads/$id"
    ;;
  env)
    id="${1:?workload id}"
    curl -sS "${auth[@]}" "$BASE/api/v1/developer/workloads/$id/environment"
    ;;
  capabilities)
    curl -sS "${auth[@]}" "$BASE/api/v1/developer/capabilities"
    ;;
  plan)
    curl -sS "${auth[@]}" "$BASE/api/v1/developer/plan"
    ;;
  network)
    id="${1:?workload id}"
    curl -sS "${auth[@]}" "$BASE/api/v1/developer/workloads/$id/network"
    ;;
  action)
    id="${1:?workload id}"
    act="${2:?start|stop|restart|redeploy|...}"
    key="$(idem)"
    curl -sS "${auth[@]}" -X POST \
      -H "Content-Type: application/json" \
      -H "Idempotency-Key: $key" \
      -d "{\"action\":\"$act\"}" \
      "$BASE/api/v1/developer/workloads/$id/actions"
    ;;
  settings)
    id="${1:?workload id}"
    body="${2:?json body}"
    key="$(idem)"
    curl -sS "${auth[@]}" -X PUT \
      -H "Content-Type: application/json" \
      -H "Idempotency-Key: $key" \
      -d "$body" \
      "$BASE/api/v1/developer/workloads/$id/settings"
    ;;
  env-put)
    id="${1:?workload id}"
    body="${2:?json EnvironmentInput}"
    key="$(idem)"
    curl -sS "${auth[@]}" -X PUT \
      -H "Content-Type: application/json" \
      -H "Idempotency-Key: $key" \
      -d "$body" \
      "$BASE/api/v1/developer/workloads/$id/environment"
    ;;
  github-autopull)
    id="${1:?workload id}"
    on="${2:-false}"
    rev="${3:?64-char revision from github-get}"
    key="$(idem)"
    curl -sS "${auth[@]}" -X PATCH \
      -H "Content-Type: application/json" \
      -H "Idempotency-Key: $key" \
      -d "$(REV="$rev" ON="$on" python3 -c 'import json,os; print(json.dumps({"revision":os.environ["REV"],"autoPull":os.environ["ON"].lower() in ("1","true","yes")}))')" \
      "$BASE/api/v1/developer/workloads/$id/github"
    ;;
  github-inspect)
    id="${1:?workload id}"
    repo="${2:?owner/repo}"
    branch="${3:-}"
    key="$(idem)"
    curl -sS "${auth[@]}" -X POST \
      -H "Content-Type: application/json" \
      -H "Idempotency-Key: $key" \
      -d "$(REPO="$repo" BR="$branch" python3 -c 'import json,os; print(json.dumps({"repository":os.environ["REPO"],"branch":os.environ["BR"]}))')" \
      "$BASE/api/v1/developer/workloads/$id/github/inspect"
    ;;
  github-get)
    id="${1:?workload id}"
    curl -sS "${auth[@]}" "$BASE/api/v1/developer/workloads/$id/github"
    ;;
  github-recover)
    id="${1:?workload id}"
    act="${2:-resume}"
    confirm="${3:-Go server}"
    key="$(idem)"
    curl -sS "${auth[@]}" -X POST \
      -H "Content-Type: application/json" \
      -H "Idempotency-Key: $key" \
      -d "$(ACT="$act" CF="$confirm" python3 -c 'import json,os; print(json.dumps({"action":os.environ["ACT"],"confirmation":os.environ["CF"]}))')" \
      "$BASE/api/v1/developer/workloads/$id/github/recover"
    ;;
  console-cmd)
    id="${1:?workload id}"
    cmd="${2:?command}"
    key="$(idem)"
    curl -sS "${auth[@]}" -X POST \
      -H "Content-Type: application/json" \
      -H "Idempotency-Key: $key" \
      -d "$(CMD="$cmd" python3 -c 'import json,os; print(json.dumps({"command":os.environ["CMD"]}))')" \
      "$BASE/api/v1/developer/workloads/$id/console/command"
    ;;
  files)
    id="${1:?workload id}"
    dir="${2:-/}"
    curl -sS "${auth[@]}" "$BASE/api/v1/developer/workloads/$id/files?directory=$(python3 -c 'import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1], safe="/"))' "$dir")"
    ;;
  file-put)
    id="${1:?workload id}"
    path="${2:?remote path}"
    local="${3:?local file}"
    key="$(idem)"
    body="$(PATH_R="$path" LOCAL="$local" python3 -c '
import json, os, base64
p = os.environ["PATH_R"]
raw = open(os.environ["LOCAL"], "rb").read()
print(json.dumps({"path": p, "data": base64.b64encode(raw).decode("ascii"), "overwrite": True}))
')"
    curl -sS "${auth[@]}" -X POST \
      -H "Content-Type: application/json" \
      -H "Idempotency-Key: $key" \
      -d "$body" \
      "$BASE/api/v1/developer/workloads/$id/files/upload"
    ;;
  github-sync)
    id="${1:?workload id}"
    repo="${2:?owner/repo}"
    rev="${3:?64-char revision}"
    branch="${4:-main}"
    req="$(idem)"
    key="$(idem)"
    confirm="${BOTKEEP_SYNC_CONFIRM:-sync}"
    body="$(REPO="$repo" BR="$branch" REV="$rev" REQ="$req" CF="$confirm" python3 -c 'import json,os; print(json.dumps({"repository":os.environ["REPO"],"branch":os.environ["BR"],"revision":os.environ["REV"],"confirmation":os.environ["CF"],"requestId":os.environ["REQ"],"restart":True}))')"
    curl -sS "${auth[@]}" -X POST \
      -H "Content-Type: application/json" \
      -H "Idempotency-Key: $key" \
      -d "$body" \
      "$BASE/api/v1/developer/workloads/$id/github/sync"
    ;;
  *)
    cat <<'EOF'
Usage:
  botkeep-api.sh openapi
  botkeep-api.sh workloads [limit]
  botkeep-api.sh workload <workload_id>
  botkeep-api.sh env <workload_id>
  botkeep-api.sh capabilities
  botkeep-api.sh plan
  botkeep-api.sh network <workload_id>
  botkeep-api.sh action <workload_id> restart
  botkeep-api.sh settings <workload_id> '<json>'
  botkeep-api.sh env-put <workload_id> '{"key":"WEB_PORT","value":"8090","isSecret":false}'
  botkeep-api.sh github-get <workload_id>
  botkeep-api.sh github-inspect <workload_id> <owner/repo> [branch]
  botkeep-api.sh github-sync <workload_id> <owner/repo> <64-char-revision> [branch]
  botkeep-api.sh github-recover <workload_id> [resume|rollback] [server-name]
  botkeep-api.sh console-cmd <workload_id> '<command>'
  botkeep-api.sh github-autopull <id> false <64-char-revision>
  ./scripts/botkeep-upload-changed.sh   # few files, not full GitHub sync
  ./scripts/botkeep-pull-deploy.sh      # full tree — slow

OpenAPI: GET /api/v1/developer/openapi.json
Power:   POST /api/v1/developer/workloads/{id}/actions  (Idempotency-Key header)
Env:     PUT  /api/v1/developer/workloads/{id}/environment
Runtime: PUT  /api/v1/developer/workloads/{id}/runtime
EOF
    exit 1
    ;;
esac
