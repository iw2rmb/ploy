#!/usr/bin/env bash
set -euo pipefail

OUT_DIR="${OUTPUT_DIR:-/workspace/out}"
CTX_DIR="${CONTEXT_DIR:-/workspace/context}"
RUN_ID_STR="${RUN_ID:-}"
CONTROLLER_URL="${CONTROLLER_URL:-}"
MOD_ID_ENV="${MOD_ID:-}"
SBOM_LATEST_URL="${SBOM_LATEST_URL:-}"
SEAWEEDFS_URL="${SEAWEEDFS_URL:-${PLOY_SEAWEEDFS_URL:-}}"

# Curl defaults: fail fast in tests (Conn timeout 3s, total 12s, 2 retries)
CURL_OPTS=(--connect-timeout 3 -m 12 --retry 2 --retry-delay 1 --retry-all-errors -sS)

mkdir -p "$OUT_DIR"

# Early startup diagnostics to confirm entrypoint is running and env is wired
echo "[LG-STUB] ENTRYPOINT starting: RUN_ID=${RUN_ID_STR:-<empty>} MOD_ID=${MOD_ID_ENV:-<empty>} SEAWEEDFS_URL=${SEAWEEDFS_URL:-<empty>} CONTROLLER_URL=${CONTROLLER_URL:-<empty>}"

log() { echo "[LG-STUB] $*"; }

post_event() {
  local level="$1"; shift
  local phase="$1"; shift
  local step="$1"; shift
  local msg="$1"
  if [[ -n "$CONTROLLER_URL" && -n "$MOD_ID_ENV" ]]; then
    curl "${CURL_OPTS[@]}" -X POST "${CONTROLLER_URL%/}/mods/${MOD_ID_ENV}/events" \
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
  if curl "${CURL_OPTS[@]}" "$pointer_url" -o "$pointer_json"; then
    # Extract storage_key (very small JSON; avoid jq dependency)
    local storage_key
    storage_key=$(sed -n 's/.*"storage_key"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$pointer_json" | head -n1)
    if [ -n "$storage_key" ]; then
      local ok=0
      if [ -n "$SEAWEEDFS_URL" ]; then
        local sbom_url="${SEAWEEDFS_URL%/}/$storage_key"
        curl "${CURL_OPTS[@]}" "$sbom_url" -o "$sbom_json" && ok=1 || ok=0
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
        curl "${CURL_OPTS[@]}" "$proxy_url" -o "$sbom_json" && ok=1 || ok=0
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
  post_event "info" "planner" "planner" "env PLOY_SEAWEEDFS_URL=${SEAWEEDFS_URL:-<empty>}"
  post_event "info" "planner" "planner" "env MOD_ID=${MOD_ID_ENV:-<empty>} RUN_ID=${RUN_ID_STR}"

  # Connectivity check to SeaweedFS (HEAD, fallback GET) for visibility
  if [ -n "${SEAWEEDFS_URL:-}" ]; then
    CHECK_URL="${SEAWEEDFS_URL%/}/"
    HEAD_CODE=$(curl "${CURL_OPTS[@]}" -o /dev/null -w '%{http_code}' -I "$CHECK_URL" || echo "000")
    if [ "$HEAD_CODE" = "200" ] || [ "$HEAD_CODE" = "204" ]; then
      post_event "info" "planner" "planner" "seaweedfs connectivity (HEAD) status=${HEAD_CODE} url=${CHECK_URL}"
    else
      # Fallback to GET with short timeout and capture a snippet
      GET_CODE=$(curl "${CURL_OPTS[@]}" -o /tmp/seaweed_check.out -w '%{http_code}' "$CHECK_URL" || echo "000")
      SNIP=$(tr -d '\r' </tmp/seaweed_check.out | head -c 200)
      post_event "warn" "planner" "planner" "seaweedfs connectivity (HEAD=${HEAD_CODE}, GET=${GET_CODE}) url=${CHECK_URL} body=${SNIP}"
      rm -f /tmp/seaweed_check.out || true
    fi
  else
    post_event "warn" "planner" "planner" "seaweedfs connectivity skipped: SEAWEEDFS_URL empty"
  fi
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
  # Upload planner plan.json to SeaweedFS for controller collection (log HTTP status)
  if [ -s "$OUT_DIR/plan.json" ] && [ -n "$SEAWEEDFS_URL" ] && [ -n "$MOD_ID_ENV" ] && [ -n "$RUN_ID_STR" ]; then
    KEY="mods/${MOD_ID_ENV}/planner/${RUN_ID_STR}/plan.json"
    URL="${SEAWEEDFS_URL%/}/artifacts/${KEY}"
    log "Uploading plan.json to $URL"
    HTTP_CODE=$(curl "${CURL_OPTS[@]}" -w '%{http_code}' -X PUT -H 'Content-Type: application/json' --data-binary @"$OUT_DIR/plan.json" "$URL" -o /tmp/plan_upload.out || echo "000")
    if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ] || [ "$HTTP_CODE" = "204" ]; then
      post_event "info" "planner" "planner" "uploaded plan to ${KEY} (status ${HTTP_CODE})"
    else
      ERR_MSG=$(tr -d '\r' </tmp/plan_upload.out | head -c 300)
      post_event "error" "planner" "planner" "plan upload failed (status ${HTTP_CODE}) to ${KEY}: ${ERR_MSG}"
    fi
    rm -f /tmp/plan_upload.out || true
  fi
elif [[ "$RUN_ID_STR" == *"reducer"* ]]; then
  log "Detected reducer run (RUN_ID=$RUN_ID_STR)"
  post_event "info" "reducer" "reducer" "job started"
  # Prefer applying the LLM branch (llm-1)
  cat >"$OUT_DIR/next.json" <<EOF
{
  "action": "apply",
  "step_id": "llm-1"
}
EOF
  log "Wrote next.json to $OUT_DIR"
  # Upload reducer next.json to SeaweedFS for controller collection (log HTTP status)
  if [ -s "$OUT_DIR/next.json" ] && [ -n "$SEAWEEDFS_URL" ] && [ -n "$MOD_ID_ENV" ] && [ -n "$RUN_ID_STR" ]; then
    KEY="mods/${MOD_ID_ENV}/reducer/${RUN_ID_STR}/next.json"
    URL="${SEAWEEDFS_URL%/}/artifacts/${KEY}"
    log "Uploading next.json to $URL"
    HTTP_CODE=$(curl "${CURL_OPTS[@]}" -w '%{http_code}' -X PUT -H 'Content-Type: application/json' --data-binary @"$OUT_DIR/next.json" "$URL" -o /tmp/next_upload.out || echo "000")
    if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ] || [ "$HTTP_CODE" = "204" ]; then
      post_event "info" "reducer" "reducer" "uploaded next to ${KEY} (status ${HTTP_CODE})"
    else
      ERR_MSG=$(tr -d '\r' </tmp/next_upload.out | head -c 300)
      post_event "error" "reducer" "reducer" "next upload failed (status ${HTTP_CODE}) to ${KEY}: ${ERR_MSG}"
    fi
    rm -f /tmp/next_upload.out || true
  fi
