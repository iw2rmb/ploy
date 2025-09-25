#!/usr/bin/env bash
set -euo pipefail

require_env() {
  local name=$1
  if [[ -z "${!name:-}" ]]; then
    printf 'Missing required environment variable: %s\n' "$name" >&2
    exit 1
  fi
}

require_env TARGET_HOST

if ! command -v git >/dev/null 2>&1; then
  echo "git binary not found" >&2
  exit 1
fi

COMMIT_SHA=$(git rev-parse --verify HEAD)
BRANCH_NAME=$(git rev-parse --abbrev-ref HEAD)
REMOTE_NAME=${MODS_REMOTE:-origin}

REMOTE_EXPORTS=()
REMOTE_EXPORTS+=("GOFLAGS=${GOFLAGS:-}")
if [[ -n "${PLOY_CONTROLLER:-}" ]]; then
  REMOTE_EXPORTS+=("PLOY_CONTROLLER=${PLOY_CONTROLLER}")
fi
if [[ -n "${PLOY_SEAWEEDFS_URL:-}" ]]; then
  REMOTE_EXPORTS+=("PLOY_SEAWEEDFS_URL=${PLOY_SEAWEEDFS_URL}")
fi
if [[ -n "${PLOY_JETSTREAM_URL:-}" ]]; then
  REMOTE_EXPORTS+=("PLOY_JETSTREAM_URL=${PLOY_JETSTREAM_URL}")
fi
if [[ -n "${PLOY_GITLAB_PAT:-}" ]]; then
  REMOTE_EXPORTS+=("PLOY_GITLAB_PAT=${PLOY_GITLAB_PAT}")
fi
if [[ -n "${GITLAB_TOKEN:-}" ]]; then
  REMOTE_EXPORTS+=("GITLAB_TOKEN=${GITLAB_TOKEN}")
fi
if [[ -n "${MODS_NATS_PORT:-}" ]]; then
  REMOTE_EXPORTS+=("MODS_NATS_PORT=${MODS_NATS_PORT}")
fi
ENV_EXPORT_BLOCK=""
for kv in "${REMOTE_EXPORTS[@]}"; do
  ENV_EXPORT_BLOCK+="export ${kv}; "
done

