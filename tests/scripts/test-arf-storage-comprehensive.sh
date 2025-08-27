#!/bin/bash
# ARF Storage Comprehensive Test Suite
# Master test runner for all ARF storage and backend tests

set -e

SCRIPT_DIR=$(dirname "$0")
ROOT_DIR=$(cd "$SCRIPT_DIR/../.." && pwd)

# Configuration
CONTROLLER_URL="${PLOY_CONTROLLER:-https://api.dev.ployman.app/v1}"
TEMP_DIR="/tmp/arf-comprehensive-test"
MASTER_RESULTS_FILE="$TEMP_DIR/comprehensive-results.json"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_suite() {
    echo -e "${PURPLE}[SUITE]${NC} $1"
}

log_separator() {
    echo -e "${BLUE}=================================================================${NC}"
}

# Test suite definitions
declare -A TEST_SUITES=(
    ["storage-integration"]="test-arf-storage-integration.sh"
    ["backend-fallbacks"]="test-arf-backend-fallbacks.sh"
    ["storage-configuration"]="test-arf-storage-configuration.sh"
)

# Setup comprehensive test environment
setup_comprehensive_test() {
    log_info "Setting up ARF Storage Comprehensive Test Suite..."
    
    mkdir -p "$TEMP_DIR"
    
    # Check controller accessibility
    if ! curl -s -f "$CONTROLLER_URL/health" > /dev/null 2>&1; then
        log_error "Controller not accessible at $CONTROLLER_URL"
        log_error "Please ensure the controller is running and accessible"
        exit 1
    fi
    
    log_info "Controller is accessible at $CONTROLLER_URL"
    
    # Get controller version and environment info
    local controller_info=$(curl -s "$CONTROLLER_URL/api/version" 2>/dev/null || echo '{}')
    local git_branch=$(echo "$controller_info" | jq -r '.git_branch // "unknown"')
    local git_commit=$(echo "$controller_info" | jq -r '.git_commit // "unknown"')
    local build_timestamp=$(echo "$controller_info" | jq -r '.build_timestamp // "unknown"')
    
    log_info "Controller info: branch=$git_branch, commit=${git_commit:0:8}, build=$build_timestamp"
    
    # Initialize master results file
    cat > "$MASTER_RESULTS_FILE" << EOF
{
    "comprehensive_test_suite": "arf-storage-complete",
    "start_time": "$(date -Iseconds)",
    "timestamp": "$TIMESTAMP",
    "controller_url": "$CONTROLLER_URL",
    "controller_info": {
        "git_branch": "$git_branch",
        "git_commit": "$git_commit",
        "build_timestamp": "$build_timestamp"
    },
    "test_suites": [],
    "summary": {}
}
EOF
}

# Run individual test suite
run_test_suite() {
    local suite_name="$1"
    local script_name="$2"
    local script_path="$SCRIPT_DIR/$script_name"
    
    log_separator
    log_suite "Running $suite_name test suite..."
    log_separator
    
    if [ ! -f "$script_path" ]; then
        log_error "Test script not found: $script_path"
        record_suite_result "$suite_name" "ERROR" "Script not found" 0 0 1 0
        return 1
    fi
    
    if [ ! -x "$script_path" ]; then
        log_warn "Making test script executable: $script_path"
        chmod +x "$script_path"
    fi
    
    local start_time=$(date +%s)
    local suite_exit_code=0
    
    # Run the test suite and capture output
    local output_file="$TEMP_DIR/${suite_name}-output.log"
    if bash "$script_path" 2>&1 | tee "$output_file"; then
        suite_exit_code=0
        log_info "✓ $suite_name test suite completed successfully"
    else
        suite_exit_code=$?
        log_error "✗ $suite_name test suite failed (exit code: $suite_exit_code)"
    fi
    
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    
    # Try to extract test counts from the output
    local passed_count=0
    local failed_count=0
    local warned_count=0
    local total_count=0
    
    if [ -f "$output_file" ]; then
        # Look for test result patterns in the output
        passed_count=$(grep -c "✓.*passed\|PASS" "$output_file" 2>/dev/null || echo 0)
        failed_count=$(grep -c "✗.*failed\|FAIL" "$output_file" 2>/dev/null || echo 0)
        warned_count=$(grep -c "!.*warn\|WARN" "$output_file" 2>/dev/null || echo 0)
        
        # Try to extract more precise counts from JSON results if available
        local temp_results=$(find "$TEMP_DIR" -name "*results.json" -newer "$output_file" 2>/dev/null | head -1)
        if [ -n "$temp_results" ] && [ -f "$temp_results" ]; then
            passed_count=$(jq '.tests | map(select(.status == "PASS")) | length' "$temp_results" 2>/dev/null || echo $passed_count)
            failed_count=$(jq '.tests | map(select(.status == "FAIL")) | length' "$temp_results" 2>/dev/null || echo $failed_count)
            warned_count=$(jq '.tests | map(select(.status == "WARN")) | length' "$temp_results" 2>/dev/null || echo $warned_count)
        fi
        
        total_count=$((passed_count + failed_count + warned_count))
        if [ $total_count -eq 0 ]; then
            total_count=1  # Assume at least one test was run
        fi
    fi
    
    # Determine overall suite status
    local suite_status="PASS"
    if [ $suite_exit_code -ne 0 ]; then
        suite_status="FAIL"
    elif [ $failed_count -gt 0 ]; then
        suite_status="FAIL"
    elif [ $warned_count -gt 0 ]; then
        suite_status="WARN"
    fi
    
    # Record suite results
    record_suite_result "$suite_name" "$suite_status" "Duration: ${duration}s" $total_count $passed_count $failed_count $warned_count
    
    log_info "$suite_name results: $total_count total, $passed_count passed, $failed_count failed, $warned_count warnings"
    
    return $suite_exit_code
}

