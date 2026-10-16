#!/usr/bin/env bash
# Copy all databases from SOURCE_MONGODB_URI to DEST_MONGODB_URI (Atlas → Atlas).
# Requires: mongodump + mongorestore (MongoDB Database Tools) or Docker.
#
#   export SOURCE_MONGODB_URI='mongodb+srv://USER:PASS@source.mongodb.net/'
#   export DEST_MONGODB_URI='mongodb+srv://USER:PASS@dest.mongodb.net/'
#   ./scripts/atlas-migrate.sh
#
# Optional: only day_remind
#   export ONLY_DB=day_remind
#
# Atlas: add your current public IP to Network Access on BOTH clusters before running.

set -euo pipefail

SOURCE_MONGODB_URI="${SOURCE_MONGODB_URI:?set SOURCE_MONGODB_URI}"
DEST_MONGODB_URI="${DEST_MONGODB_URI:?set DEST_MONGODB_URI}"
ONLY_DB="${ONLY_DB:-}"
WORKDIR="${WORKDIR:-$(mktemp -d -t dayremind-mongodump.XXXXXX)}"

run_mongodump() {
  if command -v mongodump >/dev/null; then
    mongodump "$@"
  else
    docker run --rm -v "${WORKDIR}:/dump" mongo:7 mongodump "$@" --out=/dump/data
  fi
}

run_mongorestore() {
  if command -v mongorestore >/dev/null; then
    mongorestore "$@"
  else
    docker run --rm -v "${WORKDIR}:/dump" mongo:7 mongorestore "$@" /dump/data
  fi
}

DUMP_ARGS=(--uri="$SOURCE_MONGODB_URI")
RESTORE_ARGS=(--uri="$DEST_MONGODB_URI" --drop)

if [[ -n "$ONLY_DB" ]]; then
  DUMP_ARGS+=(--db="$ONLY_DB")
  RESTORE_ARGS+=(--nsInclude="${ONLY_DB}.*")
fi

if command -v mongodump >/dev/null; then
  OUT="$WORKDIR"
  DUMP_ARGS+=(--out="$OUT")
else
  echo "Using Docker (mongo:7); ensure Docker is running."
  OUT="$WORKDIR/data"
fi

echo "Dumping to ${WORKDIR} ..."
run_mongodump "${DUMP_ARGS[@]}"

echo "Restoring to destination (--drop replaces existing collections in restored namespaces) ..."
if command -v mongorestore >/dev/null; then
  run_mongorestore "${RESTORE_ARGS[@]}" "$OUT"
else
  run_mongorestore "${RESTORE_ARGS[@]}"
fi

echo "Done. Dump left in ${WORKDIR} — remove when verified."
echo "Update DATABASE_URL on Botkeep/home server to DEST_MONGODB_URI (with database name, e.g. /day_remind)."