REMOTE_SCRIPT=$(cat <<'EOSCRIPT'
set -euo pipefail
__ENV_EXPORT_BLOCK__
cd /home/ploy/ploy

ensure_commit() {
  if git rev-parse --verify "__COMMIT_SHA__" >/dev/null 2>&1; then
    return 0
  fi
  echo "Fetching commit __COMMIT_SHA__ from __REMOTE_NAME__"
  git fetch "__REMOTE_NAME__" "__BRANCH_NAME__" || git fetch "__REMOTE_NAME__" --all
  git rev-parse --verify "__COMMIT_SHA__" >/dev/null 2>&1
}

if ! ensure_commit; then
  echo "Commit __COMMIT_SHA__ not available on __REMOTE_NAME__. Push your changes before running the harness." >&2
  exit 1
fi

CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
TMP_NATS_DIR=""
NATS_PID=""

cleanup() {
  if [[ -n "${NATS_PID}" ]]; then
    kill "${NATS_PID}" >/dev/null 2>&1 || true
    wait "${NATS_PID}" 2>/dev/null || true
  fi
  if [[ -n "${TMP_NATS_DIR}" ]]; then
    rm -rf "${TMP_NATS_DIR}"
  fi
  git checkout --quiet "${CURRENT_BRANCH}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "Switching from ${CURRENT_BRANCH} to __COMMIT_SHA__"
git checkout --quiet "__COMMIT_SHA__"

resolve_jetstream_addr() {
  if [[ -n "${PLOY_JETSTREAM_URL:-}" ]]; then
    printf '%s\n' "${PLOY_JETSTREAM_URL}"
    return 0
  fi

  if command -v curl >/dev/null 2>&1; then
    local payload
    payload=$(curl -sf http://127.0.0.1:8500/v1/catalog/service/nats 2>/dev/null || true)
    if [[ -n "${payload}" ]]; then
      if command -v python3 >/dev/null 2>&1; then
        local parsed
        parsed=$(P="${payload}" python3 - <<'PY' 2>/dev/null
import json, os, sys
value = os.environ.get("P")
if not value:
    sys.exit(0)
try:
    data = json.loads(value)
except json.JSONDecodeError:
    sys.exit(0)
if not data:
    sys.exit(0)
entry = data[0]
addr = entry.get("ServiceAddress")
port = entry.get("ServicePort")
if addr and port:
    sys.stdout.write(f"nats://{addr}:{port}")
PY
)
        if [[ -n "${parsed}" ]]; then
          printf '%s\n' "${parsed}"
          return 0
        fi
      else
        local addr port
        addr=$(printf '%s\n' "${payload}" | sed -n 's/.*"ServiceAddress":"\([^"]\+\)".*/\1/p' | head -n1)
        port=$(printf '%s\n' "${payload}" | sed -n 's/.*"ServicePort":\([0-9]\+\).*/\1/p' | head -n1)
        if [[ -n "${addr}" && -n "${port}" ]]; then
          printf 'nats://%s:%s\n' "${addr}" "${port}"
          return 0
        fi
      fi
    fi
  fi
  return 1
}

JETSTREAM_ADDR=""
if resolved=$(resolve_jetstream_addr); then
  JETSTREAM_ADDR="${resolved}"
fi

if [[ -n "${JETSTREAM_ADDR}" ]]; then
  export NATS_ADDR="${JETSTREAM_ADDR}"
  echo "Using JetStream endpoint: ${NATS_ADDR}"
else
  echo "JetStream service lookup failed; starting local ephemeral nats-server" >&2
  ensure_nats_server() {
    if command -v nats-server >/dev/null 2>&1; then
      printf '%s' "$(command -v nats-server)"
      return 0
    fi
    echo "Installing nats-server via go install"
    export GO111MODULE=on
    export GOBIN="$HOME/.local/bin"
    mkdir -p "$GOBIN"
    PATH="$GOBIN:$PATH" go install github.com/nats-io/nats-server/v2@v2.10.9
    if [[ ! -x "$GOBIN/nats-server" ]]; then
      return 1
    fi
    printf '%s' "$GOBIN/nats-server"
  }

  NATS_BIN=$(ensure_nats_server)
  if [[ -z "${NATS_BIN}" ]]; then
    echo "Unable to install nats-server" >&2
    exit 1
  fi

  TMP_NATS_DIR=$(mktemp -d)
  NATS_PORT=${MODS_NATS_PORT:-4223}
  "${NATS_BIN}" -js -p "${NATS_PORT}" -m 0 --store dir="${TMP_NATS_DIR}" >"${TMP_NATS_DIR}/server.log" 2>&1 &
  NATS_PID=$!

  for _ in $(seq 1 40); do
    if nc -z 127.0.0.1 "${NATS_PORT}" >/dev/null 2>&1; then
      break
    fi
    sleep 0.25
  done
  if ! nc -z 127.0.0.1 "${NATS_PORT}" >/dev/null 2>&1; then
    echo "Failed to start local nats-server on port ${NATS_PORT}" >&2
    exit 1
  fi
  export NATS_ADDR="nats://127.0.0.1:${NATS_PORT}"
fi

go test -tags=integration -run Integration -v ./internal/mods
EOSCRIPT
)

REMOTE_SCRIPT=${REMOTE_SCRIPT//__COMMIT_SHA__/$COMMIT_SHA}
REMOTE_SCRIPT=${REMOTE_SCRIPT//__ENV_EXPORT_BLOCK__/$ENV_EXPORT_BLOCK}
REMOTE_SCRIPT=${REMOTE_SCRIPT//__BRANCH_NAME__/$BRANCH_NAME}
REMOTE_SCRIPT=${REMOTE_SCRIPT//__REMOTE_NAME__/$REMOTE_NAME}

ssh -o ConnectTimeout=10 "root@${TARGET_HOST}" "cat <<'EOS' >/tmp/mods-integration-run.sh
${REMOTE_SCRIPT}
EOS"

ssh -o ConnectTimeout=10 "root@${TARGET_HOST}" "chmod +x /tmp/mods-integration-run.sh && su - ploy -c '/tmp/mods-integration-run.sh'"
ssh -o ConnectTimeout=10 "root@${TARGET_HOST}" 'rm -f /tmp/mods-integration-run.sh'
