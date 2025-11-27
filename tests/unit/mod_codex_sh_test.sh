#!/usr/bin/env bash
# Unit tests for mod-codex.sh
# Tests CLI flag detection, JSONL event capture, session ID extraction,
# and build validation sentinel detection.
#
# Usage: bash tests/unit/mod_codex_sh_test.sh
#
# Exit codes:
#   0: All tests passed
#   1: One or more tests failed

set -uo pipefail

ROOT_DIR=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
SCRIPT="$ROOT_DIR/docker/mods/mod-codex/mod-codex.sh"

# Create a wrapper script that patches mod-codex.sh to use $HOME/.codex instead
# of hardcoded /root/.codex. This allows tests to run outside Docker containers.
create_test_script() {
  local tmp_script
  tmp_script=$(mktemp)
  # Replace /root/.codex with $HOME/.codex for testability
  sed 's|/root/.codex|$HOME/.codex|g' "$SCRIPT" > "$tmp_script"
  chmod +x "$tmp_script"
  echo "$tmp_script"
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
  if echo "$output" | grep -q "CODEX_PROMPT"; then
    pass "help mentions CODEX_PROMPT env"
  else
    fail "help mentions CODEX_PROMPT env" "expected CODEX_PROMPT in output"
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

  local output exit_code
  output=$( (
    export HOME="$tmp_home"
    export PATH="$tmp_bin:$PATH"
    export CODEX_PROMPT="test prompt"
    bash "$tmp_script" --input "$tmp_ws" --out "$tmp_out"
  ) 2>&1 )
  exit_code=$?

  # Check that JSONL file was created and contains the event
  if [[ -f "$tmp_out/codex-events.jsonl" ]] && grep -q "thread.started" "$tmp_out/codex-events.jsonl"; then
    pass "--json flag detected and JSONL captured"
  else
    fail "--json flag detection" "JSONL file missing or empty: $output"
  fi

  rm -rf "$tmp_bin" "$tmp_out" "$tmp_ws" "$tmp_home" "$tmp_script"
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

  local exit_code
  (
    export HOME="$tmp_home"
    export PATH="$tmp_bin:$PATH"
    export CODEX_PROMPT="test prompt"
    bash "$tmp_script" --input "$tmp_ws" --out "$tmp_out"
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

  rm -rf "$tmp_bin" "$tmp_out" "$tmp_ws" "$tmp_home" "$tmp_script"
}

# ─────────────────────────────────────────────────────────────────────────────
# Test: Script detects [[REQUEST_BUILD_VALIDATION]] sentinel
# ─────────────────────────────────────────────────────────────────────────────
test_build_validation_sentinel_detection() {
  run_test

  # Create temp directories for test
  local tmp_bin tmp_out tmp_ws
  tmp_bin=$(mktemp -d)
  tmp_out=$(mktemp -d)
  tmp_ws=$(mktemp -d)

  # Mock codex CLI that supports --output-last-message
  cat > "$tmp_bin/codex" <<'MOCKCODEX'
#!/bin/bash
if [[ "$1" == "exec" && "$2" == "--help" ]]; then
  echo "Usage: codex exec [OPTIONS]"
  echo "  --yolo                Skip confirmations"
  echo "  --output-last-message Write last message to file"
  exit 0
fi
# Find and write to --output-last-message file
args=("$@")
for ((i=0; i<${#args[@]}; i++)); do
  if [[ "${args[i]}" == "--output-last-message" ]]; then
    outfile="${args[i+1]}"
    echo "[[REQUEST_BUILD_VALIDATION]]" > "$outfile"
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

  local exit_code
  (
    export HOME="$tmp_home"
    export PATH="$tmp_bin:$PATH"
    export CODEX_PROMPT="test prompt"
    bash "$tmp_script" --input "$tmp_ws" --out "$tmp_out"
  ) >/dev/null 2>&1
  exit_code=$?

  # Check that request_build_validation flag file was created
  if [[ -f "$tmp_out/request_build_validation" ]]; then
    local flag_value
    flag_value=$(cat "$tmp_out/request_build_validation")
    if [[ "$flag_value" == "true" ]]; then
      pass "build validation sentinel detected"
    else
      fail "sentinel detection" "flag file has wrong value: $flag_value"
    fi
  else
    fail "sentinel detection" "request_build_validation file not created"
  fi

  rm -rf "$tmp_bin" "$tmp_out" "$tmp_ws" "$tmp_home" "$tmp_script"
}

# ─────────────────────────────────────────────────────────────────────────────
# Test: Manifest includes requested_build_validation and session_id fields
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
# Find and write sentinel to --output-last-message file
args=("$@")
for ((i=0; i<${#args[@]}; i++)); do
  if [[ "${args[i]}" == "--output-last-message" ]]; then
    outfile="${args[i+1]}"
    echo "[[REQUEST_BUILD_VALIDATION]]" > "$outfile"
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

  local exit_code
  (
    export HOME="$tmp_home"
    export PATH="$tmp_bin:$PATH"
    export CODEX_PROMPT="test prompt"
    bash "$tmp_script" --input "$tmp_ws" --out "$tmp_out"
  ) >/dev/null 2>&1
  exit_code=$?

  # Check manifest JSON contains new fields
  if [[ -f "$tmp_out/codex-run.json" ]]; then
    local manifest
    manifest=$(cat "$tmp_out/codex-run.json")

    if echo "$manifest" | grep -q '"requested_build_validation":true'; then
      pass "manifest contains requested_build_validation:true"
    else
      fail "manifest requested_build_validation" "field missing or wrong: $manifest"
    fi

    if echo "$manifest" | grep -q '"session_id":"thread_manifest_test"'; then
      pass "manifest contains session_id"
    else
      fail "manifest session_id" "field missing or wrong: $manifest"
    fi
  else
    fail "manifest new fields" "codex-run.json not created"
  fi

  rm -rf "$tmp_bin" "$tmp_out" "$tmp_ws" "$tmp_home" "$tmp_script"
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

  local exit_code
  (
    export HOME="$tmp_home"
    export PATH="$tmp_bin:$PATH"
    export CODEX_PROMPT="test prompt"
    bash "$tmp_script" --input "$tmp_ws" --out "$tmp_out"
  ) >/dev/null 2>&1
  exit_code=$?

  # Script should still succeed and create basic files
  if [[ $exit_code -eq 0 ]] && [[ -f "$tmp_out/codex.log" ]] && [[ -f "$tmp_out/codex-run.json" ]]; then
    pass "script succeeds without JSON support"
  else
    fail "no JSON support fallback" "script failed or basic files missing"
  fi

  # Manifest should have empty session_id and false requested_build_validation
  if [[ -f "$tmp_out/codex-run.json" ]]; then
    local manifest
    manifest=$(cat "$tmp_out/codex-run.json")
    if echo "$manifest" | grep -q '"requested_build_validation":false'; then
      pass "manifest has false requested_build_validation when no sentinel"
    else
      fail "manifest fallback" "expected false, got: $manifest"
    fi
  fi

  rm -rf "$tmp_bin" "$tmp_out" "$tmp_ws" "$tmp_home" "$tmp_script"
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

  local exit_code
  (
    export HOME="$tmp_home"
    export PATH="$tmp_bin:$PATH"
    export CODEX_PROMPT="test prompt"
    bash "$tmp_script" --input "$tmp_ws" --out "$tmp_out"
  ) >/dev/null 2>&1
  exit_code=$?

  if [[ -f "$flag_file" ]]; then
    pass "--output-dir flag detected and passed to codex"
  else
    fail "--output-dir flag detection" "flag was not passed to codex"
  fi

  rm -rf "$tmp_bin" "$tmp_out" "$tmp_ws" "$tmp_home" "$tmp_script"
}

# ─────────────────────────────────────────────────────────────────────────────
# Run all tests
# ─────────────────────────────────────────────────────────────────────────────
echo "Running mod-codex.sh unit tests..."
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
echo "Test: Build validation sentinel detection"
test_build_validation_sentinel_detection

echo ""
echo "Test: Manifest contains new fields"
test_manifest_contains_new_fields

echo ""
echo "Test: No JSON support fallback"
test_no_json_support_fallback

echo ""
echo "Test: --output-dir flag detection"
test_output_dir_flag_detection

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
