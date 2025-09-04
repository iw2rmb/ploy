#!/bin/bash

# OpenRewrite Comprehensive Transformation Test Suite
# Tests all transformation scenarios from @roadmap/orw-test.md

set -e

API_URL="${API_URL:-https://api.dev.ployman.app}"
RESULTS_DIR="tests/results/openrewrite-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$RESULTS_DIR"

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Summary counters
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# Function to print colored output
print_status() {
    echo -e "${BLUE}[$(date '+%H:%M:%S')]${NC} $1"
}

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

# Function to run a transformation test
run_transformation() {
    local recipe_id="$1"
    local repository="$2"
    local test_name="$3"
    local expected_changes="$4"
    
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    
    print_status "Testing: $test_name"
    print_status "Recipe: $recipe_id"
    print_status "Repository: $repository"
    
    # Start transformation
    local response=$(curl -s -X POST "${API_URL}/v1/arf/transforms" \
        -H "Content-Type: application/json" \
        -d "{
            \"recipe_id\": \"$recipe_id\",
            \"type\": \"openrewrite\",
            \"codebase\": {
                \"repository\": \"$repository\",
                \"branch\": \"main\"
            }
        }")
    
    local transform_id=$(echo "$response" | jq -r '.transformation_id')
    
    if [ "$transform_id" == "null" ] || [ -z "$transform_id" ]; then
        print_error "Failed to start transformation: $response"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        return 1
    fi
    
    print_status "Transformation ID: $transform_id"
    
    # Monitor status
    local max_wait=60
    local wait_time=0
    local status="in_progress"
    
    while [ "$status" == "in_progress" ] && [ $wait_time -lt $max_wait ]; do
        sleep 3
        wait_time=$((wait_time + 3))
        
        local status_response=$(curl -s "${API_URL}/v1/arf/transforms/${transform_id}/status")
        status=$(echo "$status_response" | jq -r '.status')
        
        if [ "$status" == "in_progress" ]; then
            local progress=$(echo "$status_response" | jq -r '.progress.percent_complete // 0')
            print_status "Progress: ${progress}%"
        fi
    done
    
    # Save full status response
    echo "$status_response" | jq '.' > "$RESULTS_DIR/${test_name//[ \/]/-}-status.json"
    
    # Check final status
    if [ "$status" == "completed" ]; then
        print_success "Transformation completed successfully"
        
        # Get transformation details
        local details_response=$(curl -s "${API_URL}/v1/arf/transforms/${transform_id}")
        echo "$details_response" | jq '.' > "$RESULTS_DIR/${test_name//[ \/]/-}-details.json"
        
        # Extract key metrics
        local changes_applied=$(echo "$status_response" | jq -r '.changes_applied // 0')
        local files_modified=$(echo "$status_response" | jq -r '.files_modified | length // 0')
        
        print_status "Changes applied: $changes_applied"
        print_status "Files modified: $files_modified"
        
        # Extract and save diff
        local diff=$(echo "$status_response" | jq -r '.diff // "No diff available"')
        if [ "$diff" != "No diff available" ] && [ "$diff" != "null" ]; then
            echo "$diff" > "$RESULTS_DIR/${test_name//[ \/]/-}.diff"
            print_success "Diff saved to results directory"
            
            # Show a snippet of the diff
            echo -e "\n${BLUE}--- Diff Preview ---${NC}"
            echo "$diff" | head -20
            echo -e "${BLUE}--- (truncated) ---${NC}\n"
        else
            print_warning "No diff available"
        fi
        
        # Get human-readable report
        local report=$(curl -s "${API_URL}/v1/arf/transforms/${transform_id}/report" 2>/dev/null || echo "")
        if [ -n "$report" ] && [ "$report" != "null" ]; then
            echo "$report" > "$RESULTS_DIR/${test_name//[ \/]/-}-report.md"
            print_success "Report saved to results directory"
        fi
        
        # Verify expected changes
        if [ -n "$expected_changes" ] && [ "$changes_applied" -gt 0 ]; then
            print_success "Test PASSED: Changes were applied as expected"
            PASSED_TESTS=$((PASSED_TESTS + 1))
        elif [ -z "$expected_changes" ]; then
            print_success "Test PASSED: Transformation completed"
            PASSED_TESTS=$((PASSED_TESTS + 1))
        else
            print_error "Test FAILED: Expected changes but got $changes_applied"
            FAILED_TESTS=$((FAILED_TESTS + 1))
        fi
        
    elif [ "$status" == "failed" ]; then
        local error=$(echo "$status_response" | jq -r '.error // "Unknown error"')
        print_error "Transformation failed: $error"
        FAILED_TESTS=$((FAILED_TESTS + 1))
    else
        print_error "Transformation timeout or unknown status: $status"
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
    
    echo ""
    return 0
}

