#!/usr/bin/env bash
# Unit tests for images/codex/entrypoint.sh (container command: codex)
# Tests CLI flag detection, JSONL event capture, session ID extraction,
# and run manifest/session metadata.
#
# Usage: bash tests/unit/mig_codex_sh_test.sh
#
# Exit codes:
#   0: All tests passed
#   1: One or more tests failed

set -uo pipefail

ROOT_DIR=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
SCRIPT="$ROOT_DIR/images/codex/entrypoint.sh"

# Create a temporary copy of the codex entrypoint for test execution.
create_test_script() {
  local tmp_script
  tmp_script=$(mktemp)
  cat "$SCRIPT" > "$tmp_script"
  chmod +x "$tmp_script"
  echo "$tmp_script"
}

# Create a temporary prompt file for direct codex mode tests.
# Hydra contract: prompt is delivered via --prompt-file or /in/codex-prompt.txt.
create_prompt_file() {
  local content="${1:-test prompt}"
  local tmp_prompt
  tmp_prompt=$(mktemp)
  printf '%s' "$content" > "$tmp_prompt"
  echo "$tmp_prompt"
}

# Track test results
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

pass() {
  TESTS_PASSED=$((TESTS_PASSED + 1))
  echo "  ✓ $1"
}

fail() {
  TESTS_FAILED=$((TESTS_FAILED + 1))
  echo "  ✗ $1: $2"
}

run_test() {
  TESTS_RUN=$((TESTS_RUN + 1))
}

