#!/usr/bin/env bash
# validate-tdd-discipline.sh
#
# Purpose: Enforce RED→GREEN→REFACTOR discipline for test-driven development.
#
# This script validates that:
# 1. Tests exist and pass for all code changes (GREEN phase requirement)
# 2. Coverage thresholds are met (≥60% overall, ≥90% critical paths)
# 3. Binary size remains under threshold (protects against dependency bloat)
# 4. All CI checks pass (vet, staticcheck)
#
# Usage:
#   ./scripts/validate-tdd-discipline.sh [package-path...]
#
# Examples:
#   ./scripts/validate-tdd-discipline.sh                          # Validate entire repository
#   ./scripts/validate-tdd-discipline.sh ./internal/workflow/...  # Validate specific package
#
# Exit codes:
#   0 - All TDD discipline checks passed
#   1 - One or more checks failed (tests, coverage, binary size, linting)
#
# References:
#   GOLANG.md: TDD + coverage requirements (line 135-141)
#   ROADMAP.md: RED→GREEN→REFACTOR discipline (line 133-136)

set -euo pipefail

# Color output for readability
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
BUILD_DIR="${BUILD_DIR:-dist}"
BINARY="${BINARY:-ploy}"
COVERAGE_FILE="${BUILD_DIR}/coverage.out"
BINARY_SIZE_THRESHOLD_MB=15

# Track overall success
VALIDATION_FAILED=0

log_info() {
    echo -e "${GREEN}[INFO]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*"
    VALIDATION_FAILED=1
}

run_check() {
    local check_name="$1"
    shift

    echo ""
    log_info "Running: ${check_name}"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    if "$@"; then
        log_info "✓ ${check_name} passed"
    else
        log_error "✗ ${check_name} failed"
    fi
}

check_prerequisites() {
    log_info "Checking prerequisites..."

    # Ensure we're in a git repository
    if ! git rev-parse --git-dir > /dev/null 2>&1; then
        log_error "Not in a git repository"
        return 1
    fi

    # Ensure Go is available
    if ! command -v go > /dev/null 2>&1; then
        log_error "Go not found in PATH"
        return 1
    fi

    log_info "Prerequisites satisfied"
}

