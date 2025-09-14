#!/usr/bin/env bash
set -euo pipefail

OUT_DIR="${OUTPUT_DIR:-/workspace/out}"
CTX_DIR="${CONTEXT_DIR:-/workspace/context}"
RUN_ID_STR="${RUN_ID:-}"
CONTROLLER_URL="${CONTROLLER_URL:-}"
EXECUTION_ID="${MODS_EXECUTION_ID:-}"
SBOM_LATEST_URL="${SBOM_LATEST_URL:-}"
SEAWEEDFS_URL="${SEAWEEDFS_URL:-${PLOY_SEAWEEDFS_URL:-}}"

mkdir -p "$OUT_DIR"

log() { echo "[LG-STUB] $*"; }

post_event() {
  local level="$1"; shift
  local phase="$1"; shift
  local step="$1"; shift
  local msg="$1"
  if [[ -n "$CONTROLLER_URL" && -n "$EXECUTION_ID" ]]; then
    curl -sS -X POST "${CONTROLLER_URL%/}/mods/${EXECUTION_ID}/events" \
      -H "Content-Type: application/json" \
      -d "{\"phase\":\"${phase}\",\"step\":\"${step}\",\"level\":\"${level}\",\"message\":\"${msg}\",\"job_name\":\"${RUN_ID_STR}\"}" \
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

# Best-effort: fetch latest SBOM for this repo using SBOM_LATEST_URL and store summary
fetch_sbom_if_available() {
  local pointer_url="$SBOM_LATEST_URL"
  local ctx="$CTX_DIR"
  [ -z "$pointer_url" ] && return 0
  mkdir -p "$ctx" 2>/dev/null || true
  local pointer_json="$ctx/sbom_pointer.json"
  local sbom_json="$ctx/sbom.json"
  local summary_json="$ctx/sbom_summary.json"
  # Fetch pointer
  if curl -sS -m 8 "$pointer_url" -o "$pointer_json"; then
    # Extract storage_key (very small JSON; avoid jq dependency)
    local storage_key
    storage_key=$(sed -n 's/.*"storage_key"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$pointer_json" | head -n1)
    if [ -n "$storage_key" ]; then
      local ok=0
      if [ -n "$SEAWEEDFS_URL" ]; then
        local sbom_url="${SEAWEEDFS_URL%/}/$storage_key"
        curl -sS -m 15 "$sbom_url" -o "$sbom_json" && ok=1 || ok=0
      fi
      if [ "$ok" -ne 1 ] && [ -n "$CONTROLLER_URL" ]; then
        # Controller proxy fallback; best-effort URL encode
        local enc_key
        enc_key=$(python3 - <<PY
import sys,urllib.parse
print(urllib.parse.quote(sys.argv[1]))
PY
"$storage_key" 2>/dev/null || echo "$storage_key")
        local proxy_url="${CONTROLLER_URL%/}/sbom/download?key=${enc_key}"
        curl -sS -m 15 "$proxy_url" -o "$sbom_json" && ok=1 || ok=0
      fi
      if [ "$ok" -eq 1 ]; then
        post_event "info" "$(phase_from_runid)" "sbom" "downloaded latest SBOM"
        "$(dirname "$0")/summarize_sbom.sh" "$sbom_json" "$summary_json" || true
      else
        post_event "warn" "$(phase_from_runid)" "sbom" "failed to fetch SBOM"
      fi
    else
      post_event "warn" "$(phase_from_runid)" "sbom" "pointer missing storage_key"
    fi
  else
    post_event "warn" "$(phase_from_runid)" "sbom" "failed to fetch SBOM pointer"
  fi
}

phase_from_runid() {
  local p=""
  if [[ "$RUN_ID_STR" == *"planner"* ]]; then p="planner"; fi
  if [[ "$RUN_ID_STR" == *"reducer"* ]]; then p="reducer"; fi
  if [[ "$RUN_ID_STR" == *"llm-exec"* ]]; then p="llm-exec"; fi
  echo "$p"
}

trap on_exit EXIT

# Try to fetch SBOM early so tasks can use it
fetch_sbom_if_available || true

# Auto-generate a small prompt snippet if summary exists and heuristic matches
inject_prompt_hint() {
  [ ! -s "$CTX_DIR/sbom_summary.json" ] && return 0
  local want=0
  # Explicit opt-in
  if [ "${MODS_USE_SBOM_CONTEXT:-}" = "1" ]; then want=1; fi
  # Heuristic: look into planner inputs when in planner phase
  if [ $want -eq 0 ] && [[ "$RUN_ID_STR" == *"planner"* ]]; then
    if [ -s "$CTX_DIR/inputs.json" ]; then
      if grep -Eiq '(upgrade|dependency|dependencies|cve|security|license|build fail|build error)' "$CTX_DIR/inputs.json"; then
        want=1
      fi
    fi
  fi
  [ $want -eq 0 ] && return 0
  local SNIPPET="$CTX_DIR/prompt_sbom.txt"
  local bytes crit high
  bytes=$(sed -n 's/.*"bytes"[[:space:]]*:[[:space:]]*\([0-9]\+\).*/\1/p' "$CTX_DIR/sbom_summary.json" | head -n1)
  crit=$(sed -n 's/.*"critical"[[:space:]]*:[[:space:]]*\([0-9]\+\).*/\1/p' "$CTX_DIR/sbom_summary.json" | head -n1)
  high=$(sed -n 's/.*"high"[[:space:]]*:[[:space:]]*\([0-9]\+\).*/\1/p' "$CTX_DIR/sbom_summary.json" | head -n1)
  echo "SBOM summary: size=${bytes:-0}B, vulns(crit=${crit:-0}, high=${high:-0})" > "$SNIPPET"
  post_event "info" "$(phase_from_runid)" "sbom" "prompt hint generated"
}

inject_prompt_hint || true

# If we're in planner mode, automatically combine any user prompt with SBOM hint
enrich_planner_prompt() {
  [[ "$RUN_ID_STR" != *"planner"* ]] && return 0
  local base="$CTX_DIR/prompt_user.txt"
  local hint="$CTX_DIR/prompt_sbom.txt"
  local final="$CTX_DIR/prompt_final.txt"
  # Only act if SBOM hint exists
  if [ -s "$hint" ]; then
    mkdir -p "$CTX_DIR" 2>/dev/null || true
    if [ -s "$base" ]; then
      {
        cat "$base"
        echo
        echo
        echo "[Context — SBOM Summary]"
        cat "$hint"
      } > "$final"
    else
      {
        echo "[Planner Prompt]"
        echo "Provide a healing plan for the reported build issue."
        echo
        echo "[Context — SBOM Summary]"
        cat "$hint"
      } > "$final"
    fi
    post_event "info" "planner" "planner" "prompt enriched with SBOM hint"
  fi
}

enrich_planner_prompt || true

if [[ "$RUN_ID_STR" == *"planner"* ]]; then
  log "Detected planner run (RUN_ID=$RUN_ID_STR)"
  post_event "info" "planner" "planner" "job started"
  PLAN_ID="plan-$(date +%s)"
  # Flag whether we included an SBOM prompt hint
  PROMPT_HINT="false"
  if [ -s "$CTX_DIR/prompt_final.txt" ]; then PROMPT_HINT="true"; fi
  cat >"$OUT_DIR/plan.json" <<EOF
{
  "plan_id": "$PLAN_ID",
  "prompt_hint_included": $PROMPT_HINT,
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
  log "CTX_DIR=$CTX_DIR OUT_DIR=$OUT_DIR"
  mkdir -p "$OUT_DIR" || true
  ls -la "$CTX_DIR" || true
  ls -la "$OUT_DIR" || true
  # Attempt to generate a deletion patch for the known failing source to heal build
  TARGET_REL="src/healing/java/e2e/FailHealing.java"
  if [ -f "$CTX_DIR/$TARGET_REL" ]; then
    log "Generating healing diff to delete $TARGET_REL"
    (
      cd "$CTX_DIR" >/dev/null 2>&1 || exit 0
      # Use git diff --no-index to produce a proper unified diff against /dev/null (deletion)
      if command -v git >/dev/null 2>&1; then
        git -c core.safecrlf=false diff --no-index -- "$TARGET_REL" /dev/null > "$OUT_DIR/diff.patch" 2>/dev/null || true
      fi
    )
  fi
  # Fallback: if no healing target found or diff is empty, produce a minimal noop patch comment to keep pipeline moving
  if [ ! -s "$OUT_DIR/diff.patch" ]; then
    log "Healing target not found or diff empty; writing minimal placeholder patch"
    cat >"$OUT_DIR/diff.patch" <<'EOF'
diff --git a/.llm-healing b/.llm-healing
new file mode 100644
index 0000000..e69de29
--- /dev/null
+++ b/.llm-healing
# LLM healing produced no-op patch
EOF
  fi
  log "Wrote diff.patch to $OUT_DIR"
else
  log "Unknown mode (RUN_ID=$RUN_ID_STR). Defaulting to planner output."
  PLAN_ID="plan-$(date +%s)"
  echo "{\"plan_id\":\"$PLAN_ID\",\"options\":[{\"id\":\"llm-1\",\"type\":\"llm-exec\"}]}" >"$OUT_DIR/plan.json"
fi

log "Done"
