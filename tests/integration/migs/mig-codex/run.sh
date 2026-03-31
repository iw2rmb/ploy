#!/usr/bin/env bash
set -euo pipefail

# Run the real mig-codex integration test with build‑gate verification.
# - Builds the migs-codex image from repo root (includes ploy-buildgate CLI)
# - Exports CODEX_AUTH_JSON (reads ~/.codex/auth.json if unset)
# - Executes the Go test: TestModCodex_HealsUsingBuildGateLog_FromFailingBranch

ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
cd "$ROOT"

command -v go >/dev/null 2>&1 || { echo "go not found" >&2; exit 1; }
command -v docker >/dev/null 2>&1 || { echo "docker not found" >&2; exit 1; }
command -v git >/dev/null 2>&1 || { echo "git not found" >&2; exit 1; }

if [[ -z "${CODEX_AUTH_JSON:-}" ]]; then
  if [[ -f "$HOME/.codex/auth.json" ]]; then
    export CODEX_AUTH_JSON="$(cat "$HOME/.codex/auth.json")"
  else
    echo "CODEX_AUTH_JSON not set and ~/.codex/auth.json missing" >&2
    exit 1
  fi
fi

echo "[run] Building migs-codex image (repo root)…" >&2
docker build -t migs-codex:latest -f deploy/images/codex/Dockerfile . >/dev/null

echo "[run] Executing integration test…" >&2
GOFLAGS=${GOFLAGS:-} go test -v ./tests/integration/migs/mig-codex -run TestModCodex_HealsUsingBuildGateLog_FromFailingBranch -count=1
