#!/usr/bin/env bash
# Tests for check-binary-size.sh
#
# Run with: bash scripts/check-binary-size_test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CHECK_SCRIPT="$SCRIPT_DIR/check-binary-size.sh"
TEST_DIR=""
TESTS_PASSED=0
TESTS_FAILED=0

# Cleanup function to remove test directory
cleanup() {
  if [[ -n "$TEST_DIR" ]] && [[ -d "$TEST_DIR" ]]; then
    rm -rf "$TEST_DIR"
  fi
}
trap cleanup EXIT

# Setup: Create temporary directory for test binaries
setup() {
  TEST_DIR=$(mktemp -d 2>/dev/null || mktemp -d -t binsizetest)
  echo "Test directory: $TEST_DIR"
}

# Helper: Assert command succeeds
assert_success() {
  local description="$1"
  shift
  # Temporarily disable exit-on-error for test execution
  set +e
  "$@" >/dev/null 2>&1
  local exit_code=$?
  set -e

  if [[ $exit_code -eq 0 ]]; then
    echo "✓ PASS: $description"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  else
    echo "✗ FAIL: $description (expected success, got exit $exit_code)"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi
}

# Helper: Assert command fails
assert_failure() {
  local description="$1"
  shift
  # Temporarily disable exit-on-error for test execution
  set +e
  "$@" >/dev/null 2>&1
  local exit_code=$?
  set -e

  if [[ $exit_code -ne 0 ]]; then
    echo "✓ PASS: $description"
    TESTS_PASSED=$((TESTS_PASSED + 1))
  else
    echo "✗ FAIL: $description (expected failure, got success)"
    TESTS_FAILED=$((TESTS_FAILED + 1))
  fi
}

# Helper: Create binary of specified size (MB)
create_binary() {
  local path="$1"
  local size_mb="$2"
  dd if=/dev/zero of="$path" bs=1048576 count="$size_mb" 2>/dev/null
  chmod +x "$path"
}

# Test: Binary within threshold passes
test_within_threshold() {
  local bin="$TEST_DIR/small"
  create_binary "$bin" 5
  assert_success "Binary within threshold (5 MB < 15 MB default)" \
    "$CHECK_SCRIPT" "$bin"
}

# Test: Binary at threshold passes
test_at_threshold() {
  local bin="$TEST_DIR/at-limit"
  create_binary "$bin" 10
  assert_success "Binary at threshold (10 MB <= 10 MB custom)" \
    "$CHECK_SCRIPT" "$bin" 10
}

# Test: Binary exceeding threshold fails
test_exceeds_threshold() {
  local bin="$TEST_DIR/large"
  create_binary "$bin" 20
  assert_failure "Binary exceeding threshold (20 MB > 15 MB default)" \
    "$CHECK_SCRIPT" "$bin"
}

# Test: Custom threshold works
test_custom_threshold() {
  local bin="$TEST_DIR/medium"
  create_binary "$bin" 18
  # Should fail with default threshold (15 MB)
  assert_failure "Binary exceeds default threshold (18 MB > 15 MB)" \
    "$CHECK_SCRIPT" "$bin"
  # Should pass with custom threshold (20 MB)
  assert_success "Binary within custom threshold (18 MB < 20 MB)" \
    "$CHECK_SCRIPT" "$bin" 20
}

# Test: Missing binary fails
test_missing_binary() {
  assert_failure "Missing binary path fails" \
    "$CHECK_SCRIPT" "$TEST_DIR/nonexistent"
}

# Test: Invalid threshold fails
test_invalid_threshold() {
  local bin="$TEST_DIR/test"
  create_binary "$bin" 1
  assert_failure "Negative threshold fails" \
    "$CHECK_SCRIPT" "$bin" -5
  assert_failure "Zero threshold fails" \
    "$CHECK_SCRIPT" "$bin" 0
  assert_failure "Non-numeric threshold fails" \
    "$CHECK_SCRIPT" "$bin" abc
}

# Test: No arguments shows usage
test_no_args() {
  assert_failure "No arguments shows usage" \
    "$CHECK_SCRIPT"
}

# Main test runner
main() {
  echo "=== Running check-binary-size.sh tests ==="
  echo ""

  setup

  test_within_threshold
  test_at_threshold
  test_exceeds_threshold
  test_custom_threshold
  test_missing_binary
  test_invalid_threshold
  test_no_args

  echo ""
  echo "=== Test Summary ==="
  echo "Passed: $TESTS_PASSED"
  echo "Failed: $TESTS_FAILED"
  echo ""

  if [[ $TESTS_FAILED -eq 0 ]]; then
    echo "All tests passed!"
    exit 0
  else
    echo "Some tests failed."
    exit 1
  fi
}

main "$@"
