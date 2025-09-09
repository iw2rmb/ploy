#!/usr/bin/env bash
set -euo pipefail

OUT_DIR="${OUTPUT_DIR:-/workspace/out}"
CTX_DIR="${CONTEXT_DIR:-/workspace/context}"
RUN_ID_STR="${RUN_ID:-}"

mkdir -p "$OUT_DIR"

log() { echo "[LG-STUB] $*"; }

if [[ "$RUN_ID_STR" == *"planner"* ]]; then
  log "Detected planner run (RUN_ID=$RUN_ID_STR)"
  PLAN_ID="plan-$(date +%s)"
  cat >"$OUT_DIR/plan.json" <<EOF
{
  "plan_id": "$PLAN_ID",
  "options": [
    {"id": "llm-1", "type": "llm-exec"},
    {"id": "orw-1", "type": "orw-gen", "inputs": {"recipe_config": {"class": "org.openrewrite.java.RemoveUnusedImports", "coords": "org.openrewrite.recipe:rewrite-java-latest:latest", "timeout": "5m"}}}
  ]
}
EOF
  log "Wrote plan.json to $OUT_DIR"
elif [[ "$RUN_ID_STR" == *"reducer"* ]]; then
  log "Detected reducer run (RUN_ID=$RUN_ID_STR)"
  cat >"$OUT_DIR/next.json" <<EOF
{
  "action": "stop",
  "notes": "stub reducer"
}
EOF
  log "Wrote next.json to $OUT_DIR"
elif [[ "$RUN_ID_STR" == *"llm-exec"* ]]; then
  log "Detected llm-exec run (RUN_ID=$RUN_ID_STR)"
  cat >"$OUT_DIR/diff.patch" <<'EOF'
diff --git a/pom.xml b/pom.xml
index 1111111..2222222 100644
--- a/pom.xml
+++ b/pom.xml
@@ -1,3 +1,3 @@
-<!-- original -->
+<!-- modified by langgraph stub -->
 <project></project>
EOF
  log "Wrote diff.patch to $OUT_DIR"
else
  log "Unknown mode (RUN_ID=$RUN_ID_STR). Defaulting to planner output."
  PLAN_ID="plan-$(date +%s)"
  echo "{\"plan_id\":\"$PLAN_ID\",\"options\":[{\"id\":\"llm-1\",\"type\":\"llm-exec\"}]}" >"$OUT_DIR/plan.json"
fi

log "Done"
