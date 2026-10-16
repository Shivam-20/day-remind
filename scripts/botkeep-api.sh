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
  github-sync)
    id="${1:?workload id}"
    key="$(idem)"
    curl -sS "${auth[@]}" -X POST \
      -H "Content-Type: application/json" \
      -H "Idempotency-Key: $key" \
      -d '{}' \
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
  botkeep-api.sh github-sync <workload_id>

OpenAPI: GET /api/v1/developer/openapi.json
Power:   POST /api/v1/developer/workloads/{id}/actions  (Idempotency-Key header)
Env:     PUT  /api/v1/developer/workloads/{id}/environment
Runtime: PUT  /api/v1/developer/workloads/{id}/runtime
EOF
    exit 1
    ;;
esac
