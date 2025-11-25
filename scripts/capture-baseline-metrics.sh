#!/usr/bin/env bash
# capture-baseline-metrics.sh - Capture baseline test and coverage metrics
# Usage: ./scripts/capture-baseline-metrics.sh [output-file]
#
# This script establishes a baseline for test and coverage metrics before
# refactors, as specified in ROADMAP.md line 128. It captures:
# - Overall test pass/fail status
# - Overall coverage percentage (target: ≥60%)
# - Per-package coverage for critical workflow/runner packages (target: ≥90%)
# - Key packages: internal/workflow/..., internal/nodeagent/..., internal/cli/stream, cmd/ploy
#
# The output is formatted as markdown suitable for inclusion in mods checkpoint docs.
# or similar documentation.

set -euo pipefail

OUTPUT_FILE="${1:-CHECKPOINT_MODS.md}"
COVERAGE_FILE="dist/coverage.out"
TIMESTAMP=$(date -u +"%Y-%m-%d %H:%M:%S UTC")

# Key packages per ROADMAP.md line 130
# These packages are measured for the baseline to ensure overall coverage stays ≥60%
# and critical workflow/runner packages remain ≥90%
KEY_PACKAGES=(
    "github.com/iw2rmb/ploy/internal/workflow/backoff"
    "github.com/iw2rmb/ploy/internal/workflow/runner"
    "github.com/iw2rmb/ploy/internal/workflow/tickets"
    "github.com/iw2rmb/ploy/internal/nodeagent"
    "github.com/iw2rmb/ploy/internal/nodeagent/claim"
    "github.com/iw2rmb/ploy/internal/cli/stream"
    "github.com/iw2rmb/ploy/cmd/ploy"
)

# Critical packages requiring ≥90% coverage (from check-critical-coverage.sh)
CRITICAL_PACKAGES=(
    "github.com/iw2rmb/ploy/internal/server/scheduler"
    "github.com/iw2rmb/ploy/internal/worker/jobs"
)

echo "=== Capturing Baseline Test and Coverage Metrics ==="
echo ""

# Step 1: Run tests and generate coverage report
echo "Running tests with coverage..."
mkdir -p dist
# Use a temporary directory for test isolation (matching Makefile pattern)
TMP=$(mktemp -d 2>/dev/null || mktemp -d -t ploytest)
trap "rm -rf $TMP" EXIT

if PLOY_CONFIG_HOME="$TMP" go test -coverprofile="$COVERAGE_FILE" -covermode=atomic ./... 2>&1 | tee "$TMP/test-output.txt"; then
    TEST_STATUS="PASS"
    TEST_EXIT_CODE=0
else
    TEST_STATUS="FAIL"
    TEST_EXIT_CODE=$?
fi

# Count test results
TOTAL_TESTS=$(grep -E "^(ok|FAIL)" "$TMP/test-output.txt" | wc -l | tr -d ' ')
PASSED_TESTS=$(grep "^ok" "$TMP/test-output.txt" | wc -l | tr -d ' ')
FAILED_TESTS=$(grep "^FAIL" "$TMP/test-output.txt" | wc -l | tr -d ' ')

echo ""
echo "=== Test Results ==="
echo "Status: $TEST_STATUS"
echo "Total packages tested: $TOTAL_TESTS"
echo "Passed: $PASSED_TESTS"
echo "Failed: $FAILED_TESTS"
echo ""

# Step 2: Extract overall coverage
if [ ! -f "$COVERAGE_FILE" ]; then
    echo "ERROR: Coverage file $COVERAGE_FILE not found"
    exit 1
fi

OVERALL_COVERAGE=$(go tool cover -func="$COVERAGE_FILE" | grep '^total:' | awk '{print $3}' | sed 's/%//')
echo "Overall coverage: ${OVERALL_COVERAGE}%"
echo ""

# Step 3: Extract per-package coverage for key packages
# This function computes statement-weighted coverage from the coverage file
compute_package_coverage() {
    local pkg_path="$1"
    local coverage_file="$2"

    # Statement-weighted computation from coverage file
    # Each line in coverage.out has: file:startline.col,endline.col numstmts count
    # We sum numstmts where count>0 (covered) divided by total numstmts
    awk -v p="${pkg_path}/" '
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
            if (total>0) printf "%.1f", (covered*100.0)/total;
            else print "N/A";
        }
    ' "$coverage_file"
}

