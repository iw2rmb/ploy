#!/usr/bin/env bash
# Seed Mods integration fixtures into SeaweedFS and verify Git remotes.
set -euo pipefail

readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
readonly FIXTURE_ROOT="${MODS_FIXTURE_DIR:-${PROJECT_ROOT}/tests/mods-fixtures}"
readonly MODS_ARTIFACT_PREFIX="artifacts/mods"

log() {
  printf '[%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"
}

error() {
  printf '[%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*" >&2
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    error "command not found: $1"
    exit 1
  fi
}

require_env() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    error "missing required environment variable: $name"
    exit 1
  fi
}

SEAWEED_BASE="${PLOY_SEAWEEDFS_URL:-${SEAWEEDFS_FILER:-}}"
if [[ -z "${SEAWEED_BASE}" ]]; then
  error "Set PLOY_SEAWEEDFS_URL or SEAWEEDFS_FILER to target a SeaweedFS filer"
  exit 1
fi

SEAWEED_BASE=${SEAWEED_BASE%/}

require_cmd curl
require_cmd tar
require_cmd git

if [[ ! -d "${FIXTURE_ROOT}" ]]; then
  error "fixture directory not found: ${FIXTURE_ROOT}"
  exit 1
fi

UPLOAD_COUNT=0

upload_file() {
  local key="$1"
  local path="$2"
  local content_type="$3"

  local url="${SEAWEED_BASE}/${MODS_ARTIFACT_PREFIX}/${key}"
  log "Uploading ${path} -> ${url}"
  local args=(-sS -X PUT "${url}" -T "${path}")
  if [[ -n "${content_type}" ]]; then
    args=(-sS -X PUT "${url}" -H "Content-Type: ${content_type}" -T "${path}")
  fi
  if ! curl "${args[@]}" -o /dev/null; then
    error "failed to upload ${key}"
    exit 1
  fi
  UPLOAD_COUNT=$((UPLOAD_COUNT + 1))
}

seed_mod_fixture() {
  local mod_dir="$1"
  local mod_id
  mod_id="$(basename "${mod_dir}")"
  log "Seeding fixtures for ${mod_id}"

  local tmp_dir
  tmp_dir="$(mktemp -d)"

  if [[ -d "${mod_dir}/input" ]]; then
    local tarball="${tmp_dir}/input.tar"
    tar -cf "${tarball}" -C "${mod_dir}/input" .
    upload_file "${mod_id}/input.tar" "${tarball}" "application/octet-stream"
  else
    log "No input/ directory for ${mod_id}; skipping input.tar upload"
  fi

  if [[ -d "${mod_dir}/artifacts" ]]; then
    (cd "${mod_dir}/artifacts" && find . -type f -print0) | while IFS= read -r -d '' relpath; do
      local file_path="${mod_dir}/artifacts/${relpath#./}"
      local key="${mod_id}/${relpath#./}"
      local mime="application/octet-stream"
      case "${file_path}" in
        *.json) mime="application/json" ;;
        *.patch|*.diff) mime="text/plain" ;;
        *.txt|*.md) mime="text/plain" ;;
      esac
      upload_file "${key}" "${file_path}" "${mime}"
    done
  fi

  rm -rf "${tmp_dir}"
}

for mod_dir in "${FIXTURE_ROOT}"/*; do
  [[ -d "${mod_dir}" ]] || continue
  seed_mod_fixture "${mod_dir}"
done

log "Uploaded ${UPLOAD_COUNT} fixture payload(s) to SeaweedFS"

# Git fixture validation
MODS_FIXTURE_REPOS=${MODS_FIXTURE_REPOS:-"https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git"}
IFS=',' read -r -a repo_array <<< "${MODS_FIXTURE_REPOS}"
for repo in "${repo_array[@]}"; do
  repo="${repo// /}"
  [[ -n "${repo}" ]] || continue
  local auth_url="${repo}"
  case "${repo}" in
    https://github.com/*)
      require_env GITHUB_PLOY_DEV_USERNAME
      require_env GITHUB_PLOY_DEV_PAT
      auth_url=$(printf '%s' "${repo}" | sed -e "s#https://#https://${GITHUB_PLOY_DEV_USERNAME}:${GITHUB_PLOY_DEV_PAT}@#")
      ;;
    https://gitlab.com/*)
      require_env GITLAB_TOKEN
      auth_url=$(printf '%s' "${repo}" | sed -e "s#https://#https://oauth2:${GITLAB_TOKEN}@#")
      ;;
  esac
  log "Validating fixture repository access: ${repo}"
  if ! GIT_TERMINAL_PROMPT=0 git ls-remote --heads "${auth_url}" >/dev/null 2>&1; then
    error "unable to access fixture repository: ${repo}"
    exit 1
  fi
  log "Repository reachable: ${repo}"
  # Mask credentialed URL
  unset auth_url
done

log "Mods fixture seeding complete"
