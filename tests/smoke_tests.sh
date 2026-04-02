#!/usr/bin/env bash
# Smoke test runner for critical Ploy workflows.
#
# This script validates end-to-end behavior across:
# - Integration tests (database, server, SSE)
# - CLI functionality (version, help)
# - E2E selftest (container execution)
#
# Usage:
#   bash tests/smoke_tests.sh [--quick|--full]
#
# Modes:
#   --quick: Run fast unit, CLI, and integration tests (no e2e)
#   --full:  Run all tests including e2e scenarios (requires cluster)
#
# Environment:
#   PLOY_TEST_DB_DSN: PostgreSQL connection string for integration tests
#                     Example: postgresql://user:pass@localhost:5432/ploy_test
#   SKIP_E2E:         Set to "1" to skip e2e tests even in --full mode
#
# Exit codes:
#   0: All selected tests passed
#   1: One or more tests failed

set -euo pipefail

# Default mode: quick (fast tests only)
MODE="${1:-quick}"

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test results tracking
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0
FAILED_TESTS=()

# Print colored status messages
log_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[PASS]${NC} $*"
}

log_error() {
    echo -e "${RED}[FAIL]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

log_section() {
    echo ""
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}$*${NC}"
    echo -e "${BLUE}========================================${NC}"
}

# Record test result
record_test() {
    local test_name="$1"
    local exit_code="$2"

    TESTS_RUN=$((TESTS_RUN + 1))
    if [[ $exit_code -eq 0 ]]; then
        TESTS_PASSED=$((TESTS_PASSED + 1))
        log_success "$test_name"
    else
        TESTS_FAILED=$((TESTS_FAILED + 1))
        FAILED_TESTS+=("$test_name")
        log_error "$test_name (exit code: $exit_code)"
    fi
}

# Run a test with timeout and result recording
run_test() {
    local test_name="$1"
    local test_cmd="$2"
    local timeout_sec="${3:-300}" # Default 5 min timeout

    log_info "Running: $test_name"

    # Run test with timeout, capture exit code
    set +e
    timeout "$timeout_sec" bash -c "$test_cmd" >/dev/null 2>&1
    local exit_code=$?
    set -e

    # Check if timeout occurred (exit code 124)
    if [[ $exit_code -eq 124 ]]; then
        log_error "$test_name (timeout after ${timeout_sec}s)"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        FAILED_TESTS+=("$test_name (timeout)")
        TESTS_RUN=$((TESTS_RUN + 1))
    else
        record_test "$test_name" "$exit_code"
    fi
}

# Verify prerequisites
check_prerequisites() {
    log_section "Checking Prerequisites"

    # Check if ploy binary exists
    if [[ ! -f dist/ploy ]]; then
        log_error "dist/ploy not found. Run 'make build' first."
        exit 1
    fi
    log_success "Found ploy binary: dist/ploy"

    # Check if go is available
    if ! command -v go >/dev/null 2>&1; then
        log_error "go not found in PATH"
        exit 1
    fi
    log_success "Found go: $(go version)"

    # For integration tests, check database connectivity
    if [[ -n "${PLOY_TEST_DB_DSN:-}" ]]; then
        log_success "PLOY_TEST_DB_DSN is set for integration tests"
    else
        log_warn "PLOY_TEST_DB_DSN not set; database integration tests will be skipped"
    fi
}

# Run integration tests (database, server, SSE)
run_integration_tests() {
    log_section "Integration Tests"

    if [[ -z "${PLOY_TEST_DB_DSN:-}" ]]; then
        log_warn "Skipping integration tests (PLOY_TEST_DB_DSN not set)"
        return 0
    fi

    # Test 1: Database operations (happy path)
    run_test "Integration: Database happy path" \
        "go test -v -timeout=60s ./tests/integration -run=TestHappyPath_CreateRepoModRun" \
        60

    # Test 2: Database operations (lab smoke)
    run_test "Integration: Database lab smoke" \
        "go test -v -timeout=60s ./tests/integration -run=TestLabSmoke" \
        60

    # Test 3: Server start/stop (insecure mode)
    run_test "Integration: Server start/stop" \
        "go test -v -timeout=90s ./tests/integration -run=TestServerStartStop_InsecureMode" \
        90
}

# Run CLI smoke tests
run_cli_tests() {
    log_section "CLI Smoke Tests"

    # Test 1: CLI version command
    run_test "CLI: version" \
        "dist/ploy version" \
        5

    # Test 2: CLI help command
    run_test "CLI: help" \
        "dist/ploy help" \
        5

    # Test 3: CLI mig help (shows usage even with error code; check output exists)
    run_test "CLI: mig help" \
        "dist/ploy mig --help 2>&1 | grep -q 'Usage: ploy mig'" \
        5

    # Test 4: CLI cluster help (shows usage even with error code; check output exists)
    # NOTE: `ploy server` has been re-rooted under `ploy cluster deploy`.
    # We now test the cluster command instead.
    run_test "CLI: cluster help" \
        "dist/ploy cluster --help 2>&1 | grep -q 'Usage: ploy cluster'" \
        5

    # Test 5: CLI cluster deploy help (check deploy usage is cluster-scoped)
    run_test "CLI: cluster deploy help" \
        "dist/ploy cluster deploy --help 2>&1 | grep -q 'Usage: ploy cluster deploy'" \
        5
}

# Run fast unit tests for critical packages
run_unit_tests() {
    log_section "Unit Tests (Critical Packages)"

    # Test backoff package (retry logic)
    run_test "Unit: backoff package" \
        "go test -v -timeout=30s ./internal/workflow/backoff/..." \
        30

    # Test SSE stream client
    run_test "Unit: SSE stream client" \
        "go test -v -timeout=30s ./internal/cli/stream/..." \
        30

    # Test GitLab MR client
    run_test "Unit: GitLab MR client" \
        "go test -v -timeout=30s ./internal/nodeagent/gitlab/..." \
        30
}

# Run e2e selftest scenario (minimal container execution)
run_e2e_selftest() {
    log_section "E2E Smoke Test (Selftest)"

    if [[ "${SKIP_E2E:-0}" == "1" ]]; then
        log_warn "Skipping e2e tests (SKIP_E2E=1)"
        return 0
    fi

    # Check if cluster is configured
    if [[ ! -d ~/.config/ploy/default ]]; then
        log_warn "Skipping e2e selftest (no cluster configured at ~/.config/ploy/default)"
        return 0
    fi

    # Run selftest scenario (fast container echo test)
    run_test "E2E: Selftest scenario" \
        "bash tests/e2e/migs/scenario-selftest.sh" \
        180
}

# Print summary
print_summary() {
    log_section "Test Summary"

    echo "Tests run:    $TESTS_RUN"
    echo "Tests passed: $TESTS_PASSED"
    echo "Tests failed: $TESTS_FAILED"

    if [[ $TESTS_FAILED -gt 0 ]]; then
        echo ""
        log_error "Failed tests:"
        for test in "${FAILED_TESTS[@]}"; do
            echo "  - $test"
        done
        echo ""
        exit 1
    else
        echo ""
        log_success "All tests passed!"
        exit 0
    fi
}

# Main execution flow
main() {
    log_info "Ploy Smoke Tests Runner"
    log_info "Mode: $MODE"
    echo ""

    check_prerequisites

    # Always run fast tests
    run_unit_tests
    run_cli_tests
    run_integration_tests

    # Full mode includes e2e tests
    if [[ "$MODE" == "--full" || "$MODE" == "full" ]]; then
        run_e2e_selftest
    fi

    print_summary
}

# Run main
main "$@"
