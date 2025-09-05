#!/bin/bash
# Common test utilities for Ploy test scripts
# This file should be sourced by test scripts: source "$(dirname "$0")/common/test-utils.sh"

# ============================================================================
# Color Definitions
# ============================================================================
export RED='\033[0;31m'
export GREEN='\033[0;32m'
export YELLOW='\033[1;33m'
export BLUE='\033[0;34m'
export MAGENTA='\033[0;35m'
export CYAN='\033[0;36m'
export WHITE='\033[1;37m'
export NC='\033[0m' # No Color
export BOLD='\033[1m'

# ============================================================================
# Logging Functions
# ============================================================================
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_debug() {
    if [[ "${DEBUG:-false}" == "true" ]]; then
        echo -e "${CYAN}[DEBUG]${NC} $1"
    fi
}

log_section() {
    echo -e "\n${BOLD}${BLUE}=== $1 ===${NC}"
}

log_subsection() {
    echo -e "\n${CYAN}--- $1 ---${NC}"
}

# ============================================================================
# Test Result Functions
# ============================================================================
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_SKIPPED=0

test_passed() {
    local test_name="$1"
    ((TESTS_PASSED++))
    echo -e "${GREEN}✓${NC} $test_name"
}

test_failed() {
    local test_name="$1"
    local reason="${2:-Unknown reason}"
    ((TESTS_FAILED++))
    echo -e "${RED}✗${NC} $test_name"
    echo -e "  ${RED}Reason: $reason${NC}"
}

test_skipped() {
    local test_name="$1"
    local reason="${2:-}"
    ((TESTS_SKIPPED++))
    echo -e "${YELLOW}⊗${NC} $test_name"
    [[ -n "$reason" ]] && echo -e "  ${YELLOW}Reason: $reason${NC}"
}

print_test_summary() {
    echo
    echo -e "${BOLD}Test Summary:${NC}"
    echo -e "  ${GREEN}Passed:${NC} $TESTS_PASSED"
    echo -e "  ${RED}Failed:${NC} $TESTS_FAILED"
    echo -e "  ${YELLOW}Skipped:${NC} $TESTS_SKIPPED"
    
    if [[ $TESTS_FAILED -gt 0 ]]; then
        echo -e "\n${RED}${BOLD}TEST SUITE FAILED${NC}"
        return 1
    else
        echo -e "\n${GREEN}${BOLD}TEST SUITE PASSED${NC}"
        return 0
    fi
}

# ============================================================================
# Assertion Functions
# ============================================================================
assert_equals() {
    local expected="$1"
    local actual="$2"
    local test_name="${3:-Equality test}"
    
    if [[ "$expected" == "$actual" ]]; then
        test_passed "$test_name"
        return 0
    else
        test_failed "$test_name" "Expected: '$expected', Got: '$actual'"
        return 1
    fi
}

assert_not_empty() {
    local value="$1"
    local test_name="${2:-Non-empty test}"
    
    if [[ -n "$value" ]]; then
        test_passed "$test_name"
        return 0
    else
        test_failed "$test_name" "Value is empty"
        return 1
    fi
}

assert_file_exists() {
    local file="$1"
    local test_name="${2:-File existence test}"
    
    if [[ -f "$file" ]]; then
        test_passed "$test_name"
        return 0
    else
        test_failed "$test_name" "File not found: $file"
        return 1
    fi
}

assert_command_succeeds() {
    local command="$1"
    local test_name="${2:-Command execution test}"
    
    if eval "$command" > /dev/null 2>&1; then
        test_passed "$test_name"
        return 0
    else
        test_failed "$test_name" "Command failed: $command"
        return 1
    fi
}

assert_contains() {
    local haystack="$1"
    local needle="$2"
    local test_name="${3:-Contains test}"
    
    if echo "$haystack" | grep -q "$needle"; then
        test_passed "$test_name"
        return 0
    else
        test_failed "$test_name" "String '$needle' not found in output"
        return 1
    fi
}

