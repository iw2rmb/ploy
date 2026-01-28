#!/usr/bin/env bash
set -euo pipefail

# E2E: Verify Gradle Build Gate uses the remote Gradle Build Cache node.
#
# This scenario depends on the local Docker cluster from scripts/deploy-locally.sh:
# - docker compose -f local/docker-compose.yml up -d
# - gradle-build-cache service reachable as http://gradle-build-cache:5071/cache/ from gate containers
#
# It clears the cache node data, runs a no-op mod run that triggers:
# - pre_gate (Gradle build, pushes cache)
# - post_gate (Gradle build, hits cache in a fresh container)
#
# Success signal:
# - post_gate job meta includes a GRADLE_BUILD_CACHE_HIT finding emitted by the gate executor.

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
export PLOY_CONFIG_HOME="${PLOY_CONFIG_HOME:-$REPO_ROOT/local/cli}"

PLOY_BIN=""
if [[ -x "$REPO_ROOT/dist/ploy" ]]; then
  PLOY_BIN="$REPO_ROOT/dist/ploy"
else
  echo "Error: ploy binary not found at $REPO_ROOT/dist/ploy" >&2
  exit 1
fi

REPO_URL="${REPO_URL:-https://gitlab.com/iw2rmb/ploy-gradle-build-cache.git}"
# NOTE: The node's git fetcher cache keys by (repo_url, base_ref, commit_sha) and does not refresh
# cached clones for moving refs. Use a stable tag for deterministic E2E runs.
REPO_BASE_REF="${REPO_BASE_REF:-e2e/build-cache}"
REPO_TARGET_REF="${REPO_TARGET_REF:-e2e/build-cache}"
SPEC_FILE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/mod.yaml"

TS=$(date +%y%m%d%H%M%S)
OUT_BASE=${PLOY_E2E_OUT_BASE:-./tmp/gradle/build-cache}
OUT_DIR=${PLOY_E2E_OUT_DIR:-${OUT_BASE}/${TS}}
mkdir -p "${OUT_DIR}"

cache_container_id="$(docker ps --filter 'name=gradle-build-cache' --format '{{.ID}}' | head -n 1 || true)"
if [[ -z "${cache_container_id:-}" ]]; then
  echo "Error: gradle-build-cache container not found; deploy the local cluster first (scripts/deploy-locally.sh)." >&2
  exit 1
fi

echo "[e2e] Resetting gradle-build-cache node data..."
docker exec "${cache_container_id}" sh -c 'rm -rf /data/system/cache/* 2>/dev/null || true; mkdir -p /data/system/cache' >/dev/null 2>&1 || true
docker restart "${cache_container_id}" >/dev/null

echo "[e2e] Waiting for gradle-build-cache node HTTP to be ready (localhost:5071)..."
for i in {1..60}; do
  if curl -fsS http://localhost:5071/ >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

for i in {1..30}; do
  if docker exec "${cache_container_id}" sh -c 'test -d /data/system/cache' >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

cache_entries_before="$(docker exec "${cache_container_id}" sh -c 'find /data/system/cache/artifacts-v2 -type f 2>/dev/null | wc -l' | tr -d '[:space:]' || true)"
cache_entries_before="${cache_entries_before:-0}"

echo "[e2e] Ensuring gate env config for Gradle build cache..."
"$PLOY_BIN" config env set --key PLOY_GRADLE_BUILD_CACHE_URL --value "http://gradle-build-cache:5071/cache/" --scope gate >/dev/null
"$PLOY_BIN" config env set --key PLOY_GRADLE_BUILD_CACHE_PUSH --value "true" --scope gate >/dev/null

echo "[e2e] Submitting run (repo=${REPO_URL}, base=${REPO_BASE_REF}, target=${REPO_TARGET_REF})..."
RUN_JSON="$("$PLOY_BIN" mod run --json \
  --repo-url "$REPO_URL" \
  --repo-base-ref "$REPO_BASE_REF" \
  --repo-target-ref "$REPO_TARGET_REF" \
  --spec "$SPEC_FILE" \
  --follow)"

RUN_ID="$(printf '%s' "$RUN_JSON" | jq -r '.run_id')"
if [[ -z "${RUN_ID:-}" || "${RUN_ID}" == "null" ]]; then
  echo "Error: could not parse run_id from JSON output" >&2
  echo "$RUN_JSON" >&2
  exit 1
fi

echo "[e2e] Verifying remote cache hit via structured gate metadata (post_gate)..."
hit_count="$(
  docker compose -f "$REPO_ROOT/local/docker-compose.yml" exec -T db \
    psql -U ploy -d ploy -v ON_ERROR_STOP=1 -qXAt \
    -c "SET search_path TO ploy, public; SELECT count(*) FROM jobs WHERE run_id='${RUN_ID}' AND mod_type='post_gate' AND (meta->'gate'->'log_findings') @> '[{\"code\":\"GRADLE_BUILD_CACHE_HIT\"}]'::jsonb;"
)"
hit_count="$(echo "$hit_count" | tr -d '[:space:]')"
if [[ "${hit_count}" != "1" ]]; then
  echo "Error: expected exactly 1 post_gate cache-hit finding (GRADLE_BUILD_CACHE_HIT), got ${hit_count}" >&2
  docker compose -f "$REPO_ROOT/local/docker-compose.yml" exec -T db \
    psql -U ploy -d ploy -v ON_ERROR_STOP=1 -qX \
    -c "SET search_path TO ploy, public; SELECT id, mod_type, status, meta FROM jobs WHERE run_id='${RUN_ID}' ORDER BY step_index;" >&2 || true
  exit 1
fi

cache_entries_after="$(docker exec "${cache_container_id}" sh -c 'find /data/system/cache/artifacts-v2 -type f 2>/dev/null | wc -l' | tr -d '[:space:]' || true)"
cache_entries_after="${cache_entries_after:-0}"
if [[ "${cache_entries_after}" -le "${cache_entries_before}" ]]; then
  echo "Error: expected gradle-build-cache node to store entries (before=${cache_entries_before}, after=${cache_entries_after})" >&2
  exit 1
fi

echo "OK: gradle build cache used"
echo "Run: ${RUN_ID}"
echo "Output: ${OUT_DIR}"