echo "=== Key Package Coverage ==="
declare -A PACKAGE_COVERAGE
for pkg in "${KEY_PACKAGES[@]}"; do
    cov=$(compute_package_coverage "$pkg" "$COVERAGE_FILE")
    PACKAGE_COVERAGE["$pkg"]="$cov"
    printf "  %-60s %s%%\n" "$pkg" "$cov"
done
echo ""

echo "=== Critical Package Coverage (≥90% target) ==="
for pkg in "${CRITICAL_PACKAGES[@]}"; do
    cov=$(compute_package_coverage "$pkg" "$COVERAGE_FILE")
    PACKAGE_COVERAGE["$pkg"]="$cov"
    printf "  %-60s %s%%\n" "$pkg" "$cov"
done
echo ""

# Step 4: Generate markdown output
echo "=== Generating Baseline Report ==="

# Create the markdown section
MARKDOWN=$(cat <<EOF

## Baseline Test and Coverage Metrics

**Captured**: $TIMESTAMP
**Purpose**: Establish a starting point before refactors (ROADMAP.md line 128)
**Coverage Target**: ≥60% overall, ≥90% for critical workflow/runner packages

### Test Results

- **Status**: $TEST_STATUS
- **Total packages tested**: $TOTAL_TESTS
- **Passed**: $PASSED_TESTS
- **Failed**: $FAILED_TESTS
- **Exit code**: $TEST_EXIT_CODE

### Overall Coverage

- **Overall coverage**: ${OVERALL_COVERAGE}%
- **Coverage file**: \`$COVERAGE_FILE\`

### Key Package Coverage

Package coverage for critical workflow, nodeagent, and CLI packages:

| Package | Coverage |
|---------|----------|
EOF
)

# Add key packages to table
for pkg in "${KEY_PACKAGES[@]}"; do
    cov="${PACKAGE_COVERAGE[$pkg]}"
    MARKDOWN+=$'\n'"| \`$pkg\` | ${cov}% |"
done

# Add critical packages section
MARKDOWN+=$'\n\n'"### Critical Package Coverage (≥90% target)"$'\n\n'
MARKDOWN+="| Package | Coverage | Status |"$'\n'
MARKDOWN+="|---------|----------|--------|"$'\n'

for pkg in "${CRITICAL_PACKAGES[@]}"; do
    cov="${PACKAGE_COVERAGE[$pkg]}"
    # Determine status (PASS if ≥90%, WARN if <90%)
    if [ "$cov" = "N/A" ]; then
        status="⚠️  N/A"
    elif awk -v c="$cov" 'BEGIN{exit(c>=90)}'; then
        status="⚠️  Below target"
    else
        status="✅ Pass"
    fi
    MARKDOWN+=$'\n'"| \`$pkg\` | ${cov}% | $status |"
done

MARKDOWN+=$'\n\n'"### Notes"$'\n\n'
MARKDOWN+="- This baseline was captured using \`make test\` (which runs \`go test -coverprofile=... -covermode=atomic ./...\`)"$'\n'
MARKDOWN+="- Coverage is statement-weighted (atomic mode) for accuracy"$'\n'
MARKDOWN+="- Key packages are from ROADMAP.md line 130: \`internal/workflow/...\`, \`internal/nodeagent/...\`, \`internal/cli/stream\`, \`cmd/ploy\`"$'\n'
MARKDOWN+="- Critical packages requiring ≥90% coverage are defined in \`scripts/check-critical-coverage.sh\`"$'\n'
MARKDOWN+="- Use this baseline to ensure coverage does not regress during refactoring"$'\n'

# Step 5: Write to output file
if [ "$OUTPUT_FILE" != "-" ]; then
    echo ""
    echo "Appending baseline metrics to $OUTPUT_FILE..."
    echo "$MARKDOWN" >> "$OUTPUT_FILE"
    echo "✓ Baseline metrics captured and written to $OUTPUT_FILE"
else
    # Write to stdout if output file is "-"
    echo "$MARKDOWN"
fi

echo ""
echo "=== Baseline Capture Complete ==="
exit $TEST_EXIT_CODE
