#!/usr/bin/env bash
set -euo pipefail

# Minimal stub: produce an advisory plan JSON. The runner records planner
# metadata via its own Mods planner; this container is exercised for parity.

out="/out/plan.json"
mkdir -p /out
cat > "$out" <<'JSON'
{
  "summary": "Plan suggests Java 11->17 migration via OpenRewrite and healing if build fails.",
  "selected_recipes": [
    "org.openrewrite.java.migrate.UpgradeToJava17"
  ],
  "human_gate": false,
  "max_parallel": 3
}
JSON

echo "[mods-plan] Wrote plan to $out"
