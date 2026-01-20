#!/usr/bin/env bash
#
# check-untyped-contracts_test.sh — Unit tests for the untyped contracts guardrail script
#
# This test verifies that check-untyped-contracts.sh correctly:
#   1. Passes when no violations exist (current state)
#   2. Detects new map[string]any violations in boundary files
#   3. Excludes test files from violation checks
#   4. Excludes approved parsing modules from violation checks
#   5. Excludes comment lines from violation checks
#
# Run this test with: ./scripts/check-untyped-contracts_test.sh
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SCRIPT_UNDER_TEST="$SCRIPT_DIR/check-untyped-contracts.sh"

# Test utilities.
TEMP_DIR=""
TESTS_RUN=0
TESTS_PASSED=0

setup() {
    TEMP_DIR=$(mktemp -d)
}

teardown() {
    if [ -n "$TEMP_DIR" ] && [ -d "$TEMP_DIR" ]; then
        rm -rf "$TEMP_DIR"
    fi
}

pass() {
    echo "  ✓ $1"
    TESTS_PASSED=$((TESTS_PASSED + 1))
}

fail() {
    local msg="$1"
    local detail="${2:-}"
    echo "  ✗ $msg"
    if [ -n "$detail" ]; then
        echo "    $detail"
    fi
}

run_test() {
    local test_name="$1"
    TESTS_RUN=$((TESTS_RUN + 1))
    echo "Running: $test_name"
}

# Test 1: Current codebase passes the check.
test_current_codebase_passes() {
    run_test "Current codebase passes the check"

    cd "$REPO_ROOT"
    if "$SCRIPT_UNDER_TEST" --count >/dev/null 2>&1; then
        local count
        count=$("$SCRIPT_UNDER_TEST" --count)
        if [ "$count" = "0" ]; then
            pass "No violations in current codebase"
        else
            fail "Expected 0 violations, got $count"
        fi
    else
        fail "Script exited with non-zero code" ""
    fi
}

# Test 2: Script detects violations in new files.
test_detects_new_violations() {
    run_test "Detects violations in new boundary files"

    # Create a temporary handler file with a violation.
    local test_file="$REPO_ROOT/internal/server/handlers/test_violation_temp.go"

    cat > "$test_file" << 'EOF'
package handlers

// ViolatingHandlerAny uses untyped map at API boundary.
func ViolatingHandlerAny() map[string]any { return nil }

// ViolatingHandlerIface uses untyped map at API boundary.
func ViolatingHandlerIface() map[string]interface{} { return nil }
EOF

    cd "$REPO_ROOT"
    local count
    count=$("$SCRIPT_UNDER_TEST" --count 2>/dev/null || echo "0")

    # Cleanup.
    rm -f "$test_file"

    if [ "$count" = "2" ]; then
        pass "Detected $count violation(s) in test file"
    else
        fail "Expected 2 violations, got $count" ""
    fi
}

# Test 3: Test files are excluded.
test_excludes_test_files() {
    run_test "Excludes _test.go files from checks"

    # The script should not flag violations in test files.
    # We verify by checking the exclusion logic in the script itself.
    cd "$REPO_ROOT"

    # Create a test file with map[string]any - should be excluded.
    local test_file="$REPO_ROOT/internal/server/handlers/temp_exclusion_test.go"

    cat > "$test_file" << 'EOF'
package handlers

import "testing"

func TestSomething(t *testing.T) {
    data := map[string]any{"test": "value"}
    _ = data
}
EOF

    local count
    count=$("$SCRIPT_UNDER_TEST" --count 2>/dev/null || echo "error")

    # Cleanup.
    rm -f "$test_file"

    if [ "$count" = "0" ]; then
        pass "Test file was excluded from checks"
    else
        fail "Expected 0 violations (test file should be excluded), got $count" ""
    fi
}

# Test 4: Approved files are excluded.
test_excludes_approved_files() {
    run_test "Approved parsing modules are excluded"

    # Verify that approved files like claimer_spec.go are excluded.
    # We can check this by examining if the script would detect their existing usage.
    cd "$REPO_ROOT"

    # Count should be 0 even though approved files have map[string]any.
    local count
    count=$("$SCRIPT_UNDER_TEST" --count 2>/dev/null || echo "error")

    if [ "$count" = "0" ]; then
        pass "Approved files are excluded from checks"
    else
        fail "Expected 0 violations (approved files should be excluded), got $count" ""
    fi
}

# Test 5: Comment lines are excluded.
test_excludes_comment_lines() {
    run_test "Comment lines are excluded from violation checks"

    # Create a file with map[string]any only in comments.
    local test_file="$REPO_ROOT/internal/server/handlers/test_comment_temp.go"

    cat > "$test_file" << 'EOF'
package handlers

// This function uses typed structs instead of map[string]any.
// Previously used map[string]interface{} which was replaced.
func TypedHandler() string {
    return "typed"
}
EOF

    cd "$REPO_ROOT"
    local count
    count=$("$SCRIPT_UNDER_TEST" --count 2>/dev/null || echo "error")

    # Cleanup.
    rm -f "$test_file"

    if [ "$count" = "0" ]; then
        pass "Comment lines are excluded from checks"
    else
        fail "Expected 0 violations (comments should be excluded), got $count" ""
    fi
}

# Test 6: --list mode works correctly.
test_list_mode() {
    run_test "--list mode outputs violations without summary"

    cd "$REPO_ROOT"
    local output
    output=$("$SCRIPT_UNDER_TEST" --list 2>/dev/null || true)

    # In clean state, output should be empty.
    if [ -z "$output" ]; then
        pass "--list mode returns empty for clean codebase"
    else
        # If there's output, it should be file:line format (not summary text).
        if echo "$output" | grep -qE '^[a-zA-Z].*:[0-9]+:'; then
            pass "--list mode returns file:line format"
        else
            fail "Expected file:line format or empty output" "$output"
        fi
    fi
}

# Test 7: Exit codes are correct.
test_exit_codes() {
    run_test "Exit codes are correct"

    cd "$REPO_ROOT"

    # Clean codebase should exit 0.
    if "$SCRIPT_UNDER_TEST" >/dev/null 2>&1; then
        pass "Exit code 0 for clean codebase"
    else
        fail "Expected exit code 0 for clean codebase" ""
    fi
}

# Main test runner.
main() {
    echo "=== Running check-untyped-contracts.sh tests ==="
    echo ""

    setup

    test_current_codebase_passes
    test_detects_new_violations
    test_excludes_test_files
    test_excludes_approved_files
    test_excludes_comment_lines
    test_list_mode
    test_exit_codes

    teardown

    echo ""
    echo "=== Test Summary ==="
    echo "  Passed: $TESTS_PASSED / $TESTS_RUN"

    if [ "$TESTS_PASSED" -eq "$TESTS_RUN" ]; then
        echo "  All tests passed!"
        exit 0
    else
        echo "  Some tests failed."
        exit 1
    fi
}

main "$@"
