#!/usr/bin/env bash
set -euo pipefail

# Capture planner allocation logs shortly after planner starts.
# Usage:
#   PLOY_CONTROLLER=https://api.dev.ployman.app/v1 \
#   ANSIBLE_INVENTORY=iac/dev/inventory/hosts.yml \
#   TARGET_HOST=45.12.75.241 \
#   ./capture-planner-logs.sh <MOD_ID>

MOD_ID=${1:-}
if [[ -z "${PLOY_CONTROLLER:-}" ]]; then
  echo "PLOY_CONTROLLER must be set (e.g., https://api.dev.ployman.app/v1)" >&2
  exit 1
fi
if [[ -z "$MOD_ID" ]]; then
  echo "Usage: $0 <MOD_ID>" >&2
  exit 1
fi

ANSIBLE_INVENTORY=${ANSIBLE_INVENTORY:-iac/dev/inventory/hosts.yml}
TARGET_HOST=${TARGET_HOST:-}
if [[ -z "$TARGET_HOST" ]]; then
  echo "TARGET_HOST env must be set to the VPS IP/host" >&2
  exit 1
fi

echo "[capture] Waiting for planner to start (MOD_ID=$MOD_ID) …"
# Wait up to 90s for planner stage to begin
deadline=$((SECONDS+90))
started=0
while (( SECONDS < deadline )); do
  st=$(curl -fsS "$PLOY_CONTROLLER/mods/$MOD_ID/status" || true)
  phase=$(echo "$st" | jq -r '.phase // empty' 2>/dev/null || true)
  job=$(echo "$st" | jq -r '.last_job.job_name // empty' 2>/dev/null || true)
  if [[ "$phase" == "healing" && "$job" == *"planner"* ]]; then
    echo "[capture] Planner started: job=$job"
    started=1
    break
  fi
  sleep 1
done
if [[ "$started" != "1" ]]; then
  echo "[capture] Planner did not start within timeout; exiting." >&2
  exit 0
fi

echo "[capture] Polling for latest mods-planner allocation and fetching logs …"
# Try up to 10 times to get the latest allocation and its logs
for i in $(seq 1 10); do
  ALLOC_ID=$(ansible -i "$ANSIBLE_INVENTORY" -e target_host="$TARGET_HOST" ploy-dev -m shell \
    -a 'bash -lc "/opt/hashicorp/bin/nomad-job-manager.sh allocs --job mods-planner --format json | jq -r \"sort_by(.CreateTime) | reverse | .[0].ID\""' \
    -o 2>/dev/null | awk -F'>>' 'NF>1{print $2}' | tr -d '\r\n ')
  if [[ -n "$ALLOC_ID" && "${#ALLOC_ID}" -ge 36 ]]; then
    echo "[capture] Latest ALLOC_ID: $ALLOC_ID (attempt $i)"
    echo "[capture] Planner task logs (last 400 lines):"
    ansible -i "$ANSIBLE_INVENTORY" -e target_host="$TARGET_HOST" ploy-dev -m shell \
      -a "bash -lc '/opt/hashicorp/bin/nomad-job-manager.sh logs --alloc-id \"$ALLOC_ID\" --task planner --lines 400'" \
      -o 2>/dev/null | awk -F'>>' 'NF>1{print $2}' | sed 's/^/[planner-log] /'
    exit 0
  fi
  sleep 1
done

echo "[capture] Unable to resolve planner allocation logs after retries." >&2
exit 1

