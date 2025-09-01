#!/bin/bash

# Storage Fix Verification Test
# Tests the storage layer bucket/key handling fixes
# Verifies no double prefixing and correct path structure in SeaweedFS

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
API_URL="${PLOY_API_URL:-https://api.dev.ployman.app/v1}"
SEAWEEDFS_URL="${SEAWEEDFS_URL:-http://45.12.75.241:8888}"
TEST_JOB_ID="storage-test-$(date +%Y%m%d-%H%M%S)"
TEST_RESULTS_DIR="/tmp/storage-fix-test-results"

# Test counters
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# Create test results directory
mkdir -p "$TEST_RESULTS_DIR"

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[✓]${NC} $1"
    PASSED_TESTS=$((PASSED_TESTS + 1))
}

log_error() {
    echo -e "${RED}[✗]${NC} $1"
    FAILED_TESTS=$((FAILED_TESTS + 1))
}

log_test() {
    echo -e "${YELLOW}[TEST]${NC} $1"
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
}

# Function to check path in SeaweedFS
check_seaweedfs_path() {
    local path="$1"
    local expected_prefix="$2"
    local should_exist="$3"
    
    log_test "Checking path: $path"
    
    # Try to access the path in SeaweedFS
    local response=$(curl -s -o /dev/null -w "%{http_code}" "$SEAWEEDFS_URL/$path" || echo "000")
    
    if [[ "$should_exist" == "true" ]]; then
        if [[ "$response" == "200" || "$response" == "204" ]]; then
            log_success "Path exists: $path"
            
            # Check for double prefix
            if [[ "$path" == *"artifacts/artifacts/"* ]]; then
                log_error "CRITICAL: Double bucket prefix detected in path: $path"
                return 1
            fi
            
            # Verify correct prefix
            if [[ "$path" == "$expected_prefix"* ]]; then
                log_success "Path has correct prefix: $expected_prefix"
            else
                log_error "Path has incorrect prefix. Expected: $expected_prefix, Got: $path"
                return 1
            fi
        else
            log_error "Path should exist but returned: $response"
            return 1
        fi
    else
        if [[ "$response" == "404" || "$response" == "000" ]]; then
            log_success "Path correctly does not exist: $path"
        else
            log_error "Path should not exist but returned: $response"
            return 1
        fi
    fi
    
    return 0
}

# Function to test OpenRewrite transformation
test_openrewrite_transformation() {
    log_info "Testing OpenRewrite transformation with job ID: $TEST_JOB_ID"
    
    # Create test request
    local request_json=$(cat <<EOF
{
    "recipe_id": "org.openrewrite.java.migrate.Java8toJava11",
    "type": "openrewrite",
    "codebase": {
        "repository": "https://github.com/winterbe/java8-tutorial.git",
        "branch": "master"
    }
}
EOF
)
    
    # Save request for debugging
    echo "$request_json" > "$TEST_RESULTS_DIR/request.json"
    
    # Submit transformation request
    log_test "Submitting transformation request"
    local response=$(curl -s -X POST \
        -H "Content-Type: application/json" \
        -d "$request_json" \
        "$API_URL/arf/transform" \
        -o "$TEST_RESULTS_DIR/transform_response.json" \
        -w "%{http_code}")
    
    if [[ "$response" == "200" || "$response" == "202" ]]; then
        log_success "Transformation request accepted (HTTP $response)"
    else
        log_error "Transformation request failed (HTTP $response)"
        cat "$TEST_RESULTS_DIR/transform_response.json"
        return 1
    fi
    
    # Extract job ID from response (if available)
    if command -v jq &> /dev/null; then
        local actual_job_id=$(jq -r '.job_id // empty' "$TEST_RESULTS_DIR/transform_response.json")
        if [[ -n "$actual_job_id" ]]; then
            TEST_JOB_ID="$actual_job_id"
            log_info "Using job ID from response: $TEST_JOB_ID"
        fi
    fi
    
    return 0
}

