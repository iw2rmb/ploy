#!/usr/bin/env bash
# Unit tests for buildgate-validate.sh
# Tests argument parsing, validation, and JSON payload construction.
#
# Usage: bash tests/unit/buildgate_validate_sh_test.sh
#
# Exit codes:
#   0: All tests passed
#   1: One or more tests failed

set -uo pipefail

ROOT_DIR=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
SCRIPT="$ROOT_DIR/docker/mods/mod-codex/buildgate-validate.sh"

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
  if echo "$output" | grep -q "repo-url"; then
    pass "help displays --repo-url option"
  else
    fail "help displays --repo-url option" "expected --repo-url in output"
    return
  fi
  if echo "$output" | grep -q "diff-patch"; then
    pass "help displays --diff-patch option"
  else
    fail "help displays --diff-patch option" "expected --diff-patch in output"
    return
  fi
  if echo "$output" | grep -q "PLOY_BUILDGATE_REF"; then
    pass "help mentions PLOY_BUILDGATE_REF env"
  else
    fail "help mentions PLOY_BUILDGATE_REF env" "expected PLOY_BUILDGATE_REF in output"
    return
  fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Test: Missing PLOY_SERVER_URL returns exit 2
# ─────────────────────────────────────────────────────────────────────────────
test_missing_server_url() {
  run_test
  local output exit_code
  # Run in subshell and capture exit code explicitly
  output=$( (unset PLOY_SERVER_URL; bash "$SCRIPT" --repo-url "https://example.com/repo.git" --ref "main") 2>&1 )
  exit_code=$?
  if [[ $exit_code -eq 2 ]] && echo "$output" | grep -q "PLOY_SERVER_URL is required"; then
    pass "missing PLOY_SERVER_URL returns exit 2 with error"
  else
    fail "missing PLOY_SERVER_URL" "exit=$exit_code, output=$output"
  fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Test: Missing --repo-url returns exit 2
# ─────────────────────────────────────────────────────────────────────────────
test_missing_repo_url() {
  run_test
  local output exit_code
  output=$( (export PLOY_SERVER_URL="https://server:8443"; unset PLOY_REPO_URL; bash "$SCRIPT" --ref "main") 2>&1 )
  exit_code=$?
  if [[ $exit_code -eq 2 ]] && echo "$output" | grep -q "repo-url.*is required"; then
    pass "missing --repo-url returns exit 2 with error"
  else
    fail "missing --repo-url" "exit=$exit_code, output=$output"
  fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Test: Missing --ref returns exit 2
# ─────────────────────────────────────────────────────────────────────────────
test_missing_ref() {
  run_test
  local output exit_code
  output=$( (
    export PLOY_SERVER_URL="https://server:8443"
    export PLOY_REPO_URL="https://example.com/repo.git"
    unset PLOY_BUILDGATE_REF
    bash "$SCRIPT"
  ) 2>&1 )
  exit_code=$?
  if [[ $exit_code -eq 2 ]] && echo "$output" | grep -q "ref.*is required"; then
    pass "missing --ref returns exit 2 with error"
  else
    fail "missing --ref" "exit=$exit_code, output=$output"
  fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Test: Environment variables are honored (payload construction)
# ─────────────────────────────────────────────────────────────────────────────
test_env_vars_honored() {
  run_test

  # Use a mock curl that writes the --data payload to a temp file
  local tmp_bin tmp_capture
  tmp_bin=$(mktemp -d)
  tmp_capture=$(mktemp)
  cat > "$tmp_bin/curl" <<MOCKCURL
#!/bin/bash
# Mock curl that captures the --data payload
capture_file="$tmp_capture"
capture_next=false
for arg in "\$@"; do
  if [[ "\$capture_next" == "true" ]]; then
    echo "\$arg" > "\$capture_file"
    capture_next=false
  fi
  if [[ "\$arg" == "--data" ]]; then
    capture_next=true
  fi
done
# Return a mock response
echo '{"job_id":"test-123","status":"completed","result":{"passed":true}}'
exit 0
MOCKCURL
  chmod +x "$tmp_bin/curl"

  local output exit_code
  output=$( (
    export PATH="$tmp_bin:$PATH"
    export PLOY_SERVER_URL="https://server:8443"
    export PLOY_REPO_URL="https://example.com/repo.git"
    export PLOY_BUILDGATE_REF="my-branch"
    export PLOY_BUILDGATE_PROFILE="java-maven"
    export PLOY_BUILDGATE_TIMEOUT="15m"
    bash "$SCRIPT"
  ) 2>&1 )
  exit_code=$?

  local captured_json=""
  if [[ -f "$tmp_capture" ]]; then
    captured_json=$(cat "$tmp_capture")
  fi
  rm -rf "$tmp_bin" "$tmp_capture"

  if [[ $exit_code -ne 0 ]]; then
    fail "env vars honored" "script failed with exit=$exit_code, output=$output"
    return
  fi

  # Check that env values were used in captured JSON payload
  if echo "$captured_json" | grep -q '"repo_url"'; then
    pass "repo_url field present in payload"
  else
    fail "repo_url field" "expected repo_url in captured JSON: $captured_json"
    return
  fi
  if echo "$captured_json" | grep -q '"ref"'; then
    pass "ref field present in payload"
  else
    fail "ref field" "expected ref in captured JSON"
    return
  fi
  if echo "$captured_json" | grep -q '"profile"'; then
    pass "profile field present in payload"
  else
    fail "profile field" "expected profile in captured JSON"
  fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Test: --diff-patch flag encodes file correctly
# ─────────────────────────────────────────────────────────────────────────────
test_diff_patch_encoding() {
  run_test

  # Create a test diff file
  local tmp_dir
  tmp_dir=$(mktemp -d)
  local diff_file="$tmp_dir/test.patch"
  cat > "$diff_file" <<'DIFF'
--- a/file.txt
+++ b/file.txt
@@ -1 +1 @@
-old content
+new content
DIFF

  # Use a mock curl that writes the --data payload to a temp file
  local tmp_bin tmp_capture
  tmp_bin=$(mktemp -d)
  tmp_capture=$(mktemp)
  cat > "$tmp_bin/curl" <<MOCKCURL
#!/bin/bash
capture_file="$tmp_capture"
capture_next=false
for arg in "\$@"; do
  if [[ "\$capture_next" == "true" ]]; then
    echo "\$arg" > "\$capture_file"
    capture_next=false
  fi
  if [[ "\$arg" == "--data" ]]; then
    capture_next=true
  fi
done
echo '{"job_id":"test-123","status":"completed","result":{"passed":true}}'
exit 0
MOCKCURL
  chmod +x "$tmp_bin/curl"

  local output exit_code
  output=$( (
    export PATH="$tmp_bin:$PATH"
    export PLOY_SERVER_URL="https://server:8443"
    export PLOY_REPO_URL="https://example.com/repo.git"
    export PLOY_BUILDGATE_REF="main"
    bash "$SCRIPT" --diff-patch "$diff_file"
  ) 2>&1 )
  exit_code=$?

  local captured_json=""
  if [[ -f "$tmp_capture" ]]; then
    captured_json=$(cat "$tmp_capture")
  fi
  rm -rf "$tmp_bin" "$tmp_dir" "$tmp_capture"

  if [[ $exit_code -ne 0 ]]; then
    fail "diff-patch encoding" "script failed with exit=$exit_code"
    return
  fi

  # Check that diff_patch field is present in captured JSON
  if echo "$captured_json" | grep -q '"diff_patch":'; then
    pass "diff_patch field present in payload"
  else
    fail "diff_patch field" "expected diff_patch in captured JSON: $captured_json"
    return
  fi

  # Verify the encoded content is non-empty base64 (allow optional space after colon from jq)
  if echo "$captured_json" | grep -qE '"diff_patch":\s*"[A-Za-z0-9+/=]'; then
    pass "diff_patch contains base64 encoded data"
  else
    fail "diff_patch base64" "expected base64 content"
  fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Test: Missing diff-patch file returns error
# ─────────────────────────────────────────────────────────────────────────────
test_missing_diff_patch_file() {
  run_test
  local output exit_code
  output=$( (
    export PLOY_SERVER_URL="https://server:8443"
    export PLOY_REPO_URL="https://example.com/repo.git"
    export PLOY_BUILDGATE_REF="main"
    bash "$SCRIPT" --diff-patch "/nonexistent/path/to/file.patch"
  ) 2>&1 )
  exit_code=$?

  if [[ $exit_code -ne 0 ]] && echo "$output" | grep -q "diff patch file not found"; then
    pass "missing diff-patch file reports error"
  else
    fail "missing diff-patch file" "exit=$exit_code, output=$output"
  fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Test: Unknown argument returns exit 2
# ─────────────────────────────────────────────────────────────────────────────
test_unknown_argument() {
  run_test
  local output exit_code
  output=$( (
    export PLOY_SERVER_URL="https://server:8443"
    bash "$SCRIPT" --unknown-flag
  ) 2>&1 )
  exit_code=$?
  if [[ $exit_code -eq 2 ]] && echo "$output" | grep -q "unknown argument"; then
    pass "unknown argument returns exit 2 with error"
  else
    fail "unknown argument" "exit=$exit_code, output=$output"
  fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Test: CLI flags override environment variables
# ─────────────────────────────────────────────────────────────────────────────
test_cli_flags_override_env() {
  run_test

  # Use a mock curl that writes the --data payload to a temp file
  local tmp_bin tmp_capture
  tmp_bin=$(mktemp -d)
  tmp_capture=$(mktemp)
  cat > "$tmp_bin/curl" <<MOCKCURL
#!/bin/bash
capture_file="$tmp_capture"
capture_next=false
for arg in "\$@"; do
  if [[ "\$capture_next" == "true" ]]; then
    echo "\$arg" > "\$capture_file"
    capture_next=false
  fi
  if [[ "\$arg" == "--data" ]]; then
    capture_next=true
  fi
done
echo '{"job_id":"test-123","status":"completed","result":{"passed":true}}'
exit 0
MOCKCURL
  chmod +x "$tmp_bin/curl"

  local output exit_code
  output=$( (
    export PATH="$tmp_bin:$PATH"
    export PLOY_SERVER_URL="https://server:8443"
    export PLOY_REPO_URL="https://env-repo.com/repo.git"
    export PLOY_BUILDGATE_REF="env-branch"
    export PLOY_BUILDGATE_PROFILE="env-profile"
    bash "$SCRIPT" \
      --repo-url "https://cli-repo.com/repo.git" \
      --ref "cli-branch" \
      --profile "cli-profile"
  ) 2>&1 )
  exit_code=$?

  local captured_json=""
  if [[ -f "$tmp_capture" ]]; then
    captured_json=$(cat "$tmp_capture")
  fi
  rm -rf "$tmp_bin" "$tmp_capture"

  if [[ $exit_code -ne 0 ]]; then
    fail "CLI flags override env" "script failed with exit=$exit_code"
    return
  fi

  # CLI values should be used instead of env
  if echo "$captured_json" | grep -q 'cli-repo.com'; then
    pass "CLI --repo-url overrides PLOY_REPO_URL"
  else
    fail "CLI --repo-url override" "expected cli-repo in captured JSON"
    return
  fi
  if echo "$captured_json" | grep -q 'cli-branch'; then
    pass "CLI --ref overrides PLOY_BUILDGATE_REF"
  else
    fail "CLI --ref override" "expected cli-branch in captured JSON"
    return
  fi
  if echo "$captured_json" | grep -q 'cli-profile'; then
    pass "CLI --profile overrides PLOY_BUILDGATE_PROFILE"
  else
    fail "CLI --profile override" "expected cli-profile in captured JSON"
  fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Test: Payload contains only expected fields (no legacy archive field)
# ─────────────────────────────────────────────────────────────────────────────
test_payload_has_only_expected_fields() {
  run_test

  # Use a mock curl that writes the --data payload to a temp file
  local tmp_bin tmp_capture
  tmp_bin=$(mktemp -d)
  tmp_capture=$(mktemp)
  cat > "$tmp_bin/curl" <<MOCKCURL
#!/bin/bash
capture_file="$tmp_capture"
capture_next=false
for arg in "\$@"; do
  if [[ "\$capture_next" == "true" ]]; then
    echo "\$arg" > "\$capture_file"
    capture_next=false
  fi
  if [[ "\$arg" == "--data" ]]; then
    capture_next=true
  fi
done
echo '{"job_id":"test-123","status":"completed","result":{"passed":true}}'
exit 0
MOCKCURL
  chmod +x "$tmp_bin/curl"

  local output exit_code
  output=$( (
    export PATH="$tmp_bin:$PATH"
    export PLOY_SERVER_URL="https://server:8443"
    export PLOY_REPO_URL="https://example.com/repo.git"
    export PLOY_BUILDGATE_REF="main"
    bash "$SCRIPT"
  ) 2>&1 )
  exit_code=$?

  local captured_json=""
  if [[ -f "$tmp_capture" ]]; then
    captured_json=$(cat "$tmp_capture")
  fi
  rm -rf "$tmp_bin" "$tmp_capture"

  if [[ $exit_code -ne 0 ]]; then
    fail "expected fields only" "script failed with exit=$exit_code"
    return
  fi

  # Verify a minimal set of expected fields is present and no obvious legacy
  # archive-style field names appear in the payload.
  if ! echo "$captured_json" | grep -q '"repo_url"'; then
    fail "expected fields only" "repo_url missing from payload: $captured_json"
    return
  fi
  if ! echo "$captured_json" | grep -q '"ref"'; then
    fail "expected fields only" "ref missing from payload: $captured_json"
    return
  fi
  if echo "$captured_json" | grep -qi 'archive'; then
    fail "expected fields only" "unexpected archive-like field in payload: $captured_json"
  else
    pass "payload contains only expected repo+ref(+diff_patch) fields"
  fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Run all tests
# ─────────────────────────────────────────────────────────────────────────────
echo "Running buildgate-validate.sh unit tests..."
echo ""

echo "Test: --help flag"
test_help_flag

echo ""
echo "Test: Missing PLOY_SERVER_URL"
test_missing_server_url

echo ""
echo "Test: Missing --repo-url"
test_missing_repo_url

echo ""
echo "Test: Missing --ref"
test_missing_ref

echo ""
echo "Test: Environment variables honored"
test_env_vars_honored

echo ""
echo "Test: --diff-patch encoding"
test_diff_patch_encoding

echo ""
echo "Test: Missing diff-patch file"
test_missing_diff_patch_file

echo ""
echo "Test: Unknown argument"
test_unknown_argument

echo ""
echo "Test: CLI flags override environment"
test_cli_flags_override_env

echo ""
echo "Test: Payload has only expected fields"
test_payload_has_only_expected_fields

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
