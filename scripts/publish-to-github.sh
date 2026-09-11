#!/usr/bin/env bash
# Publish day-remind to GitHub (run on your machine with gh logged in as Shivam-20).
set -euo pipefail
cd "$(dirname "$0")/.."

OWNER="${GITHUB_OWNER:-Shivam-20}"
REPO="${GITHUB_REPO:-day-remind}"

if ! command -v gh >/dev/null; then
  echo "Install GitHub CLI: https://cli.github.com/"
  exit 1
fi

if ! gh repo view "$OWNER/$REPO" >/dev/null 2>&1; then
  echo "Creating $OWNER/$REPO ..."
  gh repo create "$OWNER/$REPO" --public \
    --description "Day Remind — multi-channel reminder app" \
    --source=. --remote=origin --push
else
  echo "Repo exists — pushing main ..."
  git remote remove origin 2>/dev/null || true
  git remote add origin "https://github.com/$OWNER/$REPO.git"
  git push -u origin main
fi

echo "Published: https://github.com/$OWNER/$REPO"
