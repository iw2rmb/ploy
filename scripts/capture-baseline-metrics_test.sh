#!/usr/bin/env bash
# capture-baseline-metrics_test.sh - Test script for capture-baseline-metrics.sh
#
# This script validates that the baseline metrics capture script works correctly.
# It performs basic smoke tests without running the full test suite.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/capture-baseline-metrics.sh"

echo "=== Testing capture-baseline-metrics.sh ==="
echo ""

# Test 1: Script exists and is executable
echo "Test 1: Script exists and is executable"
if [ ! -f "$SCRIPT" ]; then
    echo "FAIL: Script not found at $SCRIPT"
    exit 1
fi

if [ ! -x "$SCRIPT" ]; then
    echo "FAIL: Script is not executable"
    exit 1
fi
echo "✓ PASS"
echo ""

# Test 2: Script shows usage when no argument provided
echo "Test 2: Script has proper shebang and basic structure"
if ! head -1 "$SCRIPT" | grep -q "^#!/usr/bin/env bash"; then
    echo "FAIL: Script does not have correct shebang"
    exit 1
fi
echo "✓ PASS"
echo ""

# Test 3: Script contains required functions and logic
echo "Test 3: Script contains required functions and variables"
required_vars=("COVERAGE_FILE" "KEY_PACKAGES" "CRITICAL_PACKAGES")
for var in "${required_vars[@]}"; do
    if ! grep -q "$var" "$SCRIPT"; then
        echo "FAIL: Script does not contain required variable: $var"
        exit 1
    fi
done
echo "✓ PASS"
echo ""

# Test 4: Script has compute_package_coverage function
echo "Test 4: Script has compute_package_coverage function"
if ! grep -q "compute_package_coverage" "$SCRIPT"; then
    echo "FAIL: Script does not contain compute_package_coverage function"
    exit 1
fi
echo "✓ PASS"
echo ""

# Test 5: Test output format (dry run to stdout)
# We can't run the full script in tests, but we can validate structure
echo "Test 5: Script structure validation"
if ! grep -q "## Baseline Test and Coverage Metrics" "$SCRIPT"; then
    echo "FAIL: Script does not generate expected markdown header"
    exit 1
fi
echo "✓ PASS"
echo ""

# Test 6: Verify script handles output file parameter
echo "Test 6: Script accepts output file parameter"
if ! grep -q 'OUTPUT_FILE="${1:-CHECKPOINT_MODS.md}"' "$SCRIPT"; then
    echo "FAIL: Script does not handle output file parameter correctly"
    exit 1
fi
echo "✓ PASS"
echo ""

# Test 7: Verify script includes all critical packages from check-critical-coverage.sh
echo "Test 7: Script includes scheduler and jobs in critical packages"
if ! grep -q "internal/server/scheduler" "$SCRIPT"; then
    echo "FAIL: Script missing scheduler package"
    exit 1
fi
if ! grep -q "internal/worker/jobs" "$SCRIPT"; then
    echo "FAIL: Script missing jobs package"
    exit 1
fi
echo "✓ PASS"
echo ""

# Test 8: Verify script includes key packages from ROADMAP
echo "Test 8: Script includes key packages from ROADMAP.md"
roadmap_packages=("internal/workflow/backoff" "internal/nodeagent" "internal/cli/stream" "cmd/ploy")
for pkg in "${roadmap_packages[@]}"; do
    if ! grep -q "$pkg" "$SCRIPT"; then
        echo "FAIL: Script missing key package: $pkg"
        exit 1
    fi
done
echo "✓ PASS"
echo ""

echo "=== All Tests Passed ==="
echo ""
echo "Note: Full integration test requires running 'make test' which takes several minutes."
echo "Run the script manually to generate actual baseline metrics:"
echo "  ./scripts/capture-baseline-metrics.sh CHECKPOINT_MODS.md"