# Record test suite results
record_suite_result() {
    local suite_name="$1"
    local status="$2"
    local details="$3"
    local total="$4"
    local passed="$5"
    local failed="$6"
    local warned="$7"
    local end_time=$(date -Iseconds)
    
    # Update master results file
    tmp_file=$(mktemp)
    jq --arg name "$suite_name" --arg status "$status" --arg details "$details" \
       --arg total "$total" --arg passed "$passed" --arg failed "$failed" --arg warned "$warned" --arg time "$end_time" \
       '.test_suites += [{
           name: $name, 
           status: $status, 
           details: $details, 
           total_tests: ($total | tonumber),
           passed_tests: ($passed | tonumber),
           failed_tests: ($failed | tonumber),
           warned_tests: ($warned | tonumber),
           completed_at: $time
       }]' \
       "$MASTER_RESULTS_FILE" > "$tmp_file" && mv "$tmp_file" "$MASTER_RESULTS_FILE"
}

# Generate comprehensive final report
generate_comprehensive_report() {
    log_separator
    log_info "Generating comprehensive test report..."
    log_separator
    
    # Update results with end time
    local end_time=$(date -Iseconds)
    tmp_file=$(mktemp)
    jq --arg time "$end_time" '.end_time = $time' "$MASTER_RESULTS_FILE" > "$tmp_file" && mv "$tmp_file" "$MASTER_RESULTS_FILE"
    
    # Calculate summary statistics
    local total_suites=$(jq '.test_suites | length' "$MASTER_RESULTS_FILE")
    local passed_suites=$(jq '.test_suites | map(select(.status == "PASS")) | length' "$MASTER_RESULTS_FILE")
    local failed_suites=$(jq '.test_suites | map(select(.status == "FAIL")) | length' "$MASTER_RESULTS_FILE")
    local warned_suites=$(jq '.test_suites | map(select(.status == "WARN")) | length' "$MASTER_RESULTS_FILE")
    
    local total_tests=$(jq '.test_suites | map(.total_tests) | add' "$MASTER_RESULTS_FILE")
    local total_passed=$(jq '.test_suites | map(.passed_tests) | add' "$MASTER_RESULTS_FILE")
    local total_failed=$(jq '.test_suites | map(.failed_tests) | add' "$MASTER_RESULTS_FILE")
    local total_warned=$(jq '.test_suites | map(.warned_tests) | add' "$MASTER_RESULTS_FILE")
    
    # Add summary to results
    tmp_file=$(mktemp)
    jq --arg total_suites "$total_suites" --arg passed_suites "$passed_suites" \
       --arg failed_suites "$failed_suites" --arg warned_suites "$warned_suites" \
       --arg total_tests "$total_tests" --arg total_passed "$total_passed" \
       --arg total_failed "$total_failed" --arg total_warned "$total_warned" \
       '.summary = {
           total_suites: ($total_suites | tonumber),
           passed_suites: ($passed_suites | tonumber),
           failed_suites: ($failed_suites | tonumber),
           warned_suites: ($warned_suites | tonumber),
           total_tests: ($total_tests | tonumber),
           passed_tests: ($total_passed | tonumber),
           failed_tests: ($total_failed | tonumber),
           warned_tests: ($total_warned | tonumber)
       }' \
       "$MASTER_RESULTS_FILE" > "$tmp_file" && mv "$tmp_file" "$MASTER_RESULTS_FILE"
    
    # Display comprehensive report
    log_info "=================== COMPREHENSIVE REPORT ==================="
    log_info "ARF Storage Test Suite Results"
    log_info "Timestamp: $TIMESTAMP"
    log_info "Controller: $CONTROLLER_URL"
    log_info ""
    log_info "TEST SUITES:"
    log_info "  Total Suites: $total_suites"
    log_info "  Passed: $passed_suites"
    log_info "  Failed: $failed_suites"
    log_info "  Warnings: $warned_suites"
    log_info ""
    log_info "INDIVIDUAL TESTS:"
    log_info "  Total Tests: $total_tests"
    log_info "  Passed: $total_passed"
    log_info "  Failed: $total_failed"  
    log_info "  Warnings: $total_warned"
    log_info ""
    
    # Display individual suite results
    log_info "SUITE BREAKDOWN:"
    jq -r '.test_suites[] | "  \(.name): \(.status) (\(.passed_tests)/\(.total_tests) passed)"' "$MASTER_RESULTS_FILE"
    log_info ""
    log_info "Results saved to: $MASTER_RESULTS_FILE"
    
    # Determine overall result
    local overall_result="SUCCESS"
    local exit_code=0
    
    if [ "$failed_suites" -gt 0 ]; then
        overall_result="FAILURE"
        exit_code=1
        log_error "Overall Result: $overall_result ($failed_suites/$total_suites suites failed)"
    elif [ "$warned_suites" -gt 0 ]; then
        overall_result="SUCCESS_WITH_WARNINGS"
        log_warn "Overall Result: $overall_result ($warned_suites/$total_suites suites had warnings)"
    else
        log_info "Overall Result: $overall_result (all suites passed)"
    fi
    
    log_separator
    
    return $exit_code
}