# Function to verify storage paths
verify_storage_paths() {
    log_info "Verifying storage paths for job: $TEST_JOB_ID"
    
    # Expected paths (without double prefixing)
    local correct_input_path="artifacts/jobs/$TEST_JOB_ID/input.tar"
    local correct_output_path="artifacts/jobs/$TEST_JOB_ID/output.tar"
    
    # Incorrect paths (with double prefixing - should NOT exist)
    local wrong_input_path="artifacts/artifacts/jobs/$TEST_JOB_ID/input.tar"
    local wrong_output_path="artifacts/artifacts/jobs/$TEST_JOB_ID/output.tar"
    
    # Check correct paths
    log_test "Checking for correct input path"
    check_seaweedfs_path "$correct_input_path" "artifacts/jobs/" "true" || true
    
    log_test "Checking for correct output path (may not exist yet)"
    check_seaweedfs_path "$correct_output_path" "artifacts/jobs/" "false" || true
    
    # Check that incorrect paths do NOT exist
    log_test "Verifying no double-prefixed input path"
    check_seaweedfs_path "$wrong_input_path" "" "false"
    
    log_test "Verifying no double-prefixed output path"
    check_seaweedfs_path "$wrong_output_path" "" "false"
}

# Function to list all paths under artifacts/jobs/
list_job_paths() {
    log_info "Listing all paths under artifacts/jobs/ in SeaweedFS"
    
    local list_response=$(curl -s "$SEAWEEDFS_URL/artifacts/jobs/?pretty=y" || echo "{}")
    echo "$list_response" > "$TEST_RESULTS_DIR/seaweedfs_listing.json"
    
    if [[ "$list_response" == *"Entries"* ]]; then
        log_success "Successfully retrieved directory listing"
        
        # Check for any double-prefixed paths in the listing
        if [[ "$list_response" == *"artifacts/artifacts/"* ]]; then
            log_error "CRITICAL: Found double-prefixed paths in SeaweedFS!"
            echo "$list_response" | grep -o "artifacts/artifacts/[^\"]*" || true
        else
            log_success "No double-prefixed paths found in listing"
        fi
    else
        log_info "Could not retrieve directory listing (may be empty or restricted)"
    fi
}

# Main test execution
main() {
    echo "======================================"
    echo "Storage Fix Verification Test"
    echo "======================================"
    echo "API URL: $API_URL"
    echo "SeaweedFS URL: $SEAWEEDFS_URL"
    echo "Test Job ID: $TEST_JOB_ID"
    echo "======================================"
    echo
    
    # Test 1: Basic path structure verification
    log_info "Test 1: Basic Path Structure"
    log_test "Checking SeaweedFS connectivity"
    if curl -s -o /dev/null "$SEAWEEDFS_URL/"; then
        log_success "SeaweedFS is accessible"
    else
        log_error "Cannot connect to SeaweedFS at $SEAWEEDFS_URL"
    fi
    
    # Test 2: Submit transformation and verify paths
    log_info "Test 2: OpenRewrite Transformation"
    if [[ "${SKIP_TRANSFORM:-false}" != "true" ]]; then
        test_openrewrite_transformation
        sleep 5  # Give it time to create initial paths
        verify_storage_paths
    else
        log_info "Skipping transformation test (SKIP_TRANSFORM=true)"
    fi
    
    # Test 3: List and verify all job paths
    log_info "Test 3: Directory Listing Verification"
    list_job_paths
    
    # Test 4: Direct path verification
    log_info "Test 4: Direct Path Tests"
    
    # Test that we can create a path without double prefix
    local test_key="jobs/direct-test-$(date +%s)/test.txt"
    log_test "Testing direct path creation: artifacts/$test_key"
    
    # This would require API access to create, skipping for now
    log_info "Direct path creation would require API endpoint"
    
    # Summary
    echo
    echo "======================================"
    echo "Test Results Summary"
    echo "======================================"
    echo "Total Tests: $TOTAL_TESTS"
    echo -e "${GREEN}Passed: $PASSED_TESTS${NC}"
    echo -e "${RED}Failed: $FAILED_TESTS${NC}"
    
    if [[ $FAILED_TESTS -eq 0 ]]; then
        echo -e "${GREEN}✓ All tests passed!${NC}"
        echo "Storage layer is correctly handling bucket prefixes."
        exit 0
    else
        echo -e "${RED}✗ Some tests failed!${NC}"
        echo "Check $TEST_RESULTS_DIR for detailed results."
        exit 1
    fi
}

# Run main function
main "$@"