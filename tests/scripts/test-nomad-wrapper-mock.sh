#!/usr/bin/env bash
# CI-friendly mock test for nomad-job-manager.sh without real Nomad/VPS
set -euo pipefail

WRAPPER="$(pwd)/iac/common/scripts/nomad-job-manager.sh"

echo "[mock] Verifying wrapper exists"
test -x "$WRAPPER"

echo "[mock] Help output"
"$WRAPPER" help >/dev/null

echo "[mock] Start mock Nomad API server"
TMPDIR="$(mktemp -d)"
PORT=8765
mkdir -p "$TMPDIR/mock/v1/job/ploy-api"
cat > "$TMPDIR/mock/v1/job/ploy-api/allocations" <<'JSON'
[
  {
    "ID": "abcd1234efgh",
    "Name": "ploy-api",
    "ClientStatus": "running",
    "DesiredStatus": "run",
    "NodeName": "mock-node",
    "CreateTime": 1736716800000000000
  }
]
JSON

(cd "$TMPDIR" && python3 -m http.server "$PORT" >/dev/null 2>&1 &)
SERVER_PID=$!
trap 'kill $SERVER_PID 2>/dev/null || true; rm -rf "$TMPDIR"' EXIT

export NOMAD_ADDR="http://localhost:${PORT}/mock"

echo "[mock] Run allocs against mock server"
"$WRAPPER" allocs --job ploy-api --format json | jq -e 'type=="array" and length>=1' >/dev/null

echo "[mock] Validate with JSON file (no Nomad CLI required)"
"$WRAPPER" validate --file tests/nomad-jobs/ci-valid.json >/dev/null

echo "[mock] All mock tests passed"

