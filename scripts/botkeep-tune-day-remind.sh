#!/usr/bin/env bash
# Apply recommended Day Remind settings on Botkeep via Developer API (no panel clicks).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
API="$ROOT/scripts/botkeep-api.sh"
WL_ID="${BOTKEEP_WORKLOAD_ID:-56ba890b-2b74-4e58-911d-61db5985b287}"

# Build only when sources are newer than the binary. Never `go run` (recompiles the whole tree every boot).
START_CMD='export GOCACHE=/home/container/.cache/go-build GOTMPDIR=/home/container/tmp CGO_ENABLED=0; mkdir -p "$GOCACHE" "$GOTMPDIR"; if [ ! -x ./day-remind ] || [ cmd/server -nt ./day-remind ] || [ internal -nt ./day-remind ]; then go build -o day-remind ./cmd/server; fi; exec ./day-remind'

echo "Fetching workload $WL_ID ..."
detail="$("$API" workload "$WL_ID")"
name=$(echo "$detail" | /usr/bin/python3 -c "import json,sys; d=json.load(sys.stdin); print(d['name'])")
branch=$(echo "$detail" | /usr/bin/python3 -c "import json,sys; d=json.load(sys.stdin); print(d['branch'])")
runtime=$(echo "$detail" | /usr/bin/python3 -c "import json,sys; d=json.load(sys.stdin); print(d['runtime'])")
rv=$(echo "$detail" | /usr/bin/python3 -c "import json,sys; d=json.load(sys.stdin); print(d['runtimeVersion'])")

port=$(echo "$("$API" network "$WL_ID")" | /usr/bin/python3 -c "
import json,sys
d=json.load(sys.stdin)
alloc=d.get('allocations') or []
for a in alloc:
    if a.get('primary') and a.get('port'):
        print(a['port']); raise SystemExit
if alloc and alloc[0].get('port'):
    print(alloc[0]['port']); raise SystemExit
print(8090)
" 2>/dev/null || echo 8090)

echo "Updating startup (cache build, run binary) ..."
notif='{"deploymentFailed":true,"botCrashed":true,"usageWarning":true}'
body=$(NOTIF="$notif" START="$START_CMD" NAME="$name" BR="$branch" RT="$runtime" RV="$rv" /usr/bin/python3 -c '
import json, os
print(json.dumps({
  "name": os.environ["NAME"],
  "runtime": os.environ["RT"],
  "runtimeVersion": os.environ["RV"],
  "branch": os.environ["BR"],
  "startCommand": os.environ["START"],
  "notifications": json.loads(os.environ["NOTIF"]),
}))
')
"$API" settings "$WL_ID" "$body" >/dev/null

echo "Ensuring DAY_REMIND_SKIP_DOTENV=1 and WEB_PORT=$port ..."
"$API" env-put "$WL_ID" "{\"key\":\"DAY_REMIND_SKIP_DOTENV\",\"value\":\"1\",\"isSecret\":false}" >/dev/null
"$API" env-put "$WL_ID" "{\"key\":\"WEB_PORT\",\"value\":\"$port\",\"isSecret\":false}" >/dev/null

echo "Fixing DATABASE_URL database name if missing /day_remind ..."
"$API" env "$WL_ID" | /usr/bin/python3 -c "
import json, sys, subprocess, os
d=json.load(sys.stdin)
api=os.environ['API']
wl=os.environ['WL_ID']
for v in d.get('variables', []):
    if v.get('key') != 'DATABASE_URL':
        continue
    val = v.get('value') or ''
    if not val:
        break
    if val.rstrip('/').endswith('day_remind'):
        print('DATABASE_URL already has day_remind')
        break
    sep = '&' if '?' in val else '?'
    if val.endswith('/'):
        new = val + 'day_remind'
    else:
        new = val.rstrip('/') + '/day_remind'
    payload = json.dumps({'key': 'DATABASE_URL', 'value': new, 'isSecret': True})
    subprocess.run([api, 'env-put', wl, payload], check=True, capture_output=True)
    print('DATABASE_URL updated (value not printed)')
    break
" API="$API" WL_ID="$WL_ID"

echo "Restarting workload ..."
"$API" action "$WL_ID" restart
echo "Done. Check console logs in the panel."
