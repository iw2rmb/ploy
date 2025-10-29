#!/bin/zsh
set -euo pipefail

# Non-interactive Codex-only driver for the role-separated TDD experiment.
# Requirements:
#   - CODEX_BIN points to the Codex CLI binary (default: codex)
#   - Run from anywhere; script cds to repo root.

SCRIPT_DIR="${0:A:h}"
REPO_ROOT="${SCRIPT_DIR:h:h}"

CODEX_BIN="${CODEX_BIN:-codex}"
PHASE="${1:-both}"

cd "$REPO_ROOT"

common_args=(
  exec
  "--dangerously-bypass-approvals-and-sandbox"
  "--cd" "$REPO_ROOT"
)

stub_prompt=$'You are Codex operating non-interactively in this repo.\n'
stub_prompt+=$'Goal: Run the role-separated TDD experiment Phase A (stub).\n'
stub_prompt+=$'- Do not modify files.\n'
stub_prompt+=$'- Execute: go test -tags "experiment experiment_stub" ./tests/guards ./tests/experiments/role_sep -run ^TestHT_\n'
stub_prompt+=$'- Report the failing tests and exit.\n'

impl_prompt=$'You are Codex operating non-interactively in this repo.\n'
impl_prompt+=$'Goal: Run the role-separated TDD experiment Phase B (impl).\n'
impl_prompt+=$'- Do not modify files.\n'
impl_prompt+=$'- Execute: go test -tags "experiment experiment_impl" ./tests/guards ./tests/experiments/role_sep -run ^TestHT_ -cover\n'
impl_prompt+=$'- Report results and coverage and exit.\n'

case "$PHASE" in
  stub)
    "$CODEX_BIN" "${common_args[@]}" - <<< "$stub_prompt" ;;
  impl)
    "$CODEX_BIN" "${common_args[@]}" - <<< "$impl_prompt" ;;
  both)
    "$CODEX_BIN" "${common_args[@]}" - <<< "$stub_prompt" || true
    "$CODEX_BIN" "${common_args[@]}" - <<< "$impl_prompt"
    ;;
  *)
    print -u2 "unknown phase: $PHASE (use stub|impl|both)"; exit 2 ;;
esac

