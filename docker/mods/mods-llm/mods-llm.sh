#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<USAGE
mods-llm [--plan|--execute] [--input <dir>] [--out <file>]

This is a minimal stub used for E2E. It simulates:
 - planning: writes a trivial plan JSON
 - execution: applies a known fix for the ploy-orw-java11-maven e2e/fail-missing-symbol branch
USAGE
}

mode="execute"
in_dir="/workspace"
out_file="/out/plan.json"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --plan) mode="plan"; shift ;;
    --execute) mode="execute"; shift ;;
    --input) in_dir="$2"; shift 2 ;;
    --out) out_file="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown arg: $1" >&2; usage >&2; exit 1 ;;
  esac
done

mkdir -p "$(dirname "$out_file")"

if [[ "$mode" == "plan" ]]; then
  echo '{"actions":[{"type":"create","path":"src/main/java/e2e/UnknownClass.java"}]}' > "$out_file"
  echo "[mods-llm] Plan written to $out_file"
  exit 0
fi

# Execute: apply fix when the failing sample is detected
proj="$in_dir"
fail_indicator="$proj/src/main/java/e2e/FailMissingSymbol.java"
target_file="$proj/src/main/java/e2e/UnknownClass.java"

if [[ -f "$fail_indicator" ]]; then
  mkdir -p "$(dirname "$target_file")"
  cat > "$target_file" <<'JAVA'
package e2e;

public class UnknownClass {
    @Override
    public String toString() { return "llm-heal"; }
}
JAVA
  echo "[mods-llm] Healed missing symbol by creating e2e.UnknownClass"
else
  echo "[mods-llm] No known failure pattern detected; no-op"
fi

# Always emit a small execution log for diagnostics
echo "{\"executed\":true,\"ts\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}" > "$(dirname "$out_file")/exec.json"