# Phase 1: RED - Verify tests exist and can detect failures
# (This is validated by ensuring tests exist for changed code)
check_test_coverage_exists() {
    local packages="${1:-./...}"

    log_info "Verifying test files exist for packages..."

    # Find all Go packages with implementation files
    local impl_packages
    impl_packages=$(go list -f '{{.Dir}}' "$packages" | sort -u)

    local missing_tests=0
    while IFS= read -r pkg_dir; do
        # Skip vendor, dist, and other non-source directories
        if [[ "$pkg_dir" == *"/vendor/"* ]] || \
           [[ "$pkg_dir" == *"/dist/"* ]] || \
           [[ "$pkg_dir" == *"/tools/"* ]]; then
            continue
        fi

        # Check if package has Go files (excluding tests)
        if ! ls "$pkg_dir"/*.go 2>/dev/null | grep -v '_test\.go$' > /dev/null; then
            continue
        fi

        # Check if package has test files
        if ! ls "$pkg_dir"/*_test.go 2>/dev/null > /dev/null; then
            log_warn "No test files found in: ${pkg_dir#$(pwd)/}"
            ((missing_tests++)) || true
        fi
    done <<< "$impl_packages"

    if [ "$missing_tests" -gt 0 ]; then
        log_warn "Found $missing_tests package(s) without test files"
        log_warn "TDD discipline requires tests for all code (RED phase)"
    else
        log_info "All packages have test files"
    fi
}

# Phase 2: GREEN - Verify tests pass
check_tests_pass() {
    local packages="${1:-./...}"

    log_info "Running tests for: $packages"

    # Create temporary directory for isolated config
    local tmp_dir
    tmp_dir=$(mktemp -d 2>/dev/null || mktemp -d -t ploytest)

    # Run tests with coverage
    if PLOY_CONFIG_HOME="$tmp_dir" go test -cover "$packages"; then
        rm -rf "$tmp_dir"
        log_info "All tests passed (GREEN phase complete)"
        return 0
    else
        rm -rf "$tmp_dir"
        log_error "Tests failed - must achieve GREEN before proceeding"
        return 1
    fi
}

# Phase 3: GREEN validation - Coverage thresholds
check_coverage_thresholds() {
    log_info "Validating coverage thresholds..."

    # Ensure coverage file exists
    if [ ! -f "$COVERAGE_FILE" ]; then
        log_error "Coverage file not found: $COVERAGE_FILE"
        log_error "Run 'make test-coverage' first"
        return 1
    fi

    # Check overall coverage (≥60%)
    local coverage
    coverage=$(go tool cover -func="$COVERAGE_FILE" | grep '^total:' | awk '{print $3}' | sed 's/%//')
    local threshold=60

    echo "Overall coverage: ${coverage}% (threshold: ${threshold}%)"

    if awk -v c="$coverage" -v t="$threshold" 'BEGIN{exit(c>=t)}'; then
        log_error "Coverage ${coverage}% is below threshold ${threshold}%"
        return 1
    fi

    # Check critical path coverage (≥90%)
    if ! ./scripts/check-critical-coverage.sh "$COVERAGE_FILE"; then
        log_error "Critical path coverage below 90% threshold"
        return 1
    fi

    log_info "Coverage thresholds met"
}

# Phase 4: REFACTOR validation - Binary size (detect dependency bloat)
check_binary_size() {
    log_info "Validating binary size..."

    if [ ! -f "${BUILD_DIR}/${BINARY}" ]; then
        log_error "Binary not found: ${BUILD_DIR}/${BINARY}"
        log_error "Run 'make build' first"
        return 1
    fi

    if ! ./scripts/check-binary-size.sh "${BUILD_DIR}/${BINARY}" "$BINARY_SIZE_THRESHOLD_MB"; then
        log_error "Binary size exceeds threshold (dependency bloat detected)"
        return 1
    fi

    log_info "Binary size within threshold"
}

# Code quality checks (vet, staticcheck)
check_code_quality() {
    log_info "Running go vet..."
    if ! go vet ./...; then
        log_error "go vet found issues"
        return 1
    fi

    if command -v staticcheck > /dev/null 2>&1; then
        log_info "Running staticcheck..."
        if ! staticcheck ./...; then
            log_error "staticcheck found issues"
            return 1
        fi
    else
        log_warn "staticcheck not installed (optional but recommended)"
        log_warn "Install: go install honnef.co/go/tools/cmd/staticcheck@latest"
    fi

    log_info "Code quality checks passed"
}

main() {
    local packages="${1:-./...}"

    echo "╔════════════════════════════════════════════════════════════╗"
    echo "║       TDD Discipline Validation (RED→GREEN→REFACTOR)      ║"
    echo "╚════════════════════════════════════════════════════════════╝"
    echo ""

    # Prerequisites
    if ! check_prerequisites; then
        exit 1
    fi

    # RED phase: Verify tests exist
    run_check "Test existence (RED phase)" check_test_coverage_exists "$packages"

    # GREEN phase: Tests must pass
    run_check "Tests passing (GREEN phase)" check_tests_pass "$packages"

    # GREEN validation: Coverage thresholds
    if [ -f "$COVERAGE_FILE" ]; then
        run_check "Coverage thresholds (GREEN validation)" check_coverage_thresholds
    else
        log_warn "Skipping coverage threshold checks (no coverage file)"
        log_warn "Run 'make test-coverage' to generate coverage report"
    fi

    # REFACTOR validation: Binary size
    if [ -f "${BUILD_DIR}/${BINARY}" ]; then
        run_check "Binary size (REFACTOR validation)" check_binary_size
    else
        log_warn "Skipping binary size check (binary not built)"
        log_warn "Run 'make build' to build binary"
    fi

    # Code quality
    run_check "Code quality (vet + staticcheck)" check_code_quality

    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    if [ "$VALIDATION_FAILED" -eq 0 ]; then
        echo -e "${GREEN}✓ All TDD discipline checks passed${NC}"
        echo ""
        log_info "RED→GREEN→REFACTOR discipline maintained"
        echo ""
        return 0
    else
        echo -e "${RED}✗ TDD discipline validation failed${NC}"
        echo ""
        log_error "Fix failing checks before proceeding"
        log_error "Reference: GOLANG.md (line 135-141), ROADMAP.md (line 133-136)"
        echo ""
        return 1
    fi
}

main "$@"
