#!/usr/bin/env bash
# Deploy day-remind sources to Botkeep via SFTP (panel → server → SFTP).
# Credentials are short-lived; create them in the Botkeep UI before running.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
API="$ROOT/scripts/botkeep-api.sh"
WL_ID="${BOTKEEP_WORKLOAD_ID:-56ba890b-2b74-4e58-911d-61db5985b287}"
ENV_FILE="${BOTKEEP_SFTP_ENV:-$ROOT/.botkeep-sftp.env}"

if [[ -f "$ENV_FILE" ]]; then
  # shellcheck disable=SC1090
  set -a && source "$ENV_FILE" && set +a
fi

: "${BOTKEEP_SFTP_HOST:?Set BOTKEEP_SFTP_HOST (panel → SFTP → Server)}"
: "${BOTKEEP_SFTP_USER:?Set BOTKEEP_SFTP_USER}"
: "${BOTKEEP_SFTP_PASS:?Set BOTKEEP_SFTP_PASS}"
BOTKEEP_SFTP_PORT="${BOTKEEP_SFTP_PORT:-2022}"

if ! command -v lftp >/dev/null 2>&1; then
  if command -v brew >/dev/null 2>&1; then
    echo "Installing lftp …"
    brew install lftp
  else
    echo "Install lftp (brew install lftp) for recursive SFTP deploy." >&2
    exit 1
  fi
fi

echo "Syncing $ROOT → sftp://${BOTKEEP_SFTP_HOST}:${BOTKEEP_SFTP_PORT}/ (workload $WL_ID)"
export LFTP_PASSWORD="$BOTKEEP_SFTP_PASS"
lftp -u "$BOTKEEP_SFTP_USER" -p "$BOTKEEP_SFTP_PORT" "sftp://${BOTKEEP_SFTP_HOST}" <<EOF
set sftp:connect-program "ssh -a -x -o StrictHostKeyChecking=accept-new"
mirror -R --verbose \
  -X .git/ -X .git \
  -X bin/ -X data/ \
  -X .env -X .botkeep-api-key -X .botkeep-sftp.env \
  -X .cursor/ -X .cf-remind-proxy/ \
  "$ROOT" /
bye
EOF
unset LFTP_PASSWORD

echo "Restarting workload …"
"$API" action "$WL_ID" restart
echo "Deploy uploaded. After restart, check https://dev4help.in/remind/api/health"
