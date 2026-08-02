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

IMAGE="findsenryu4slack:local"

echo "Building Docker image ${IMAGE}..." >&2
docker build -t "${IMAGE}" .

# Socket Mode uses an outbound connection, so no port mapping is required.
# Publish the optional health endpoint only when PORT is set.
PORT_ARGS=""
if [ -n "${PORT:-}" ]; then
  PORT_ARGS="-p ${PORT}:${PORT} -e PORT=${PORT}"
fi

echo "Running ${IMAGE}..." >&2
# shellcheck disable=SC2086
exec docker run --rm -it \
  -e SLACK_BOT_TOKEN="${SLACK_BOT_TOKEN}" \
  -e SLACK_APP_TOKEN="${SLACK_APP_TOKEN}" \
  -e LOG_LEVEL="${LOG_LEVEL:-debug}" \
  ${PORT_ARGS} \
  "${IMAGE}"