# Main test execution
echo "========================================"
echo "OpenRewrite Transformation Test Suite"
echo "========================================"
echo "API: $API_URL"
echo "Results: $RESULTS_DIR"
echo ""

# Test 1: Basic Java Project (ploy-orw-test-java)
echo -e "${BLUE}=== Test Repository 1: ploy-orw-test-java ===${NC}"
echo ""

run_transformation \
    "org.openrewrite.java.RemoveUnusedImports" \
    "https://github.com/iw2rmb/ploy-orw-test-java.git" \
    "Test 1.1: Remove Unused Imports" \
    "expect_changes"

run_transformation \
    "org.openrewrite.java.cleanup.UseStringReplace" \
    "https://github.com/iw2rmb/ploy-orw-test-java.git" \
    "Test 1.2: Modernize String Operations" \
    ""

run_transformation \
    "org.openrewrite.java.migrate.UpgradeToJava17" \
    "https://github.com/iw2rmb/ploy-orw-test-java.git" \
    "Test 1.3: Java 17 Migration" \
    "expect_changes"

run_transformation \
    "org.openrewrite.java.cleanup.UnnecessaryParentheses" \
    "https://github.com/iw2rmb/ploy-orw-test-java.git" \
    "Test 1.4: Remove Unnecessary Parentheses" \
    ""

# Test 2: Legacy Java Project (ploy-orw-test-legacy)
echo -e "${BLUE}=== Test Repository 2: ploy-orw-test-legacy ===${NC}"
echo ""

run_transformation \
    "org.openrewrite.java.migrate.Java8toJava11" \
    "https://github.com/iw2rmb/ploy-orw-test-legacy.git" \
    "Test 2.1: Java 8 to 11 Migration" \
    "expect_changes"

run_transformation \
    "org.openrewrite.java.migrate.UpgradeToJava17" \
    "https://github.com/iw2rmb/ploy-orw-test-legacy.git" \
    "Test 2.2: Full Java 17 Upgrade" \
    "expect_changes"

# Test 3: Spring Boot Project (ploy-orw-test-spring)
echo -e "${BLUE}=== Test Repository 3: ploy-orw-test-spring ===${NC}"
echo ""

run_transformation \
    "org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_2" \
    "https://github.com/iw2rmb/ploy-orw-test-spring.git" \
    "Test 3.1: Spring Boot 3.2 Upgrade" \
    "expect_changes"

run_transformation \
    "org.openrewrite.java.RemoveUnusedImports" \
    "https://github.com/iw2rmb/ploy-orw-test-spring.git" \
    "Test 3.2: Spring Project Cleanup" \
    ""

# Generate summary report
echo ""
echo "========================================"
echo "Test Summary"
echo "========================================"
echo -e "Total Tests: ${BLUE}$TOTAL_TESTS${NC}"
echo -e "Passed: ${GREEN}$PASSED_TESTS${NC}"
echo -e "Failed: ${RED}$FAILED_TESTS${NC}"
echo ""

# Create summary file
cat > "$RESULTS_DIR/summary.txt" <<EOF
OpenRewrite Transformation Test Results
Generated: $(date)

Total Tests: $TOTAL_TESTS
Passed: $PASSED_TESTS
Failed: $FAILED_TESTS

Success Rate: $(echo "scale=2; $PASSED_TESTS * 100 / $TOTAL_TESTS" | bc)%

Test Details:
- Test Repository 1 (ploy-orw-test-java): Basic Java project
- Test Repository 2 (ploy-orw-test-legacy): Legacy Java 7 project
- Test Repository 3 (ploy-orw-test-spring): Spring Boot project

Results saved to: $RESULTS_DIR
EOF

if [ $FAILED_TESTS -eq 0 ]; then
    print_success "All tests passed successfully!"
    exit 0
else
    print_error "Some tests failed. Check results in $RESULTS_DIR"
    exit 1
fi