elif [[ "$RUN_ID_STR" == *"llm-exec"* ]]; then
  log "Detected llm-exec run (RUN_ID=$RUN_ID_STR)"
  post_event "info" "llm-exec" "llm-exec" "job started"
  log "CTX_DIR=$CTX_DIR OUT_DIR=$OUT_DIR"
  post_event "info" "llm-exec" "llm-exec" "env CTX_DIR=$CTX_DIR OUT_DIR=$OUT_DIR"
  post_event "info" "llm-exec" "llm-exec" "env PLOY_SEAWEEDFS_URL=${SEAWEEDFS_URL:-<empty>}"
  mkdir -p "$OUT_DIR" || true
  ls -la "$CTX_DIR" || true
  ls -la "$OUT_DIR" || true
  # 1) If a diff is provided via context, use it.
  # 2) Else, try to parse inputs.json.last_error to compute a minimal fix
  #    by editing the first offending Java file line (comment it out).
  # 3) Else, emit a harmless no-op patch.
  if [ -s "$CTX_DIR/diff.patch" ]; then
    log "Using provided diff from context (diff.patch)"
    cp "$CTX_DIR/diff.patch" "$OUT_DIR/diff.patch"
    post_event "info" "llm-exec" "llm-exec" "using context diff.patch"
  else
    # Attempt to parse build error and derive target file + line
    TARGET_FILE=""
    TARGET_REL=""
    TARGET_LINE=""
    if [ -s "$CTX_DIR/inputs.json" ]; then
      # Prefer explicit top-level hints if present
      CAND=$(sed -n 's/.*"first_error_file"[[:space:]]*:[[:space:]]*"\([^\"]*\)".*/\1/p' "$CTX_DIR/inputs.json" | head -n1)
      LINE=$(sed -n 's/.*"first_error_line"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p' "$CTX_DIR/inputs.json" | head -n1)
      # Else try structured errors array
      if [ -z "$CAND" ] || [ -z "$LINE" ]; then
        FIRST_ERR_JSON=$(awk 'BEGIN{RS="}"} /"errors"[[:space:]]*:/ {print $0"}"; exit}' "$CTX_DIR/inputs.json" 2>/dev/null || true)
        if [ -n "$FIRST_ERR_JSON" ]; then
          CAND=$(printf "%s" "$FIRST_ERR_JSON" | sed -n 's/.*"file"[[:space:]]*:[[:space:]]*"\([^"]\+\)".*/\1/p' | head -n1)
          LINE=$(printf "%s" "$FIRST_ERR_JSON" | sed -n 's/.*"line"[[:space:]]*:[[:space:]]*\([0-9]\+\).*/\1/p' | head -n1)
        fi
      fi
      # Fallback: extract from raw last_error stderr
      if [ -z "$CAND" ] || [ -z "$LINE" ]; then
        ERR=$(awk 'BEGIN{RS=""} /"last_error"/ {print}' "$CTX_DIR/inputs.json" 2>/dev/null)
        # Find first Java file path
        CAND=$(printf "%s" "$ERR" | sed -n 's/.*\([A-Za-z0-9_\.\/-]\+\.java\).*/\1/p' | head -n1)
        # Accept either :123 or :[123,456] (portable BSD/GNU sed)
        LINE=$(printf "%s" "$ERR" | sed -n 's/.*\\.java:\\[\\([0-9][0-9]*\\),.*/\\1/p' | head -n1)
        if [ -z "$LINE" ]; then
          LINE=$(printf "%s" "$ERR" | sed -n 's/.*\\.java:\\([0-9][0-9]*\\).*/\\1/p' | head -n1)
        fi
      fi
      if [ -n "$CAND" ]; then
        # Normalize absolute paths to repo-relative when possible
        CAND_REL="$CAND"
        if [ "${CAND_REL#/}" != "$CAND_REL" ]; then
          # Prefer stripping the common temp repo segment: */repo/<relpath>
          case "$CAND_REL" in
            */repo/*)
              CAND_REL="${CAND_REL#*/repo/}"
              ;;
            */src/*)
              # Ensure it begins with src/
              CAND_REL="src/${CAND_REL#*/src/}"
              ;;
          esac
        fi
        # If a source snapshot was provided, prefer it to ensure exact diff
        if [ -f "$CTX_DIR/sources/$CAND_REL" ]; then
          TARGET_FILE="$CTX_DIR/sources/$CAND_REL"
          TARGET_REL="$CAND_REL"
        elif [ -f "$CTX_DIR/sources/$CAND" ]; then
          TARGET_FILE="$CTX_DIR/sources/$CAND"
          TARGET_REL="$CAND"
        fi
      fi
      if [ -z "$TARGET_FILE" ]; then
        post_event "warn" "llm-exec" "llm-exec" "could not resolve offending file from inputs.json"
      fi
      if [ -n "$LINE" ]; then
        TARGET_LINE="$LINE"
      fi
    fi
    if [ -z "$TARGET_FILE" ] && [ -n "$CAND" ] && [ -d "$CTX_DIR/sources" ]; then
      BASENAME="$(basename "$CAND")"
      if [ -n "$BASENAME" ]; then
        FOUND=$(find "$CTX_DIR/sources" -type f -name "$BASENAME" | head -n1 || true)
        if [ -n "$FOUND" ] && [ -f "$FOUND" ]; then
          TARGET_FILE="$FOUND"
          TARGET_REL="${FOUND#"$CTX_DIR/sources/"}"
          TARGET_REL="${TARGET_REL#./}"
        fi
      fi
    fi
    if [ -z "$TARGET_REL" ]; then
      if [ -n "$CAND_REL" ]; then
        TARGET_REL="$CAND_REL"
      elif [ -n "$CAND" ]; then
        TARGET_REL="$CAND"
      fi
    fi
    if [ -n "$TARGET_FILE" ] && [ -s "$TARGET_FILE" ] && [ -n "$TARGET_LINE" ]; then
      # Strategy: generate minimal edit by commenting the offending line
      # and the immediate next line (covers `obj` usage).
      TMP_NEW="$(mktemp)"
      awk -v ln="$TARGET_LINE" '{ if (NR==ln || NR==ln+1) { print "// " $0 } else { print $0 } }' "$TARGET_FILE" > "$TMP_NEW"
      # Try unified diff first
      if command -v diff >/dev/null 2>&1; then
        diff -u --label "a/$TARGET_REL" --label "b/$TARGET_REL" "$TARGET_FILE" "$TMP_NEW" > "$OUT_DIR/diff.patch" || true
      fi
      if [ ! -s "$OUT_DIR/diff.patch" ]; then
        # Manual minimal hunk around target line
        TOTAL=$(wc -l < "$TARGET_FILE" | tr -d ' ')
        S=$(( TARGET_LINE>1 ? TARGET_LINE-1 : TARGET_LINE ))
        E=$(( TARGET_LINE+1<=TOTAL ? TARGET_LINE+1 : TARGET_LINE ))
        ORIG_LINE=$(sed -n "${TARGET_LINE}p" "$TARGET_FILE")
        NEXT_LINE=$(sed -n "$((TARGET_LINE+1))p" "$TARGET_FILE")
        printf "--- a/%s\n+++ b/%s\n" "$TARGET_REL" "$TARGET_REL" > "$OUT_DIR/diff.patch"
        printf "@@ -%d,%d +%d,%d @@\n" "$S" "$((E-S+1))" "$S" "$((E-S+1))" >> "$OUT_DIR/diff.patch"
        if [ "$S" -lt "$TARGET_LINE" ]; then sed -n "${S},$((TARGET_LINE-1))p" "$TARGET_FILE" | sed 's/^/ /' >> "$OUT_DIR/diff.patch"; fi
        printf "-%s\n+// %s\n" "$ORIG_LINE" "$ORIG_LINE" >> "$OUT_DIR/diff.patch"
        if [ -n "$NEXT_LINE" ]; then printf "-%s\n+// %s\n" "$NEXT_LINE" "$NEXT_LINE" >> "$OUT_DIR/diff.patch"; fi
      fi
      rm -f "$TMP_NEW" || true
      post_event "info" "llm-exec" "llm-exec" "generated minimal edit patch (comment offending lines)"
    else
      log "No actionable inputs; writing minimal placeholder patch"
      cat >"$OUT_DIR/diff.patch" <<'EOF'