# ─────────────────────────────────────────────────────────────────────────────
# Test: --help flag displays usage
# ─────────────────────────────────────────────────────────────────────────────
test_help_flag() {
  run_test
  local output
  output=$(bash "$SCRIPT" --help 2>&1) || true
  if echo "$output" | grep -q -- "--input"; then
    pass "help displays --input option"
  else
    fail "help displays --input option" "expected --input in output"
    return
  fi
  if echo "$output" | grep -q -- "--out"; then
    pass "help displays --out option"
  else
    fail "help displays --out option" "expected --out in output"
    return
  fi
  if echo "$output" | grep -q -- "--prompt-file"; then
    pass "help displays --prompt-file option"
  else
    fail "help displays --prompt-file option" "expected --prompt-file in output"
    return
  fi
  if echo "$output" | grep -q "CODEX_RESUME"; then
    pass "help mentions CODEX_RESUME env"
  else
    fail "help mentions CODEX_RESUME env" "expected CODEX_RESUME in output"
    return
  fi
  if echo "$output" | grep -q "Hydra"; then
    pass "help mentions Hydra file delivery"
  else
    fail "help mentions Hydra file delivery" "expected Hydra in output"
    return
  fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Test: Script detects --json flag from codex --help output
# ─────────────────────────────────────────────────────────────────────────────
test_json_flag_detection() {
  run_test

  # Create temp directories for test
  local tmp_bin tmp_out tmp_ws
  tmp_bin=$(mktemp -d)
  tmp_out=$(mktemp -d)
  tmp_ws=$(mktemp -d)

  # Mock codex CLI that advertises --json and outputs JSON events
  cat > "$tmp_bin/codex" <<'MOCKCODEX'
#!/bin/bash
if [[ "$1" == "exec" && "$2" == "--help" ]]; then
  echo "Usage: codex exec [OPTIONS]"
  echo "  --yolo                Skip confirmations"
  echo "  --json                Output JSON events"
  echo "  --add-dir <dir>       Add directory to context"
  exit 0
fi
# Check if --json was passed and output JSON event
for arg in "$@"; do
  if [[ "$arg" == "--json" ]]; then
    echo '{"type":"thread.started","thread_id":"thread_abc123"}'
    exit 0
  fi
done
echo "No JSON output"
exit 0
MOCKCODEX
  chmod +x "$tmp_bin/codex"

  # Use temp dir as HOME to avoid /root permission issues
  local tmp_home tmp_script
  tmp_home=$(mktemp -d)
  tmp_script=$(create_test_script)

  local tmp_prompt output exit_code
  tmp_prompt=$(create_prompt_file)
  output=$( (
    export HOME="$tmp_home"
    export PATH="$tmp_bin:$PATH"
    bash "$tmp_script" --input "$tmp_ws" --out "$tmp_out" --prompt-file "$tmp_prompt"
  ) 2>&1 )
  exit_code=$?

  # Check that JSONL file was created and contains the event
  if [[ -f "$tmp_out/codex-events.jsonl" ]] && grep -q "thread.started" "$tmp_out/codex-events.jsonl"; then
    pass "--json flag detected and JSONL captured"
  else
    fail "--json flag detection" "JSONL file missing or empty: $output"
  fi

  rm -rf "$tmp_bin" "$tmp_out" "$tmp_ws" "$tmp_home" "$tmp_script" "$tmp_prompt"
}

# ─────────────────────────────────────────────────────────────────────────────
# Test: Script extracts session ID from JSONL events
# ─────────────────────────────────────────────────────────────────────────────
test_session_id_extraction() {
  run_test

  # Create temp directories for test
  local tmp_bin tmp_out tmp_ws
  tmp_bin=$(mktemp -d)
  tmp_out=$(mktemp -d)
  tmp_ws=$(mktemp -d)

  # Mock codex CLI that outputs JSON with thread_id
  cat > "$tmp_bin/codex" <<'MOCKCODEX'
#!/bin/bash
if [[ "$1" == "exec" && "$2" == "--help" ]]; then
  echo "Usage: codex exec [OPTIONS]"
  echo "  --yolo                Skip confirmations"
  echo "  --json                Output JSON events"
  exit 0
fi
# Output JSON events with thread_id
echo '{"type":"thread.started","thread_id":"thread_test_session_xyz789"}'
echo '{"type":"message","content":"Hello"}'
exit 0
MOCKCODEX
  chmod +x "$tmp_bin/codex"

  # Use temp dir as HOME to avoid /root permission issues
  local tmp_home tmp_script
  tmp_home=$(mktemp -d)
  tmp_script=$(create_test_script)

  local tmp_prompt exit_code
  tmp_prompt=$(create_prompt_file)
  (
    export HOME="$tmp_home"
    export PATH="$tmp_bin:$PATH"
    bash "$tmp_script" --input "$tmp_ws" --out "$tmp_out" --prompt-file "$tmp_prompt"
  ) >/dev/null 2>&1
  exit_code=$?

  # Check that session ID file was created with correct content
  if [[ -f "$tmp_out/codex-session.txt" ]]; then
    local session_id
    session_id=$(cat "$tmp_out/codex-session.txt")
    if [[ "$session_id" == "thread_test_session_xyz789" ]]; then
      pass "session ID extracted to codex-session.txt"
    else
      fail "session ID extraction" "got: $session_id, expected: thread_test_session_xyz789"
    fi
  else
    fail "session ID extraction" "codex-session.txt not created"
  fi

  rm -rf "$tmp_bin" "$tmp_out" "$tmp_ws" "$tmp_home" "$tmp_script" "$tmp_prompt"
}

# ─────────────────────────────────────────────────────────────────────────────
# Test: Manifest includes session_id and resumed fields
# ─────────────────────────────────────────────────────────────────────────────
test_manifest_contains_new_fields() {
  run_test

  # Create temp directories for test
  local tmp_bin tmp_out tmp_ws
  tmp_bin=$(mktemp -d)
  tmp_out=$(mktemp -d)
  tmp_ws=$(mktemp -d)

  # Mock codex CLI with full feature support
  cat > "$tmp_bin/codex" <<'MOCKCODEX'
#!/bin/bash
if [[ "$1" == "exec" && "$2" == "--help" ]]; then
  echo "Usage: codex exec [OPTIONS]"
  echo "  --yolo                Skip confirmations"
  echo "  --json                Output JSON events"
  echo "  --output-last-message Write last message to file"
  exit 0
fi
# Output thread.started event for session extraction
echo '{"type":"thread.started","thread_id":"thread_manifest_test"}'
exit 0
MOCKCODEX
  chmod +x "$tmp_bin/codex"

  # Use temp dir as HOME to avoid /root permission issues
  local tmp_home tmp_script
  tmp_home=$(mktemp -d)
  tmp_script=$(create_test_script)

  local tmp_prompt exit_code
  tmp_prompt=$(create_prompt_file)
  (
    export HOME="$tmp_home"
    export PATH="$tmp_bin:$PATH"
    bash "$tmp_script" --input "$tmp_ws" --out "$tmp_out" --prompt-file "$tmp_prompt"
  ) >/dev/null 2>&1
  exit_code=$?

  # Check manifest JSON contains new fields
  if [[ -f "$tmp_out/codex-run.json" ]]; then
    local manifest
    manifest=$(cat "$tmp_out/codex-run.json")

    if echo "$manifest" | grep -q '"session_id":"thread_manifest_test"'; then
      pass "manifest contains session_id"
    else
      fail "manifest session_id" "field missing or wrong: $manifest"
    fi

    if echo "$manifest" | grep -q '"resumed":false'; then
      pass "manifest contains resumed field"
    else
      fail "manifest resumed" "field missing or wrong: $manifest"
    fi
  else
    fail "manifest new fields" "codex-run.json not created"
  fi

  rm -rf "$tmp_bin" "$tmp_out" "$tmp_ws" "$tmp_home" "$tmp_script" "$tmp_prompt"
}

# ─────────────────────────────────────────────────────────────────────────────
# Test: Script handles codex without JSON support gracefully
# ─────────────────────────────────────────────────────────────────────────────
test_no_json_support_fallback() {
  run_test

  # Create temp directories for test
  local tmp_bin tmp_out tmp_ws
  tmp_bin=$(mktemp -d)
  tmp_out=$(mktemp -d)
  tmp_ws=$(mktemp -d)

  # Mock codex CLI without --json support
  cat > "$tmp_bin/codex" <<'MOCKCODEX'
#!/bin/bash
if [[ "$1" == "exec" && "$2" == "--help" ]]; then
  echo "Usage: codex exec [OPTIONS]"
  echo "  --yolo                Skip confirmations"
  # No --json, --output-last-message, or --output-dir advertised
  exit 0
fi
echo "Plain text output from codex"
exit 0
MOCKCODEX
  chmod +x "$tmp_bin/codex"

  # Use temp dir as HOME to avoid /root permission issues
  local tmp_home tmp_script
  tmp_home=$(mktemp -d)
  tmp_script=$(create_test_script)

  local tmp_prompt exit_code
  tmp_prompt=$(create_prompt_file)
  (
    export HOME="$tmp_home"
    export PATH="$tmp_bin:$PATH"
    bash "$tmp_script" --input "$tmp_ws" --out "$tmp_out" --prompt-file "$tmp_prompt"
  ) >/dev/null 2>&1
  exit_code=$?

  # Script should still succeed and create basic files
  if [[ $exit_code -eq 0 ]] && [[ -f "$tmp_out/codex.log" ]] && [[ -f "$tmp_out/codex-run.json" ]]; then
    pass "script succeeds without JSON support"
  else
    fail "no JSON support fallback" "script failed or basic files missing"
  fi

  # Manifest should have empty session_id and resumed:false
  if [[ -f "$tmp_out/codex-run.json" ]]; then
    local manifest
    manifest=$(cat "$tmp_out/codex-run.json")
    if echo "$manifest" | grep -q '"session_id":""'; then
      pass "manifest has empty session_id when no JSON support"
    else
      fail "manifest fallback" "expected empty session_id, got: $manifest"
    fi
    if echo "$manifest" | grep -q '"resumed":false'; then
      pass "manifest has resumed:false when not resuming"
    else
      fail "manifest resumed fallback" "expected resumed:false, got: $manifest"
    fi
  fi

  rm -rf "$tmp_bin" "$tmp_out" "$tmp_ws" "$tmp_home" "$tmp_script" "$tmp_prompt"
}

# ─────────────────────────────────────────────────────────────────────────────
# Test: CODEX_RESUME=1 with existing session invokes resume mode
# ─────────────────────────────────────────────────────────────────────────────
test_resume_mode_with_existing_session() {
  run_test

  # Create temp directories for test
  local tmp_bin tmp_out tmp_ws tmp_in
  tmp_bin=$(mktemp -d)
  tmp_out=$(mktemp -d)
  tmp_ws=$(mktemp -d)
  tmp_in=$(mktemp -d)

  # Write a prior session ID and build gate log to /in
  echo "thread_prior_session_abc123" > "$tmp_in/codex-session.txt"
  echo "build failed details" > "$tmp_in/build-gate.log"

  # Track if resume was invoked by recording all arguments
  local args_file="$tmp_out/.codex_args"

  # Mock codex CLI that records invocation arguments
  cat > "$tmp_bin/codex" <<MOCKCODEX
#!/bin/bash
if [[ "\$1" == "exec" && "\$2" == "--help" ]]; then
  echo "Usage: codex exec [OPTIONS]"
  echo "  --yolo                Skip confirmations"
  echo "  --json                Output JSON events"
  exit 0
fi
# Record all arguments to verify resume was passed
printf "%s\n" "\$@" > "$args_file"
# Output JSON event with new session (resumed sessions still emit thread.started)
echo '{"type":"thread.started","thread_id":"thread_resumed_xyz789"}'
exit 0
MOCKCODEX
  chmod +x "$tmp_bin/codex"

  # Use temp dir as HOME to avoid /root permission issues
  local tmp_home tmp_script
  tmp_home=$(mktemp -d)
  tmp_script=$(create_test_script)

  # Patch the test script to use /in as $tmp_in
  sed -i.bak "s|/in|$tmp_in|g" "$tmp_script"

  local tmp_prompt exit_code
  tmp_prompt=$(create_prompt_file "ORIGINAL_PROMPT")
  (
    export HOME="$tmp_home"
    export PATH="$tmp_bin:$PATH"
    export CODEX_RESUME=1
    bash "$tmp_script" --input "$tmp_ws" --out "$tmp_out" --prompt-file "$tmp_prompt"
  ) >/dev/null 2>&1
  exit_code=$?

  # Check that "resume" and the session ID were passed to codex
  if [[ -f "$args_file" ]]; then
    local args_content
    args_content=$(cat "$args_file")
    if echo "$args_content" | grep -q "resume" && echo "$args_content" | grep -q "thread_prior_session_abc123"; then
      pass "resume mode invoked with session ID"
    else
      fail "resume mode invocation" "expected 'resume thread_prior_session_abc123' in args: $args_content"
    fi
  else
    fail "resume mode invocation" "codex was not called (args file missing)"
  fi

  # Check that codex.log contains resume mode message
  if [[ -f "$tmp_out/codex.log" ]] && grep -q "resume mode enabled" "$tmp_out/codex.log"; then
    pass "codex.log indicates resume mode enabled"
  else
    fail "resume mode logging" "expected 'resume mode enabled' in codex.log"
  fi

  rm -rf "$tmp_bin" "$tmp_out" "$tmp_ws" "$tmp_in" "$tmp_home" "$tmp_script" "$tmp_prompt"
}

# ─────────────────────────────────────────────────────────────────────────────
# Test: CODEX_RESUME=1 without session file runs fresh exec
# ─────────────────────────────────────────────────────────────────────────────
test_resume_mode_without_session_file() {
  run_test

  # Create temp directories for test
  local tmp_bin tmp_out tmp_ws tmp_in
  tmp_bin=$(mktemp -d)
  tmp_out=$(mktemp -d)
  tmp_ws=$(mktemp -d)
  tmp_in=$(mktemp -d)

  # Do NOT create codex-session.txt; resume should be skipped

  # Track invocation arguments
  local args_file="$tmp_out/.codex_args"

  # Mock codex CLI
  cat > "$tmp_bin/codex" <<MOCKCODEX
#!/bin/bash
if [[ "\$1" == "exec" && "\$2" == "--help" ]]; then
  echo "Usage: codex exec [OPTIONS]"
  echo "  --yolo                Skip confirmations"
  exit 0
fi
printf "%s\n" "\$@" > "$args_file"
exit 0
MOCKCODEX
  chmod +x "$tmp_bin/codex"

  local tmp_home tmp_script
  tmp_home=$(mktemp -d)
  tmp_script=$(create_test_script)
  sed -i.bak "s|/in|$tmp_in|g" "$tmp_script"

  local tmp_prompt exit_code
  tmp_prompt=$(create_prompt_file)
  (
    export HOME="$tmp_home"
    export PATH="$tmp_bin:$PATH"
    export CODEX_RESUME=1
    bash "$tmp_script" --input "$tmp_ws" --out "$tmp_out" --prompt-file "$tmp_prompt"
  ) >/dev/null 2>&1
  exit_code=$?

  # Check that "resume" was NOT passed
  if [[ -f "$args_file" ]]; then
    local args_content
    args_content=$(cat "$args_file")
    if echo "$args_content" | grep -q "resume"; then
      fail "fresh exec mode" "resume was passed despite no session file: $args_content"
    else
      pass "fresh exec mode when no session file exists"
    fi
  else
    fail "fresh exec mode" "codex was not called"
  fi

  rm -rf "$tmp_bin" "$tmp_out" "$tmp_ws" "$tmp_in" "$tmp_home" "$tmp_script" "$tmp_prompt"
}

# ─────────────────────────────────────────────────────────────────────────────
# Test: Manifest includes resumed:true when resuming
# ─────────────────────────────────────────────────────────────────────────────
test_manifest_resumed_field_true() {
  run_test

  local tmp_bin tmp_out tmp_ws tmp_in
  tmp_bin=$(mktemp -d)
  tmp_out=$(mktemp -d)
  tmp_ws=$(mktemp -d)
  tmp_in=$(mktemp -d)

  echo "thread_session_for_resume" > "$tmp_in/codex-session.txt"

  # Mock codex CLI
  cat > "$tmp_bin/codex" <<'MOCKCODEX'
#!/bin/bash
if [[ "$1" == "exec" && "$2" == "--help" ]]; then
  echo "Usage: codex exec [OPTIONS]"
  echo "  --yolo                Skip confirmations"
  echo "  --json                Output JSON events"
  exit 0
fi
echo '{"type":"thread.started","thread_id":"thread_resumed_manifest"}'
exit 0
MOCKCODEX
  chmod +x "$tmp_bin/codex"

  local tmp_home tmp_script
  tmp_home=$(mktemp -d)
  tmp_script=$(create_test_script)
  sed -i.bak "s|/in|$tmp_in|g" "$tmp_script"

  local tmp_prompt
  tmp_prompt=$(create_prompt_file)
  (
    export HOME="$tmp_home"
    export PATH="$tmp_bin:$PATH"
    export CODEX_RESUME=1
    bash "$tmp_script" --input "$tmp_ws" --out "$tmp_out" --prompt-file "$tmp_prompt"
  ) >/dev/null 2>&1

  # Check manifest contains resumed:true
  if [[ -f "$tmp_out/codex-run.json" ]]; then
    local manifest
    manifest=$(cat "$tmp_out/codex-run.json")
    if echo "$manifest" | grep -q '"resumed":true'; then
      pass "manifest contains resumed:true when resuming"
    else
      fail "manifest resumed field" "expected resumed:true, got: $manifest"
    fi
  else
    fail "manifest resumed field" "codex-run.json not created"
  fi

  rm -rf "$tmp_bin" "$tmp_out" "$tmp_ws" "$tmp_in" "$tmp_home" "$tmp_script" "$tmp_prompt"
}

# ─────────────────────────────────────────────────────────────────────────────
# Test: Manifest includes resumed:false when not resuming
# ─────────────────────────────────────────────────────────────────────────────
test_manifest_resumed_field_false() {
  run_test

  local tmp_bin tmp_out tmp_ws
  tmp_bin=$(mktemp -d)
  tmp_out=$(mktemp -d)
  tmp_ws=$(mktemp -d)

  # Mock codex CLI
  cat > "$tmp_bin/codex" <<'MOCKCODEX'
#!/bin/bash
if [[ "$1" == "exec" && "$2" == "--help" ]]; then
  echo "Usage: codex exec [OPTIONS]"
  echo "  --yolo                Skip confirmations"
  exit 0
fi
exit 0
MOCKCODEX
  chmod +x "$tmp_bin/codex"

  local tmp_home tmp_script
  tmp_home=$(mktemp -d)
  tmp_script=$(create_test_script)

  local tmp_prompt
  tmp_prompt=$(create_prompt_file)
  (
    export HOME="$tmp_home"
    export PATH="$tmp_bin:$PATH"
    # CODEX_RESUME not set
    bash "$tmp_script" --input "$tmp_ws" --out "$tmp_out" --prompt-file "$tmp_prompt"
  ) >/dev/null 2>&1

  # Check manifest contains resumed:false
  if [[ -f "$tmp_out/codex-run.json" ]]; then
    local manifest
    manifest=$(cat "$tmp_out/codex-run.json")
    if echo "$manifest" | grep -q '"resumed":false'; then
      pass "manifest contains resumed:false when not resuming"
    else
      fail "manifest resumed field" "expected resumed:false, got: $manifest"
    fi
  else
    fail "manifest resumed field" "codex-run.json not created"
  fi

  rm -rf "$tmp_bin" "$tmp_out" "$tmp_ws" "$tmp_home" "$tmp_script" "$tmp_prompt"
}

# ─────────────────────────────────────────────────────────────────────────────
# Test: Script detects --output-dir flag
# ─────────────────────────────────────────────────────────────────────────────
test_output_dir_flag_detection() {
  run_test

  # Create temp directories for test
  local tmp_bin tmp_out tmp_ws
  tmp_bin=$(mktemp -d)
  tmp_out=$(mktemp -d)
  tmp_ws=$(mktemp -d)

  # Track if --output-dir was passed
  local flag_file="$tmp_out/.output_dir_flag"

  # Mock codex CLI that advertises --output-dir
  cat > "$tmp_bin/codex" <<MOCKCODEX
#!/bin/bash
if [[ "\$1" == "exec" && "\$2" == "--help" ]]; then
  echo "Usage: codex exec [OPTIONS]"
  echo "  --yolo                Skip confirmations"
  echo "  --output-dir <dir>    Write transcript to directory"
  exit 0
fi
# Check if --output-dir was passed
for arg in "\$@"; do
  if [[ "\$arg" == "--output-dir" ]]; then
    echo "detected" > "$flag_file"
    break
  fi
done
exit 0
MOCKCODEX
  chmod +x "$tmp_bin/codex"

  # Use temp dir as HOME to avoid /root permission issues
  local tmp_home tmp_script
  tmp_home=$(mktemp -d)
  tmp_script=$(create_test_script)

  local tmp_prompt exit_code
  tmp_prompt=$(create_prompt_file)
  (
    export HOME="$tmp_home"
    export PATH="$tmp_bin:$PATH"
    bash "$tmp_script" --input "$tmp_ws" --out "$tmp_out" --prompt-file "$tmp_prompt"
  ) >/dev/null 2>&1
  exit_code=$?

  if [[ -f "$flag_file" ]]; then
    pass "--output-dir flag detected and passed to codex"
  else
    fail "--output-dir flag detection" "flag was not passed to codex"
  fi

  rm -rf "$tmp_bin" "$tmp_out" "$tmp_ws" "$tmp_home" "$tmp_script" "$tmp_prompt"
}


# ─────────────────────────────────────────────────────────────────────────────
# Helper: portable octal permission check
# ─────────────────────────────────────────────────────────────────────────────
file_perms_octal() {
  stat -c "%a" "$1" 2>/dev/null || stat -f "%Lp" "$1" 2>/dev/null
}

# Helper: assert file content and secure mode
assert_file_content_and_mode_600() {
  local file_path="$1"
  local expected_content="$2"
  local label="$3"

  if [[ ! -f "$file_path" ]]; then
    fail "$label exists" "missing file: $file_path"
    return
  fi

  local content
  content=$(cat "$file_path")
  if [[ "$content" == "$expected_content" ]]; then
    pass "$label content"
  else
    fail "$label content" "got: $content"
    return
  fi

  local perms
  perms=$(file_perms_octal "$file_path")
  if [[ "$perms" == "600" ]]; then
    pass "$label permissions"
  else
    fail "$label permissions" "got: $perms, want: 600"
  fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Test: --auth and --config flags deliver credentials to CODEX_HOME with
#       correct content and secure (600) permissions
# ─────────────────────────────────────────────────────────────────────────────
test_credentials_via_flags() {
  run_test

  local tmp_bin tmp_out tmp_ws
  tmp_bin=$(mktemp -d)
  tmp_out=$(mktemp -d)
  tmp_ws=$(mktemp -d)

  cat > "$tmp_bin/codex" <<'MOCKCODEX'
#!/bin/bash
if [[ "$1" == "exec" && "$2" == "--help" ]]; then
  echo "Usage: codex exec [OPTIONS]"
  echo "  --yolo  Skip confirmations"
  exit 0
fi
exit 0
MOCKCODEX
  chmod +x "$tmp_bin/codex"

  local tmp_home tmp_script tmp_auth tmp_config tmp_prompt
  tmp_home=$(mktemp -d)
  tmp_script=$(create_test_script)
  tmp_auth=$(mktemp)
  tmp_config=$(mktemp)
  tmp_prompt=$(mktemp)
  echo -n '{"token":"auth_secret"}' > "$tmp_auth"
  printf '[model]\nname = "o4-mini"' > "$tmp_config"
  echo "test prompt" > "$tmp_prompt"

  (
    export HOME="$tmp_home"
    export PATH="$tmp_bin:$PATH"
    export OUTDIR="$tmp_out"
    bash "$tmp_script" --auth "$tmp_auth" --config "$tmp_config" --prompt-file "$tmp_prompt" --input "$tmp_ws" --out "$tmp_out"
  ) >/dev/null 2>&1

  # Verify auth.json was installed with correct content
  if [[ -f "$tmp_home/.codex/auth.json" ]]; then
    local auth_content
    auth_content=$(cat "$tmp_home/.codex/auth.json")
    if [[ "$auth_content" == '{"token":"auth_secret"}' ]]; then
      pass "--auth flag installs auth.json with correct content"
    else
      fail "--auth flag auth.json content" "got: $auth_content"
    fi
    local perms
    perms=$(file_perms_octal "$tmp_home/.codex/auth.json")
    if [[ "$perms" == "600" ]]; then
      pass "auth.json has secure permissions (600)"
    else
      fail "auth.json permissions" "got: $perms, want: 600"
    fi
  else
    fail "--auth flag auth.json delivery" "auth.json not created"
  fi

  # Verify config.toml was installed with correct content
  if [[ -f "$tmp_home/.codex/config.toml" ]]; then
    local cfg_content
    cfg_content=$(cat "$tmp_home/.codex/config.toml")
    if echo "$cfg_content" | grep -q 'o4-mini'; then
      pass "--config flag installs config.toml with correct content"
    else
      fail "--config flag config.toml content" "got: $cfg_content"
    fi
    local perms
    perms=$(file_perms_octal "$tmp_home/.codex/config.toml")
    if [[ "$perms" == "600" ]]; then
      pass "config.toml has secure permissions (600)"
    else
      fail "config.toml permissions" "got: $perms, want: 600"
    fi
  else
    fail "--config flag config.toml delivery" "config.toml not created"
  fi

  rm -rf "$tmp_bin" "$tmp_out" "$tmp_ws" "$tmp_home" "$tmp_script" "$tmp_auth" "$tmp_config" "$tmp_prompt"
}

# ─────────────────────────────────────────────────────────────────────────────
# Test: CODEX_HOME override controls --auth/--config destinations
# ─────────────────────────────────────────────────────────────────────────────
test_codex_home_override_flag_delivery() {
  run_test

  local tmp_bin tmp_out tmp_ws
  tmp_bin=$(mktemp -d)
  tmp_out=$(mktemp -d)
  tmp_ws=$(mktemp -d)

  cat > "$tmp_bin/codex" <<'MOCKCODEX'
#!/bin/bash
if [[ "$1" == "exec" && "$2" == "--help" ]]; then
  echo "Usage: codex exec [OPTIONS]"
  echo "  --yolo  Skip confirmations"
  exit 0
fi
exit 0
MOCKCODEX
  chmod +x "$tmp_bin/codex"

  local tmp_home tmp_script codex_home tmp_auth tmp_config tmp_prompt
  tmp_home=$(mktemp -d)
  tmp_script=$(create_test_script)
  codex_home="$tmp_out/codex-home"
  tmp_auth=$(mktemp)
  tmp_config=$(mktemp)
  tmp_prompt=$(mktemp)
  echo -n '{"token":"auth_override"}' > "$tmp_auth"
  printf '[model]\nname = "o4-mini-override"' > "$tmp_config"
  echo "test prompt" > "$tmp_prompt"

  (
    export HOME="$tmp_home"
    export PATH="$tmp_bin:$PATH"
    export OUTDIR="$tmp_out"
    export CODEX_HOME="$codex_home"
    bash "$tmp_script" --auth "$tmp_auth" --config "$tmp_config" --prompt-file "$tmp_prompt" --input "$tmp_ws" --out "$tmp_out"
  ) >/dev/null 2>&1

  assert_file_content_and_mode_600 \
    "$codex_home/auth.json" \
    '{"token":"auth_override"}' \
    "CODEX_HOME override auth.json"

  if [[ -f "$codex_home/config.toml" ]] && grep -q 'o4-mini-override' "$codex_home/config.toml"; then
    pass "CODEX_HOME override config.toml content"
  else
    fail "CODEX_HOME override config.toml content" "config.toml missing or unexpected"
  fi

  local perms
  perms=$(file_perms_octal "$codex_home/config.toml")
  if [[ "$perms" == "600" ]]; then
    pass "CODEX_HOME override config.toml permissions"
  else
    fail "CODEX_HOME override config.toml permissions" "got: $perms, want: 600"
  fi

  if [[ ! -f "$tmp_home/.codex/auth.json" && ! -f "$tmp_home/.codex/config.toml" ]]; then
    pass "default ~/.codex not used when CODEX_HOME is set"
  else
    fail "default ~/.codex not used when CODEX_HOME is set" "found files under $tmp_home/.codex"
  fi

  rm -rf "$tmp_bin" "$tmp_out" "$tmp_ws" "$tmp_home" "$tmp_script" "$tmp_auth" "$tmp_config" "$tmp_prompt"
}

# ─────────────────────────────────────────────────────────────────────────────
# Test: CODEX_API_KEY is passed through to codex exec in direct mode
# ─────────────────────────────────────────────────────────────────────────────
test_codex_api_key_passthrough_direct_mode() {
  run_test

  local tmp_bin tmp_out tmp_ws
  tmp_bin=$(mktemp -d)
  tmp_out=$(mktemp -d)
  tmp_ws=$(mktemp -d)
  local env_file="$tmp_out/.codex_env"

  cat > "$tmp_bin/codex" <<MOCKCODEX
#!/bin/bash
if [[ "\$1" == "exec" && "\$2" == "--help" ]]; then
  echo "Usage: codex exec [OPTIONS]"
  echo "  --yolo  Skip confirmations"
  exit 0
fi
# Dump CODEX_API_KEY from the environment to verify passthrough
echo "CODEX_API_KEY=\${CODEX_API_KEY:-}" > "$env_file"
exit 0
MOCKCODEX
  chmod +x "$tmp_bin/codex"

  local tmp_home tmp_script
  tmp_home=$(mktemp -d)
  tmp_script=$(create_test_script)

  local tmp_prompt
  tmp_prompt=$(create_prompt_file)
  (
    export HOME="$tmp_home"
    export PATH="$tmp_bin:$PATH"
    export CODEX_API_KEY="test_direct_api_key_xyz"
    bash "$tmp_script" --input "$tmp_ws" --out "$tmp_out" --prompt-file "$tmp_prompt"
  ) >/dev/null 2>&1

  if [[ -f "$env_file" ]]; then
    local env_content
    env_content=$(cat "$env_file")
    if echo "$env_content" | grep -q "CODEX_API_KEY=test_direct_api_key_xyz"; then
      pass "CODEX_API_KEY passed through to codex in direct mode"
    else
      fail "CODEX_API_KEY direct mode passthrough" "got: $env_content"
    fi
  else
    fail "CODEX_API_KEY direct mode passthrough" "codex env dump not created"
  fi

  rm -rf "$tmp_bin" "$tmp_out" "$tmp_ws" "$tmp_home" "$tmp_script" "$tmp_prompt"
}


# ─────────────────────────────────────────────────────────────────────────────
# Test: Direct codex mode requires prompt file (--prompt-file or /in/codex-prompt.txt)
# ─────────────────────────────────────────────────────────────────────────────
test_direct_mode_requires_prompt_file() {
  run_test

  local tmp_bin tmp_out tmp_ws
  tmp_bin=$(mktemp -d)
  tmp_out=$(mktemp -d)
  tmp_ws=$(mktemp -d)

  # Mock codex that would succeed if called, but we expect the script to exit before that
  cat > "$tmp_bin/codex" <<'MOCKCODEX'
#!/bin/bash
if [[ "$1" == "exec" && "$2" == "--help" ]]; then
  echo "Usage: codex exec [OPTIONS]"
  echo "  --yolo  Skip confirmations"
  exit 0
fi
echo "codex ran"
exit 0
MOCKCODEX
  chmod +x "$tmp_bin/codex"

  local tmp_home tmp_script
  tmp_home=$(mktemp -d)
  tmp_script=$(create_test_script)

  local exit_code
  (
    export HOME="$tmp_home"
    export PATH="$tmp_bin:$PATH"
    bash "$tmp_script" --input "$tmp_ws" --out "$tmp_out"
  ) >/dev/null 2>&1
  exit_code=$?

  if [[ $exit_code -ne 0 ]]; then
    pass "direct codex mode exits non-zero without prompt file"
  else
    fail "direct mode prompt required" "expected non-zero exit without --prompt-file or /in/codex-prompt.txt"
  fi

  rm -rf "$tmp_bin" "$tmp_out" "$tmp_ws" "$tmp_home" "$tmp_script"
}

# ─────────────────────────────────────────────────────────────────────────────
# Run all tests
# ─────────────────────────────────────────────────────────────────────────────
echo "Running codex entrypoint unit tests..."
echo ""

echo "Test: --help flag"
test_help_flag

echo ""
echo "Test: --json flag detection"
test_json_flag_detection

echo ""
echo "Test: Session ID extraction"
test_session_id_extraction

echo ""
echo "Test: Manifest contains new fields"
test_manifest_contains_new_fields

echo ""
echo "Test: No JSON support fallback"
test_no_json_support_fallback

echo ""
echo "Test: CODEX_RESUME=1 with existing session"
test_resume_mode_with_existing_session

echo ""
echo "Test: CODEX_RESUME=1 without session file"
test_resume_mode_without_session_file

echo ""
echo "Test: Manifest resumed:true when resuming"
test_manifest_resumed_field_true

echo ""
echo "Test: Manifest resumed:false when not resuming"
test_manifest_resumed_field_false

echo ""
echo "Test: --output-dir flag detection"
test_output_dir_flag_detection

echo ""
echo "Test: --auth/--config flags deliver credentials with secure permissions"
test_credentials_via_flags

echo ""
echo "Test: CODEX_HOME override controls --auth/--config delivery destination"
test_codex_home_override_flag_delivery

echo ""
echo "Test: CODEX_API_KEY passthrough in direct codex mode"
test_codex_api_key_passthrough_direct_mode

echo ""
echo "Test: Direct codex mode requires prompt file"
test_direct_mode_requires_prompt_file

# ─────────────────────────────────────────────────────────────────────────────
# Summary
# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "═══════════════════════════════════════════════════════════════════════"
echo "Tests: $TESTS_RUN | Passed: $TESTS_PASSED | Failed: $TESTS_FAILED"
echo "═══════════════════════════════════════════════════════════════════════"

if [[ $TESTS_FAILED -gt 0 ]]; then
  exit 1
fi

echo "OK"
exit 0
