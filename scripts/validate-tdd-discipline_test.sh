#!/usr/bin/env bash
# validate-tdd-discipline_test.sh
#
# Unit tests for validate-tdd-discipline.sh
#
# Usage:
#   bash scripts/validate-tdd-discipline_test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="${SCRIPT_DIR}/validate-tdd-discipline.sh"

# Test helpers
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

assert_success() {
    local cmd="$1"
    local description="$2"

    ((TESTS_RUN++)) || true

    if eval "$cmd" > /dev/null 2>&1; then
        echo "✓ PASS: $description"
        ((TESTS_PASSED++)) || true
    else
        echo "✗ FAIL: $description"
        echo "  Command: $cmd"
        ((TESTS_FAILED++)) || true
    fi
}

assert_failure() {
    local cmd="$1"
    local description="$2"

    ((TESTS_RUN++)) || true

    if eval "$cmd" > /dev/null 2>&1; then
        echo "✗ FAIL: $description (expected failure, got success)"
        echo "  Command: $cmd"
        ((TESTS_FAILED++)) || true
    else
        echo "✓ PASS: $description"
        ((TESTS_PASSED++)) || true
    fi
}

echo "╔════════════════════════════════════════════════════════════╗"
echo "║         TDD Discipline Validation Script Tests            ║"
echo "╚════════════════════════════════════════════════════════════╝"
echo ""

# Test 1: Script exists and is executable
assert_success "[ -x '$SCRIPT' ]" \
    "Script exists and is executable"

# Test 2: Script has correct shebang
assert_success "head -1 '$SCRIPT' | grep -q '^#!/usr/bin/env bash'" \
    "Script has correct shebang"

# Test 3: Script requires git repository
(
    cd /tmp
    assert_failure "bash '$SCRIPT' 2>&1" \
        "Script detects when not in git repository"
)

# Test 4: Script has required functions
assert_success "grep -q 'check_tests_pass' '$SCRIPT'" \
    "Script contains check_tests_pass function (GREEN phase)"

assert_success "grep -q 'check_coverage_thresholds' '$SCRIPT'" \
    "Script contains check_coverage_thresholds function (GREEN validation)"

assert_success "grep -q 'check_binary_size' '$SCRIPT'" \
    "Script contains check_binary_size function (REFACTOR validation)"

# Test 5: Script references TDD phases
assert_success "grep -qi 'RED.*GREEN.*REFACTOR' '$SCRIPT'" \
    "Script references RED→GREEN→REFACTOR methodology"

# Test 6: Script checks coverage thresholds
assert_success "grep -q '60%' '$SCRIPT' && grep -q '90%' '$SCRIPT'" \
    "Script enforces 60% overall and 90% critical coverage thresholds"

# Test 7: Script validates binary size
assert_success "grep -q 'BINARY_SIZE_THRESHOLD' '$SCRIPT'" \
    "Script checks binary size threshold"

# Test 8: Script integrates with existing validation scripts
assert_success "grep -q 'check-critical-coverage.sh' '$SCRIPT'" \
    "Script uses check-critical-coverage.sh for critical path validation"

assert_success "grep -q 'check-binary-size.sh' '$SCRIPT'" \
    "Script uses check-binary-size.sh for binary size validation"

# Summary
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Tests run:    $TESTS_RUN"
echo "Tests passed: $TESTS_PASSED"
echo "Tests failed: $TESTS_FAILED"
echo ""

if [ "$TESTS_FAILED" -eq 0 ]; then
    echo "✓ All tests passed"
    exit 0
else
    echo "✗ Some tests failed"
    exit 1
fi
