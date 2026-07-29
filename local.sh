#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")"

if [ -f .env ]; then
  set -a
  # shellcheck disable=SC1091
  . ./.env
  set +a
fi

if [ -z "${SLACK_BOT_TOKEN:-}" ]; then
  echo "SLACK_BOT_TOKEN is required. Set it in .env or export it before running." >&2
  exit 1
fi

if [ -z "${SLACK_APP_TOKEN:-}" ]; then
  echo "SLACK_APP_TOKEN is required. Set it in .env or export it before running." >&2
  exit 1
fi

export LOG_LEVEL="${LOG_LEVEL:-debug}"

exec go run .
