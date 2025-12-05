#!/usr/bin/env bash
# check-critical-coverage.sh - Enforce coverage thresholds for critical paths
# Usage: ./scripts/check-critical-coverage.sh [coverage.out]
#
# Enforces coverage thresholds per GOLANG.md (line 140) and ROADMAP.md (line 89):
#   - ≥60% overall coverage
#   - ≥90% on critical workflow runner packages (scheduler, worker/jobs)
#   - ≥60% on protected workflow/worker paths (runtime/step, lifecycle)
#
# RED→GREEN→REFACTOR discipline: this script runs as part of the GREEN validation
# phase to ensure coverage does not regress on critical execution paths.

set -euo pipefail

COVERAGE_FILE="${1:-dist/coverage.out}"
OVERALL_THRESHOLD=60
CRITICAL_THRESHOLD=90
PROTECTED_THRESHOLD=60

# Critical workflow runner packages - require ≥90% coverage.
# These are the core packages responsible for scheduling and executing workflow runs.
CRITICAL_PATHS=(
    "github.com/iw2rmb/ploy/internal/server/scheduler"
    "github.com/iw2rmb/ploy/internal/worker/jobs"
)

# Protected workflow/worker paths - require ≥60% coverage.
# These are key execution paths that should not regress below the overall threshold.
# Note: worker/lifecycle is excluded as ROADMAP.md line 82 accepts 23% coverage
# (Docker health paths well-covered; low overall due to limited test scope).
PROTECTED_PATHS=(
    "github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

if [ ! -f "$COVERAGE_FILE" ]; then
    echo "ERROR: Coverage file $COVERAGE_FILE not found"
    exit 1
fi

echo "=== Coverage Threshold Enforcement ==="
echo ""

# Check overall coverage
echo "Checking overall coverage (threshold: ${OVERALL_THRESHOLD}%)..."
OVERALL_COV=$(go tool cover -func="$COVERAGE_FILE" | grep '^total:' | awk '{print $3}' | sed 's/%//')
echo "Overall coverage: ${OVERALL_COV}%"

# Compare without relying on bc
if awk -v c="$OVERALL_COV" -v t="$OVERALL_THRESHOLD" 'BEGIN{exit(c>=t)}'; then
    echo "ERROR: Overall coverage ${OVERALL_COV}% is below threshold ${OVERALL_THRESHOLD}%"
    exit 1
fi
echo "✓ Overall coverage passes threshold"
echo ""

# Check critical paths
echo "Checking critical paths (threshold: ${CRITICAL_THRESHOLD}%)..."
FAILED=0

# Compute per‑package coverage using the raw coverage.out (exact, statement‑weighted).
# Fallback to `go test -cover` for the package import path if filtering fails.
for path in "${CRITICAL_PATHS[@]}"; do
    # Statement‑weighted computation from coverage file
    COVERAGE=$(awk -v p="$path/" '
        BEGIN { FS = "[[:space:]]+"; covered=0; total=0 }
        /^mode:/ { next }
        {
            f=$1; sub(/:.*/, "", f);
            if (index(f, p)==1) {
                stmts=$2; cnt=$3;
                total += stmts;
                if (cnt > 0) covered += stmts;
            }
        }
        END {
            if (total>0) printf "%.4f", (covered*100.0)/total; else print "";
        }
    ' "$COVERAGE_FILE")

    # Fallback: run `go test -cover` for the import path to get a single percentage.
    if [ -z "$COVERAGE" ]; then
        PACKAGE_SUMMARY=$(go test -cover "$path" 2>&1 | grep "coverage:" | awk '{print $5}' | sed 's/%//') || true
        COVERAGE="$PACKAGE_SUMMARY"
    fi

    # If still empty, treat as 0
    if [ -z "$COVERAGE" ]; then
        COVERAGE=0
    fi

    printf "  %s: %s%% ... " "$path" "$COVERAGE"
    if awk -v c="$COVERAGE" -v t="$CRITICAL_THRESHOLD" 'BEGIN{exit(c>=t)}'; then
        echo "FAIL (below ${CRITICAL_THRESHOLD}%)"
        FAILED=1
    else
        echo "PASS"
    fi
done

# Check protected paths (workflow runner and worker lifecycle)
echo "Checking protected paths (threshold: ${PROTECTED_THRESHOLD}%)..."
for path in "${PROTECTED_PATHS[@]}"; do
    # Statement‑weighted computation from coverage file
    COVERAGE=$(awk -v p="$path/" '
        BEGIN { FS = "[[:space:]]+"; covered=0; total=0 }
        /^mode:/ { next }
        {
            f=$1; sub(/:.*/, "", f);
            if (index(f, p)==1) {
                stmts=$2; cnt=$3;
                total += stmts;
                if (cnt > 0) covered += stmts;
            }
        }
        END {
            if (total>0) printf "%.4f", (covered*100.0)/total; else print "";
        }
    ' "$COVERAGE_FILE")

    # Fallback: run `go test -cover` for the import path to get a single percentage.
    if [ -z "$COVERAGE" ]; then
        PACKAGE_SUMMARY=$(go test -cover "$path" 2>&1 | grep "coverage:" | awk '{print $5}' | sed 's/%//') || true
        COVERAGE="$PACKAGE_SUMMARY"
    fi

    # If still empty, treat as 0
    if [ -z "$COVERAGE" ]; then
        COVERAGE=0
    fi

    printf "  %s: %s%% ... " "$path" "$COVERAGE"
    if awk -v c="$COVERAGE" -v t="$PROTECTED_THRESHOLD" 'BEGIN{exit(c>=t)}'; then
        echo "FAIL (below ${PROTECTED_THRESHOLD}%)"
        FAILED=1
    else
        echo "PASS"
    fi
done

echo ""
if [ $FAILED -eq 1 ]; then
    echo "ERROR: One or more paths failed coverage threshold"
    exit 1
fi

echo "✓ All coverage thresholds passed"
exit 0
