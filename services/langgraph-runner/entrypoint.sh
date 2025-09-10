#!/usr/bin/env bash
set -euo pipefail

OUT_DIR="${OUTPUT_DIR:-/workspace/out}"
CTX_DIR="${CONTEXT_DIR:-/workspace/context}"
RUN_ID_STR="${RUN_ID:-}"
CONTROLLER_URL="${CONTROLLER_URL:-}"
EXECUTION_ID="${TRANSFLOW_EXECUTION_ID:-}"

mkdir -p "$OUT_DIR"

log() { echo "[LG-STUB] $*"; }

post_event() {
  local level="$1"; shift
  local phase="$1"; shift
  local step="$1"; shift
  local msg="$1"
  if [[ -n "$CONTROLLER_URL" && -n "$EXECUTION_ID" ]]; then
    curl -sS -X POST "${CONTROLLER_URL}/transflow/event" \
      -H "Content-Type: application/json" \
      -d "{\"execution_id\":\"${EXECUTION_ID}\",\"phase\":\"${phase}\",\"step\":\"${step}\",\"level\":\"${level}\",\"message\":\"${msg}\",\"job_name\":\"${RUN_ID_STR}\"}" \
      -o /dev/null || true
  fi
}

on_exit() {
  code=$?
  local phase=""
  local step=""
  if [[ "$RUN_ID_STR" == *"planner"* ]]; then phase="planner"; step="planner"; fi
  if [[ "$RUN_ID_STR" == *"reducer"* ]]; then phase="reducer"; step="reducer"; fi
  if [[ "$RUN_ID_STR" == *"llm-exec"* ]]; then phase="llm-exec"; step="llm-exec"; fi
  if [[ $code -eq 0 ]]; then
    post_event "info" "$phase" "$step" "job completed"
  else
    post_event "error" "$phase" "$step" "job failed"
  fi
}

trap on_exit EXIT

if [[ "$RUN_ID_STR" == *"planner"* ]]; then
  log "Detected planner run (RUN_ID=$RUN_ID_STR)"
  post_event "info" "planner" "planner" "job started"
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
  post_event "info" "reducer" "reducer" "job started"
  cat >"$OUT_DIR/next.json" <<EOF
{
  "action": "stop",
  "notes": "stub reducer"
}
EOF
  log "Wrote next.json to $OUT_DIR"
elif [[ "$RUN_ID_STR" == *"llm-exec"* ]]; then
  log "Detected llm-exec run (RUN_ID=$RUN_ID_STR)"
  post_event "info" "llm-exec" "llm-exec" "job started"
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