diff --git a/.llm-healing b/.llm-healing
new file mode 100644
index 0000000..e69de29
--- /dev/null
+++ b/.llm-healing
# LLM healing produced no-op patch
EOF
      post_event "warn" "llm-exec" "llm-exec" "no actionable inputs; wrote placeholder"
    fi
  fi
  log "Wrote diff.patch to $OUT_DIR"
  # Upload to SeaweedFS step-scoped key to mirror ORW behavior (log HTTP status)
  if [ -s "$OUT_DIR/diff.patch" ] && [ -n "$SEAWEEDFS_URL" ] && [ -n "$MOD_ID_ENV" ]; then
    # Derive branch ID from RUN_ID: strip llm-exec- prefix and trailing -<ts>
    BRANCH_ID=$(echo "$RUN_ID_STR" | sed -E 's/^llm-exec-//' | sed -E 's/-[0-9]+$//')
    STEP_ID="$RUN_ID_STR"
    KEY="mods/${MOD_ID_ENV}/branches/${BRANCH_ID}/steps/${STEP_ID}/diff.patch"
    URL="${SEAWEEDFS_URL%/}/artifacts/${KEY}"
    log "Uploading diff to $URL"
    HTTP_CODE=$(curl "${CURL_OPTS[@]}" -w '%{http_code}' -X PUT -H 'Content-Type: text/plain' --data-binary @"$OUT_DIR/diff.patch" "$URL" -o /tmp/diff_upload.out || echo "000")
    if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ] || [ "$HTTP_CODE" = "204" ]; then
      post_event "info" "llm-exec" "llm-exec" "uploaded diff to ${KEY} (status ${HTTP_CODE})"
    else
      ERR_MSG=$(tr -d '\r' </tmp/diff_upload.out | head -c 300)
      post_event "error" "llm-exec" "llm-exec" "diff upload failed (status ${HTTP_CODE}) to ${KEY}: ${ERR_MSG}"
    fi
    rm -f /tmp/diff_upload.out || true
  fi
else
  log "Unknown mode (RUN_ID=$RUN_ID_STR). Defaulting to planner output."
  PLAN_ID="plan-$(date +%s)"
  echo "{\"plan_id\":\"$PLAN_ID\",\"options\":[{\"id\":\"llm-1\",\"type\":\"llm-exec\"}]}" >"$OUT_DIR/plan.json"
fi

log "Done"
