#!/usr/bin/env bash
# check-critical-coverage.sh - Enforce coverage thresholds for critical paths
# Usage: ./scripts/check-critical-coverage.sh [coverage.out]

set -euo pipefail

COVERAGE_FILE="${1:-dist/coverage.out}"
OVERALL_THRESHOLD=60
CRITICAL_THRESHOLD=90

# Critical paths as defined in ROADMAP.md line 10
CRITICAL_PATHS=(
    "github.com/iw2rmb/ploy/internal/api/scheduler"
    "github.com/iw2rmb/ploy/internal/api/pki"
    "github.com/iw2rmb/ploy/internal/node/jobs"
)

if [ ! -f "$COVERAGE_FILE" ]; then
    echo "ERROR: Coverage file $COVERAGE_FILE not found"
    exit 1
fi

echo "=== Coverage Threshold Enforcement ==="
echo ""

# Check overall coverage
echo "Checking overall coverage (threshold: ${OVERALL_THRESHOLD}%)..."
OVERALL_COV=$(go tool cover -func="$COVERAGE_FILE" | grep total: | awk '{print $3}' | sed 's/%//')
echo "Overall coverage: ${OVERALL_COV}%"

if (( $(echo "$OVERALL_COV < $OVERALL_THRESHOLD" | bc -l) )); then
    echo "ERROR: Overall coverage ${OVERALL_COV}% is below threshold ${OVERALL_THRESHOLD}%"
    exit 1
fi
echo "✓ Overall coverage passes threshold"
echo ""

# Check critical paths
echo "Checking critical paths (threshold: ${CRITICAL_THRESHOLD}%)..."
FAILED=0

for path in "${CRITICAL_PATHS[@]}"; do
    # Extract coverage for this path
    COVERAGE=$(go tool cover -func="$COVERAGE_FILE" | grep "^$path" | awk '{sum+=$3; count++} END {if (count > 0) print sum/count; else print 0}' | sed 's/%//')

    # Handle empty coverage (package might not have statements)
    if [ -z "$COVERAGE" ] || [ "$COVERAGE" = "0" ]; then
        # Try alternative approach - get package summary
        PACKAGE_SUMMARY=$(go test -coverprofile=/dev/null -covermode=atomic "./$path" 2>&1 | grep "coverage:" | awk '{print $5}' | sed 's/%//')
        if [ -n "$PACKAGE_SUMMARY" ]; then
            COVERAGE="$PACKAGE_SUMMARY"
        else
            COVERAGE="0"
        fi
    fi

    echo -n "  $path: ${COVERAGE}% ... "

    if (( $(echo "$COVERAGE < $CRITICAL_THRESHOLD" | bc -l) )); then
        echo "FAIL (below ${CRITICAL_THRESHOLD}%)"
        FAILED=1
    else
        echo "PASS"
    fi
done

echo ""
if [ $FAILED -eq 1 ]; then
    echo "ERROR: One or more critical paths failed coverage threshold"
    exit 1
fi

echo "✓ All coverage thresholds passed"
exit 0