# ============================================================================
# Environment Setup Functions
# ============================================================================
setup_test_environment() {
    # Common environment variables
    export TEST_DIR="${TEST_DIR:-/tmp/ploy-tests-$$}"
    export API_URL="${API_URL:-https://api.dev.ployman.app}"
    export API_VERSION="${API_VERSION:-v1}"
    export TEST_TIMEOUT="${TEST_TIMEOUT:-30}"
    
    # Create test directory
    mkdir -p "$TEST_DIR"
    log_debug "Test directory: $TEST_DIR"
}

cleanup_test_environment() {
    if [[ -d "$TEST_DIR" ]]; then
        log_debug "Cleaning up test directory: $TEST_DIR"
        rm -rf "$TEST_DIR"
    fi
}

# ============================================================================
# API Helper Functions
# ============================================================================
api_request() {
    local method="$1"
    local endpoint="$2"
    local data="${3:-}"
    local expected_status="${4:-200}"
    
    local url="${API_URL}/${API_VERSION}/${endpoint#/}"
    local response
    local status
    
    log_debug "API Request: $method $url"
    
    if [[ -n "$data" ]]; then
        response=$(curl -s -w "\n%{http_code}" -X "$method" \
            -H "Content-Type: application/json" \
            -d "$data" \
            "$url" 2>/dev/null)
    else
        response=$(curl -s -w "\n%{http_code}" -X "$method" \
            "$url" 2>/dev/null)
    fi
    
    status=$(echo "$response" | tail -n1)
    body=$(echo "$response" | head -n-1)
    
    if [[ "$status" == "$expected_status" ]]; then
        echo "$body"
        return 0
    else
        log_error "API request failed with status $status (expected $expected_status)"
        log_error "Response: $body"
        return 1
    fi
}

wait_for_condition() {
    local condition="$1"
    local timeout="${2:-$TEST_TIMEOUT}"
    local message="${3:-Waiting for condition}"
    
    log_info "$message (timeout: ${timeout}s)"
    
    local elapsed=0
    while [[ $elapsed -lt $timeout ]]; do
        if eval "$condition"; then
            log_success "Condition met after ${elapsed}s"
            return 0
        fi
        sleep 1
        ((elapsed++))
    done
    
    log_error "Timeout waiting for condition after ${timeout}s"
    return 1
}

# ============================================================================
# Process Management Functions
# ============================================================================
start_background_process() {
    local command="$1"
    local name="${2:-background-process}"
    local log_file="${TEST_DIR}/${name}.log"
    
    log_info "Starting $name"
    eval "$command" > "$log_file" 2>&1 &
    local pid=$!
    
    echo "$pid" > "${TEST_DIR}/${name}.pid"
    log_debug "$name started with PID $pid"
    return 0
}

stop_background_process() {
    local name="${1:-background-process}"
    local pid_file="${TEST_DIR}/${name}.pid"
    
    if [[ -f "$pid_file" ]]; then
        local pid=$(cat "$pid_file")
        if kill -0 "$pid" 2>/dev/null; then
            log_info "Stopping $name (PID: $pid)"
            kill "$pid"
            wait "$pid" 2>/dev/null
            rm -f "$pid_file"
        fi
    fi
}

# ============================================================================
# Utility Functions
# ============================================================================
generate_test_name() {
    echo "test-$(date +%Y%m%d-%H%M%S)-$$"
}

random_string() {
    local length="${1:-8}"
    cat /dev/urandom | tr -dc 'a-z0-9' | fold -w "$length" | head -n 1
}

get_script_dir() {
    cd "$(dirname "${BASH_SOURCE[0]}")" && pwd
}

# ============================================================================
# Initialization
# ============================================================================
# Set up trap for cleanup on exit
trap 'cleanup_test_environment' EXIT

# Export common variables
export SCRIPT_DIR="$(get_script_dir)"
export PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

log_debug "Test utilities loaded from: $SCRIPT_DIR/test-utils.sh"
log_debug "Project root: $PROJECT_ROOT"