# Print usage information
usage() {
    echo "Usage: $0 [OPTIONS] [SUITE_NAMES...]"
    echo ""
    echo "ARF Storage Comprehensive Test Suite"
    echo ""
    echo "OPTIONS:"
    echo "  -h, --help     Show this help message"
    echo "  -l, --list     List available test suites"
    echo "  -a, --all      Run all test suites (default)"
    echo "  -v, --verbose  Enable verbose output"
    echo ""
    echo "SUITE_NAMES:"
    echo "  If specified, only run the named test suites"
    echo "  Available suites: ${!TEST_SUITES[*]}"
    echo ""
    echo "EXAMPLES:"
    echo "  $0                                    # Run all suites"
    echo "  $0 storage-integration               # Run only integration tests"
    echo "  $0 backend-fallbacks storage-config  # Run specific suites"
    echo ""
}

# Main execution function
main() {
    local run_all=true
    local selected_suites=()
    local verbose=false
    
    # Parse command line arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                usage
                exit 0
                ;;
            -l|--list)
                echo "Available ARF Storage Test Suites:"
                for suite in "${!TEST_SUITES[@]}"; do
                    echo "  $suite -> ${TEST_SUITES[$suite]}"
                done
                exit 0
                ;;
            -a|--all)
                run_all=true
                shift
                ;;
            -v|--verbose)
                verbose=true
                shift
                ;;
            -*)
                echo "Unknown option: $1"
                usage
                exit 1
                ;;
            *)
                # Check if it's a valid suite name
                if [[ -n "${TEST_SUITES[$1]}" ]]; then
                    selected_suites+=("$1")
                    run_all=false
                else
                    echo "Unknown test suite: $1"
                    echo "Available suites: ${!TEST_SUITES[*]}"
                    exit 1
                fi
                shift
                ;;
        esac
    done
    
    # Determine which suites to run
    local suites_to_run=()
    if [ "$run_all" = true ]; then
        suites_to_run=(${!TEST_SUITES[@]})
    else
        suites_to_run=("${selected_suites[@]}")
    fi
    
    log_info "Starting ARF Storage Comprehensive Test Suite"
    log_info "Controller URL: $CONTROLLER_URL"
    log_info "Suites to run: ${suites_to_run[*]}"
    
    setup_comprehensive_test
    
    # Run selected test suites
    local suite_failures=0
    for suite in "${suites_to_run[@]}"; do
        if run_test_suite "$suite" "${TEST_SUITES[$suite]}"; then
            log_info "Suite '$suite' completed successfully"
        else
            log_error "Suite '$suite' failed"
            suite_failures=$((suite_failures + 1))
        fi
        echo ""  # Add spacing between suites
    done
    
    # Generate final comprehensive report
    generate_comprehensive_report
    
    local exit_code=$?
    if [ $exit_code -eq 0 ]; then
        log_info "ARF Storage Comprehensive Test Suite PASSED"
    else
        log_error "ARF Storage Comprehensive Test Suite FAILED"
    fi
    
    exit $exit_code
}

# Execute main function with all arguments
main "$@"