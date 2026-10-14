#!/bin/sh
set -e
cd /home/container
chmod 755 day-remind
export SSL_CERT_FILE="${SSL_CERT_FILE:-/home/container/ca-certificates.crt}"
# Load every .env entry. Skip a key the panel has already set.
if [ -f .env ]; then
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in
      ''|\#*) continue ;;
    esac
    case "$line" in
      export\ *) line=${line#export } ;;
    esac
    key=${line%%=*}
    key=$(printf '%s' "$key" | tr -d '[:space:]')
    case "$key" in
      ''|*[!A-Za-z0-9_]*) continue ;;
    esac
    if printenv "$key" >/dev/null 2>&1; then
      continue
    fi
    val=${line#*=}
    case "$val" in
      \"*\") val=${val#\"}; val=${val%\"} ;;
      \'*\') val=${val#\'}; val=${val%\'} ;;
    esac
    export "$key=$val"
  done < .env
fi
printf '__BOTKEEP_PROCESS_START__\n'
exec ./day-remind
