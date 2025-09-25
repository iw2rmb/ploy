#!/usr/bin/env bash
set -euo pipefail

require_env() {
  local name=$1
  if [[ -z "${!name:-}" ]]; then
    printf 'Missing required environment variable: %s\n' "$name" >&2
    exit 1
  fi
}

if ! command -v git >/dev/null 2>&1; then
  echo "git not found" >&2
  exit 1
fi

require_env TARGET_HOST

COMMIT_SHA=$(git rev-parse --verify HEAD)
BRANCH_NAME=$(git rev-parse --abbrev-ref HEAD)
REMOTE_NAME=${MODS_REMOTE:-origin}

REMOTE_ENV_VARS=()
REMOTE_ENV_VARS+=("GOFLAGS=${GOFLAGS:-}")
if [[ -n "${PLOY_CONTROLLER:-}" ]]; then
  REMOTE_ENV_VARS+=("PLOY_CONTROLLER=${PLOY_CONTROLLER}")
fi
if [[ -n "${PLOY_SEAWEEDFS_URL:-}" ]]; then
  REMOTE_ENV_VARS+=("PLOY_SEAWEEDFS_URL=${PLOY_SEAWEEDFS_URL}")
fi
if [[ -n "${PLOY_GITLAB_PAT:-}" ]]; then
  REMOTE_ENV_VARS+=("PLOY_GITLAB_PAT=${PLOY_GITLAB_PAT}")
fi
if [[ -n "${GITLAB_TOKEN:-}" ]]; then
  REMOTE_ENV_VARS+=("GITLAB_TOKEN=${GITLAB_TOKEN}")
fi

REMOTE_ENV_BLOCK=""
for kv in "${REMOTE_ENV_VARS[@]}"; do
  REMOTE_ENV_BLOCK+="export ${kv}; "
done

read -r -d '' REMOTE_SCRIPT <<EOSCRIPT
set -euo pipefail
cd /home/ploy/ploy
if ! git rev-parse --verify ${COMMIT_SHA} >/dev/null 2>&1; then
  echo "Fetching commit ${COMMIT_SHA} from ${REMOTE_NAME}"
  git fetch ${REMOTE_NAME} "${BRANCH_NAME}" || git fetch ${REMOTE_NAME} --all
  if ! git rev-parse --verify ${COMMIT_SHA} >/dev/null 2>&1; then
    echo "Commit ${COMMIT_SHA} not available on ${REMOTE_NAME}. Push your changes before running the harness." >&2
    exit 1
  fi
fi
CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
trap 'git checkout --quiet "${CURRENT_BRANCH}" >/dev/null 2>&1 || true' EXIT
echo "Switching from \\${CURRENT_BRANCH} to ${COMMIT_SHA}"
git checkout --quiet ${COMMIT_SHA}
${REMOTE_ENV_BLOCK}go test -tags=integration -v ./internal/mods
EOSCRIPT

ssh -o ConnectTimeout=10 "root@${TARGET_HOST}" \
  "cat <<'EOS' >/tmp/mods-integration-run.sh
${REMOTE_SCRIPT}
EOS"

ssh -o ConnectTimeout=10 "root@${TARGET_HOST}" \
  "chmod +x /tmp/mods-integration-run.sh && su - ploy -c '/tmp/mods-integration-run.sh'"

ssh -o ConnectTimeout=10 "root@${TARGET_HOST}" 'rm -f /tmp/mods-integration-run.sh'

